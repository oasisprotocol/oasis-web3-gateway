package gas

import (
	"math/big"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethMath "github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/oasisprotocol/oasis-web3-gateway/db/model"
)

// feeHistoryWindowSize is the number of recent blocks to store for the fee history query.
const feeHistoryWindowSize = 20

// FeeHistoryResult is the result of a fee history query.
type FeeHistoryResult struct {
	OldestBlock  *hexutil.Big     `json:"oldestBlock"`
	Reward       [][]*hexutil.Big `json:"reward,omitempty"`
	BaseFee      []*hexutil.Big   `json:"baseFeePerGas,omitempty"`
	GasUsedRatio []float64        `json:"gasUsedRatio"`
}

type txGasAndReward struct {
	gasUsed uint64
	reward  *hexutil.Big
}

type feeHistoryBlockData struct {
	height       uint64
	baseFee      *hexutil.Big
	gasUsedRatio float64
	gasUsed      uint64

	// Sorted list of transaction by reward. This is used to
	// compute the queried percentiles in the fee history query.
	sortedTxs []*txGasAndReward
}

// rewards computes the queried reward percentiles for the given block data.
func (d *feeHistoryBlockData) rewards(percentiles []float64) []*hexutil.Big {
	rewards := make([]*hexutil.Big, len(percentiles))

	// No percentiles requested.
	if len(percentiles) == 0 {
		return rewards
	}

	// No transactions in the block.
	if len(d.sortedTxs) == 0 {
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

// Implements Backend.
func (g *gasPriceOracle) FeeHistory(blockCount uint64, lastBlock rpc.BlockNumber, percentiles []float64) *FeeHistoryResult {
	g.feeHistoryLock.RLock()
	defer g.feeHistoryLock.RUnlock()

	// No history available.
	if len(g.feeHistoryData) == 0 {
		return &FeeHistoryResult{OldestBlock: (*hexutil.Big)(common.Big0)}
	}

	// Find the latest block index.
	var lastBlockIdx int
	switch lastBlock {
	case rpc.PendingBlockNumber, rpc.LatestBlockNumber, rpc.FinalizedBlockNumber, rpc.SafeBlockNumber:
		// Latest available block.
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

	// Find the oldest block index.
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

func (g *gasPriceOracle) trackFeeHistory(block *model.Block, txs []*model.Transaction, receipts []*model.Receipt) {
	// TODO: could populate old blocks on first received block (if available).

	d := &feeHistoryBlockData{
		height:       block.Round,
		gasUsed:      block.Header.GasUsed,
		gasUsedRatio: float64(block.Header.GasUsed) / float64(block.Header.GasLimit),
		sortedTxs:    make([]*txGasAndReward, len(receipts)),
	}

	// Base fee.
	var baseFee big.Int
	if err := baseFee.UnmarshalText([]byte(block.Header.BaseFee)); err != nil {
		g.Logger.Error("unmarshal base fee", "base_fee", block.Header.BaseFee, "block", block, "err", err)
		return
	}
	d.baseFee = (*hexutil.Big)(&baseFee)

	// Transactions.
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
