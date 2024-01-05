// Package gas implements a gas price oracle.
package gas

import (
	"context"
	"math"
	"math/big"
	"slices"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethMath "github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/oasisprotocol/oasis-core/go/common/quantity"
	"github.com/oasisprotocol/oasis-core/go/common/service"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"

	"github.com/oasisprotocol/oasis-web3-gateway/db/model"
	"github.com/oasisprotocol/oasis-web3-gateway/indexer"
)

var (
	metricNodeMinPrice  = promauto.NewGauge(prometheus.GaugeOpts{Name: "oasis_oasis_web3_gateway_gas_orcale_node_min_price", Help: "Min gas price periodically queried from the node."})
	metricComputedPrice = promauto.NewGauge(prometheus.GaugeOpts{Name: "oasis_oasis_web3_gateway_gas_oracle_computed_price", Help: "Computed recommended gas price based on recent full blocks. -1 if none (no recent full blocks)."})
)

// FeeHistoryResult is the result of a fee history query.
type FeeHistoryResult struct {
	OldestBlock  *hexutil.Big     `json:"oldestBlock"`
	Reward       [][]*hexutil.Big `json:"reward,omitempty"`
	BaseFee      []*hexutil.Big   `json:"baseFeePerGas,omitempty"`
	GasUsedRatio []float64        `json:"gasUsedRatio"`
}

// Backend is the gas price oracle backend.
type Backend interface {
	service.BackgroundService

	// GasPrice returns the currently recommended minimum gas price.
	GasPrice() *hexutil.Big

	// FeeHistory returns the fee history for the given blocks and percentiles.
	FeeHistory(blockCount uint64, lastBlock rpc.BlockNumber, percentiles []float64) *FeeHistoryResult
}

const (
	// windowSize is the number of recent blocks to use for calculating min gas price.
	// NOTE: code assumes that this is relatively small.
	windowSize = 12

	// feeHistoryWindowSize is the number of recent blocks to store for the fee history query.
	feeHistoryWindowSize = 20

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

type txGasAndReward struct {
	gasUsed uint64
	reward  *hexutil.Big
}

type feeHistoryData struct {
	height       uint64
	baseFee      *hexutil.Big
	gasUsedRatio float64
	gasUsed      uint64

	// Sorted list of transaction gas prices and rewards. This is used to
	// compute the provided percentiles.
	sortedTxs []*txGasAndReward
}

// rewards computes the reward percentiles for the given block data.
func (d *feeHistoryData) rewards(percentiles []float64) []*hexutil.Big {
	rewards := make([]*hexutil.Big, len(percentiles))

	if len(percentiles) == 0 {
		// No percentiles requested.
		return rewards
	}
	if len(d.sortedTxs) == 0 {
		// All zeros if there are no transactions in the block.
		for i := range rewards {
			rewards[i] = (*hexutil.Big)(common.Big0)
		}
		return rewards
	}

	// Compute the requested percentiles.
	var txIndex int
	sumGasUsed := d.sortedTxs[0].gasUsed
	for i, p := range percentiles {
		thresholdGasUsed := uint64(float64(d.gasUsed) * p / 100)
		for sumGasUsed < thresholdGasUsed && txIndex < len(d.sortedTxs)-1 {
			txIndex++
			sumGasUsed += d.sortedTxs[txIndex].gasUsed
		}
		rewards[i] = d.sortedTxs[txIndex].reward
	}

	return rewards
}

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

	// protects feeHistoryData.
	feeHistoryLock sync.RWMutex
	// feeHistoryData contains the data needed to compute the fee history for the
	// last `feeHistoryWindowSize` blocks.
	feeHistoryData []*feeHistoryData

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
		feeHistoryData:        make([]*feeHistoryData, 0, feeHistoryWindowSize),
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

func (g *gasPriceOracle) FeeHistory(blockCount uint64, lastBlock rpc.BlockNumber, percentiles []float64) *FeeHistoryResult {
	g.feeHistoryLock.RLock()
	defer g.feeHistoryLock.RUnlock()

	if len(g.feeHistoryData) == 0 {
		return &FeeHistoryResult{OldestBlock: (*hexutil.Big)(common.Big0)}
	}

	var lastBlockIdx int
	switch lastBlock {
	case rpc.PendingBlockNumber, rpc.LatestBlockNumber, rpc.FinalizedBlockNumber, rpc.SafeBlockNumber:
		// Take the latest available block.
		lastBlockIdx = len(g.feeHistoryData) - 1
	case rpc.EarliestBlockNumber:
		// Doesn't make sense to start at earliest block.
		return &FeeHistoryResult{OldestBlock: (*hexutil.Big)(common.Big0)}
	default:
		// Check if the requested block number is available.
		var found bool
		for i, d := range g.feeHistoryData {
			if d.height == uint64(lastBlock) {
				lastBlockIdx = i
				found = true
				break
			}
		}
		// Data for requested block number not available.
		if !found {
			return &FeeHistoryResult{OldestBlock: (*hexutil.Big)(common.Big0)}
		}
	}

	// Compute the oldest block to return.
	var oldestBlockIdx int
	if blockCount > uint64(lastBlockIdx) {
		// Not enough blocks available, return all available blocks.
		oldestBlockIdx = 0
	} else {
		oldestBlockIdx = lastBlockIdx + 1 - int(blockCount)
	}

	// Return the requested fee history.
	result := &FeeHistoryResult{
		OldestBlock:  (*hexutil.Big)(big.NewInt(int64(g.feeHistoryData[oldestBlockIdx].height))),
		Reward:       make([][]*hexutil.Big, lastBlockIdx-oldestBlockIdx+1),
		BaseFee:      make([]*hexutil.Big, lastBlockIdx-oldestBlockIdx+1),
		GasUsedRatio: make([]float64, lastBlockIdx-oldestBlockIdx+1),
	}
	for i := oldestBlockIdx; i <= lastBlockIdx; i++ {
		result.Reward[i-oldestBlockIdx] = g.feeHistoryData[i].rewards(percentiles)
		result.BaseFee[i-oldestBlockIdx] = g.feeHistoryData[i].baseFee
		result.GasUsedRatio[i-oldestBlockIdx] = g.feeHistoryData[i].gasUsedRatio
	}

	return result
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
			g.trackMinPrice(blk.Block, blk.LastTransactionPrice)
			g.trackFeeHistory(blk.Block, blk.UniqueTxes, blk.Receipts)
		}
	}
}

func (g *gasPriceOracle) trackMinPrice(b *model.Block, lastTxPrice *quantity.Quantity) {
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

func (g *gasPriceOracle) trackFeeHistory(block *model.Block, txs []*model.Transaction, receipts []*model.Receipt) {
	// TODO: could populate old blocks on first received block (if available).

	d := &feeHistoryData{
		height: block.Round,
		// Base fee is always zero.
		// https://github.com/oasisprotocol/oasis-sdk/blob/da9d86d52abca27930ec4e63c7735ca30f2f16b8/runtime-sdk/modules/evm/src/raw_tx.rs#L102-L105
		baseFee:      (*hexutil.Big)(common.Big0),
		gasUsed:      block.Header.GasUsed,
		gasUsedRatio: float64(block.Header.GasUsed) / float64(block.Header.GasLimit),
		sortedTxs:    make([]*txGasAndReward, len(receipts)),
	}
	for i, tx := range txs {
		var tipGas, feeCap big.Int
		if err := feeCap.UnmarshalText([]byte(tx.GasFeeCap)); err != nil {
			g.Logger.Error("unmarshal gas fee cap", "fee_cap", tx.GasFeeCap, "block", block, "tx", tx, "err", err)
			return
		}
		if err := tipGas.UnmarshalText([]byte(tx.GasTipCap)); err != nil {
			g.Logger.Error("unmarshal gas tip cap", "tip_cap", tx.GasTipCap, "block", block, "tx", tx, "err", err)
			return
		}
		d.sortedTxs[i] = &txGasAndReward{
			gasUsed: receipts[i].GasUsed,
			reward:  (*hexutil.Big)(ethMath.BigMin(&tipGas, &feeCap)),
		}
	}
	slices.SortStableFunc(d.sortedTxs, func(a, b *txGasAndReward) int {
		return a.reward.ToInt().Cmp(b.reward.ToInt())
	})

	// Add new data to the history.
	g.feeHistoryLock.Lock()
	defer g.feeHistoryLock.Unlock()

	// Delete oldest entry if we are at capacity.
	if len(g.feeHistoryData) == feeHistoryWindowSize {
		g.feeHistoryData = g.feeHistoryData[1:]
	}
	g.feeHistoryData = append(g.feeHistoryData, d)
}
