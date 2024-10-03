// Package gas implements a gas price oracle.
package gas

import (
	"context"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/oasisprotocol/oasis-core/go/common/quantity"
	"github.com/oasisprotocol/oasis-core/go/common/service"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"

	"github.com/oasisprotocol/oasis-web3-gateway/conf"
	"github.com/oasisprotocol/oasis-web3-gateway/db/model"
	"github.com/oasisprotocol/oasis-web3-gateway/indexer"
)

var (
	metricNodeMinPrice  = promauto.NewGauge(prometheus.GaugeOpts{Name: "oasis_web3_gateway_gas_oracle_node_min_price", Help: "Min gas price queried from the node."})
	metricComputedPrice = promauto.NewGauge(prometheus.GaugeOpts{Name: "oasis_web3_gateway_gas_oracle_computed_price", Help: "Computed recommended gas price based on recent full blocks. -1 if none (no recent full blocks)."})
)

// Backend is the gas price oracle backend.
type Backend interface {
	service.BackgroundService

	// GasPrice returns the currently recommended minimum gas price.
	GasPrice() *hexutil.Big

	// FeeHistory returns the fee history of the last blocks and percentiles.
	FeeHistory(blockCount uint64, lastBlock rpc.BlockNumber, percentiles []float64) *FeeHistoryResult
}

const (
	// defaultWindowSize is the default number of recent blocks to use for calculating min gas price.
	// NOTE: code assumes that this is relatively small.
	defaultWindowSize uint64 = 6

	// defaultFullBlockThreshold is the default percentage of block used gas over which a block
	// should be considered full.
	defaultFullBlockThreshold float64 = 0.5
)

var (
	// defaultComputedPriceMargin is the default constant added to the median transaction gas price.
	defaultComputedPriceMargin = *quantity.NewFromUint64(1_000_000_000) // 1 "gwei".

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
//   - set recommended gas price to the maximum of the median-priced transactions from recent full blocks + a small constant (controlled by `computedPriceMargin`)
//
// This handles the case when most blocks are full and the cheapest transactions are higher than the configured min gas price.
// This can be the case if dynamic min gas price is disabled for the runtime, or the transactions are increasing in price faster
// than the dynamic gas price adjustments.
//
// (b) Query gas price configured on the oasis-node:
//   - after every block, query the oasis-node for its configured gas price
//
// This handles the case when min gas price is higher than the heuristically computed price in (a).
// This can happen if node has overridden the min gas price configuration or if dynamic min gas price is enabled and
// has just increased.
//
// Return the greater of (a) and (b), default to a `defaultGasPrice `if neither are available.
type gasPriceOracle struct {
	service.BaseBackgroundService

	ctx       context.Context
	cancelCtx context.CancelFunc

	// protects nodeMinGasPrice and computedMinGasPrice.
	priceLock sync.RWMutex
	// nodeMinGasPrice is the minimum gas price as reported by the oasis node.
	// This is queried from the node and updated periodically.
	nodeMinGasPrice *quantity.Quantity
	// computedGasPrice is the computed suggested gas price by observing recent blocks.
	computedGasPrice *quantity.Quantity

	// blockPrices is a rolling-array containing minimum transaction prices for
	// last up to `windowSize` blocks.
	blockPrices []*quantity.Quantity
	// tracks the current index of the blockPrices rolling array.:w
	blockPricesCurrentIdx int

	// protects feeHistoryData.
	feeHistoryLock sync.RWMutex
	// feeHistoryData contains the per block data needed to compute the fee history.
	feeHistoryData []*feeHistoryBlockData

	// Configuration parameters.
	windowSize          uint64
	fullBlockThreshold  float64
	defaultGasPrice     quantity.Quantity
	computedPriceMargin quantity.Quantity

	blockWatcher indexer.BlockWatcher
	coreClient   core.V1
}

func New(ctx context.Context, cfg *conf.GasConfig, blockWatcher indexer.BlockWatcher, coreClient core.V1) Backend {
	ctxB, cancelCtx := context.WithCancel(ctx)

	windowSize := defaultWindowSize
	blockFullThreshold := defaultFullBlockThreshold
	minGasPrice := defaultGasPrice
	computedPriceMargin := defaultComputedPriceMargin
	if cfg != nil {
		if cfg.WindowSize != 0 {
			windowSize = cfg.WindowSize
		}
		if cfg.BlockFullThreshold != 0 {
			blockFullThreshold = cfg.BlockFullThreshold
		}
		if cfg.MinGasPrice != 0 {
			minGasPrice = *quantity.NewFromUint64(cfg.MinGasPrice)
		}
		if cfg.ComputedPriceMargin != 0 {
			computedPriceMargin = *quantity.NewFromUint64(cfg.ComputedPriceMargin)
		}
	}

	g := &gasPriceOracle{
		BaseBackgroundService: *service.NewBaseBackgroundService("gas-price-oracle"),
		ctx:                   ctxB,
		cancelCtx:             cancelCtx,
		feeHistoryData:        make([]*feeHistoryBlockData, 0, feeHistoryWindowSize),
		windowSize:            windowSize,
		fullBlockThreshold:    blockFullThreshold,
		defaultGasPrice:       minGasPrice,
		computedPriceMargin:   computedPriceMargin,
		blockWatcher:          blockWatcher,
		coreClient:            coreClient,
	}

	g.blockPrices = make([]*quantity.Quantity, windowSize)
	for i := range windowSize {
		g.blockPrices[i] = quantity.NewQuantity()
	}

	return g
}

// Start starts service.
func (g *gasPriceOracle) Start() error {
	go g.indexedBlockWatcher()

	return nil
}

// Stop stops service.
func (g *gasPriceOracle) Stop() {
	g.cancelCtx()
}

// Implements Backend.
func (g *gasPriceOracle) GasPrice() *hexutil.Big {
	g.priceLock.RLock()
	defer g.priceLock.RUnlock()

	if g.computedGasPrice == nil && g.nodeMinGasPrice == nil {
		// No blocks tracked yet and no min gas price from the node,
		// default to a default value.
		price := hexutil.Big(*g.defaultGasPrice.ToBigInt())
		return &price
	}

	// Set maxPrice to the larger of the `nodeMinGasPrice` and `computedGasPrice`.
	maxPrice := quantity.NewQuantity()
	if g.nodeMinGasPrice != nil {
		maxPrice = g.nodeMinGasPrice.Clone()
	}
	if g.computedGasPrice != nil && g.computedGasPrice.Cmp(maxPrice) > 0 {
		maxPrice = g.computedGasPrice.Clone()
	}

	price := hexutil.Big(*maxPrice.ToBigInt())
	return &price
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
	ch, sub, err := g.blockWatcher.WatchBlocks(g.ctx, int64(g.windowSize))
	if err != nil {
		g.Logger.Error("indexed block watcher failed to watch blocks", "err", err)
		return
	}
	defer sub.Close()

	// Guards against multiple concurrent queries in case web3 is catching up
	// with the chain.
	var queryLock sync.Mutex
	var queryInProgress bool

	for {
		select {
		case <-g.ctx.Done():
			return
		case blk := <-ch:
			// After every block fetch the reported min gas price from the node.
			// The returned price will reflect min price changes in the next block if
			// dynamic min gas price is enabled for the runtime.
			queryLock.Lock()
			if !queryInProgress {
				queryInProgress = true
				go func() {
					g.fetchMinGasPrice(g.ctx)
					queryLock.Lock()
					queryInProgress = false
					queryLock.Unlock()
				}()
			}
			queryLock.Unlock()

			// Track price for the block.
			g.onBlock(blk.Block, blk.MedianTransactionGasPrice)
			// Track fee history.
			g.trackFeeHistory(blk.Block, blk.UniqueTxes, blk.Receipts)
		}
	}
}

func (g *gasPriceOracle) onBlock(b *model.Block, medTxPrice *quantity.Quantity) {
	// Consider block full if block gas used is greater than `fullBlockThreshold` of gas limit.
	blockFull := (float64(b.Header.GasLimit) * g.fullBlockThreshold) <= float64(b.Header.GasUsed)
	if !blockFull {
		// Track 0 for non-full blocks.
		g.trackPrice(quantity.NewQuantity())
		return
	}

	if medTxPrice == nil {
		g.Logger.Error("no med tx gas price for block", "block", b)
		return
	}
	trackPrice := medTxPrice.Clone()
	if err := trackPrice.Add(&g.computedPriceMargin); err != nil {
		g.Logger.Error("failed to add minPriceEps to medTxPrice", "err", err)
	}
	g.trackPrice(trackPrice)
}

func (g *gasPriceOracle) trackPrice(price *quantity.Quantity) {
	g.blockPrices[g.blockPricesCurrentIdx] = price
	g.blockPricesCurrentIdx = (g.blockPricesCurrentIdx + 1) % int(g.windowSize)

	// Find maximum gas price.
	maxPrice := quantity.NewQuantity()
	for _, price := range g.blockPrices {
		if price.Cmp(maxPrice) > 0 {
			maxPrice = price
		}
	}

	reportedPrice := float64(maxPrice.ToBigInt().Int64())
	// No full blocks among last `windowSize` blocks.
	if maxPrice.IsZero() {
		maxPrice = nil
		reportedPrice = float64(-1)
	}

	g.priceLock.Lock()
	g.computedGasPrice = maxPrice
	g.priceLock.Unlock()
	metricComputedPrice.Set(reportedPrice)
}
