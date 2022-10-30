// Package gas implements a gas price oracle.
package gas

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/oasisprotocol/oasis-core/go/common/quantity"
	"github.com/oasisprotocol/oasis-core/go/common/service"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/oasisprotocol/emerald-web3-gateway/db/model"
	"github.com/oasisprotocol/emerald-web3-gateway/indexer"
)

var (
	metricNodeMinPrice  = promauto.NewGauge(prometheus.GaugeOpts{Name: "oasis_emerald_web3_gateway_gas_orcale_node_min_price", Help: "Min gas price periodically queried from the node."})
	metricComputedPrice = promauto.NewGauge(prometheus.GaugeOpts{Name: "oasis_emerald_web3_gateway_gas_oracle_computed_price", Help: "Computed recommended gas price based on recent full blocks. -1 if none (no recent full blocks)."})
)

// Backend is the gas price oracle backend.
type Backend interface {
	service.BackgroundService

	// GasPrice returns the currently recommended minimum gas price.
	GasPrice() *hexutil.Big
}

const (
	// windowSize is the number of recent blocks to use for calculating min gas price.
	// NOTE: code assumes that this is relatively small.
	windowSize = 12

	// fullBlockThreshold is the percentage of block used gas over which a block should
	// be considered full.
	fullBlockThreshold = 0.8

	// period for querying oasis node for minimum gas price.
	nodeMinGasPriceTimeout = 60 * time.Second
)

var (
	// minPriceEps is a constant added to the cheapest transaction executed in last windowSize blocks.
	minPriceEps = *quantity.NewFromUint64(1_000_000_000) // 1 "gwei".

	// defaultGasPrice is the default gas price reported if no better estimate can be returned.
	//
	// This is only returned when all calls to oasis-node for min gas price failed and there were no
	// full blocks in last `windowSize` blocks.
	defaultGasPrice = *quantity.NewFromUint64(100_000_000_000) // 100 "gwei".
)

// gasPriceOracle implements the gas price backend by looking at transaction costs in previous blocks.
//
// The gas price oracle does roughly:
// (a) Compute the recommended gas price based on recent blocks:
//   - look at the most recent block(s) (controlled by `windowSize` parameter)
//   - if block gas used is greater than some threshold, consider it "full" (controlled by `fullBlockThresholdParameter`)
//   - set recommended gas price to the lowest-priced transaction from the full blocks + a small constant (controlled by `minPriceEps`)
//
// (b) Query gas price configured on the oasis-node:
//   - periodically query the oasis-node for it's configured gas price
//
// Return the greater of the (a) and (b), default to a `defaultGasPrice `if neither are available.
type gasPriceOracle struct {
	service.BaseBackgroundService

	ctx       context.Context
	cancelCtx context.CancelFunc

	// protects nodeMinGasPrice and computedMinGasPrice.
	priceLock sync.RWMutex
	// nodeMinGasPrice is the minimum gas price as reported by the oasis node.
	// This is queried from the node and updated periodically.
	nodeMinGasPrice *quantity.Quantity
	// computedMinGasPrice is the computed min gas price by observing recent blocks.
	computedMinGasPrice *quantity.Quantity

	// blockPrices is a rolling-array containing minimum transaction prices for
	// last up to `windowSize` blocks.
	blockPrices []*quantity.Quantity
	// tracks the current index of the blockPrices rolling array.:w
	blockPricesCurrentIdx int

	blockWatcher indexer.BlockWatcher
	coreClient   core.V1
}

func New(ctx context.Context, blockWatcher indexer.BlockWatcher, coreClient core.V1) Backend {
	ctxB, cancelCtx := context.WithCancel(ctx)
	g := &gasPriceOracle{
		BaseBackgroundService: *service.NewBaseBackgroundService("gas-price-oracle"),
		ctx:                   ctxB,
		cancelCtx:             cancelCtx,
		blockPrices:           make([]*quantity.Quantity, 0, windowSize),
		blockWatcher:          blockWatcher,
		coreClient:            coreClient,
	}

	return g
}

// Start starts service.
func (g *gasPriceOracle) Start() error {
	go g.nodeMinGasPriceFetcher()
	go g.indexedBlockWatcher()

	return nil
}

// Stop stops service.
func (g *gasPriceOracle) Stop() {
	g.cancelCtx()
}

func (g *gasPriceOracle) GasPrice() *hexutil.Big {
	g.priceLock.RLock()
	defer g.priceLock.RUnlock()

	if g.computedMinGasPrice == nil && g.nodeMinGasPrice == nil {
		// No blocks tracked yet and no min gas price from the node,
		// default to a default value.
		price := hexutil.Big(*defaultGasPrice.ToBigInt())
		return &price
	}

	// Set minPrice to the larger of the `nodeMinGasPrice` and `computedMinGasPrice`.
	minPrice := quantity.NewQuantity()
	if g.nodeMinGasPrice != nil {
		minPrice = g.nodeMinGasPrice.Clone()
	}
	if g.computedMinGasPrice != nil && g.computedMinGasPrice.Cmp(minPrice) > 0 {
		minPrice = g.computedMinGasPrice.Clone()
		// Add small constant to the min price.
		if err := minPrice.Add(&minPriceEps); err != nil {
			g.Logger.Error("failed to add minPriceEps to minPrice", "err", err, "min_price", minPrice, "min_price_eps", minPriceEps)
			minPrice = &defaultGasPrice
		}
	}

	price := hexutil.Big(*minPrice.ToBigInt())
	return &price
}

func (g *gasPriceOracle) nodeMinGasPriceFetcher() {
	for {
		// Fetch and update min gas price from the node.
		g.fetchMinGasPrice(g.ctx)

		select {
		case <-g.ctx.Done():
			return
		case <-time.After(nodeMinGasPriceTimeout):
		}
	}
}

func (g *gasPriceOracle) fetchMinGasPrice(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	mgp, err := g.coreClient.MinGasPrice(ctx)
	if err != nil {
		g.Logger.Error("node min gas price query failed", "err", err)
		return
	}
	price := mgp[types.NativeDenomination]

	g.priceLock.Lock()
	g.nodeMinGasPrice = &price
	g.priceLock.Unlock()

	metricNodeMinPrice.Set(float64(price.ToBigInt().Int64()))
}

func (g *gasPriceOracle) indexedBlockWatcher() {
	ch, sub, err := g.blockWatcher.WatchBlocks(g.ctx, windowSize)
	if err != nil {
		g.Logger.Error("indexed block watcher failed to watch blocks", "err", err)
		return
	}
	defer sub.Close()

	for {
		select {
		case <-g.ctx.Done():
			return
		case blk := <-ch:
			g.onBlock(blk.Block, blk.LastTransactionPrice)
		}
	}
}

func (g *gasPriceOracle) onBlock(b *model.Block, lastTxPrice *quantity.Quantity) {
	// Consider block full if block gas used is greater than `fullBlockThreshold` of gas limit.
	blockFull := (float64(b.Header.GasLimit) * fullBlockThreshold) <= float64(b.Header.GasUsed)
	if !blockFull {
		// Track 0 for non-full blocks.
		g.trackPrice(quantity.NewFromUint64(0))
		return
	}

	if lastTxPrice == nil {
		g.Logger.Error("no last tx gas price for block", "block", b)
		return
	}
	g.trackPrice(lastTxPrice)
}

func (g *gasPriceOracle) trackPrice(price *quantity.Quantity) {
	// One item always gets added added to the prices array.
	// Bump the current index for next iteration.
	defer func() {
		g.blockPricesCurrentIdx = (g.blockPricesCurrentIdx + 1) % windowSize
	}()

	// Recalculate min-price over the block window.
	defer func() {
		minPrice := quantity.NewFromUint64(math.MaxUint64)
		// Find smallest non-zero gas price.
		for _, price := range g.blockPrices {
			if price.IsZero() {
				continue
			}
			if price.Cmp(minPrice) < 0 {
				minPrice = price
			}
		}

		// No full blocks among last `windowSize` blocks.
		if minPrice.Cmp(quantity.NewFromUint64(math.MaxUint64)) == 0 {
			g.priceLock.Lock()
			g.computedMinGasPrice = nil
			g.priceLock.Unlock()
			metricComputedPrice.Set(float64(-1))

			return
		}
		g.priceLock.Lock()
		g.computedMinGasPrice = minPrice
		g.priceLock.Unlock()
		metricComputedPrice.Set(float64(minPrice.ToBigInt().Int64()))
	}()

	if len(g.blockPrices) < windowSize {
		g.blockPrices = append(g.blockPrices, price)
		return
	}
	g.blockPrices[g.blockPricesCurrentIdx] = price
}
