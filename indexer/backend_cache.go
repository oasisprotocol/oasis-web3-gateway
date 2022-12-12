package indexer

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/oasisprotocol/oasis-core/go/common/pubsub"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/oasisprotocol/oasis-web3-gateway/conf"
	"github.com/oasisprotocol/oasis-web3-gateway/db/model"
	"github.com/oasisprotocol/oasis-web3-gateway/storage"
)

const periodicMetricsInterval = 60 * time.Second

var (
	metricCacheHits = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "oasis_oasis_web3_gateway_cache_hits",
			Help: "Number of cache hits.",
		},
		[]string{"cache"},
	)
	metricCacheMisses = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "oasis_oasis_web3_gateway_cache_misses",
			Help: "Number of cache misses.",
		},
		[]string{"cache"},
	)
	metricCacheHitRatio = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "oasis_oasis_web3_gateway_cache_hit_ratio",
			Help: "Percent of Hits over all accesses (Hits + Misses).",
		},
		[]string{"cache"},
	)
)

type txCacheEntry struct {
	tx          *model.Transaction
	blockNumber uint64
}

type receiptCacheEntry struct {
	receipt     *model.Receipt
	blockNumber uint64
}

type cacheMetrics struct {
	hits   uint64
	misses uint64
}

func (m *cacheMetrics) OnHit() {
	atomic.AddUint64(&m.hits, 1)
}

func (m *cacheMetrics) OnMiss() {
	atomic.AddUint64(&m.misses, 1)
}

func (m *cacheMetrics) Accumulate(label string) {
	hits := atomic.SwapUint64(&m.hits, 0)
	misses := atomic.SwapUint64(&m.misses, 0)
	total := hits + misses

	ratio := float64(1.0)
	if fTotal := float64(total); fTotal > 0 {
		ratio = float64(hits) / fTotal
	}
	metricCacheHits.WithLabelValues(label).Set(float64(hits))
	metricCacheMisses.WithLabelValues(label).Set(float64(misses))
	metricCacheHitRatio.WithLabelValues(label).Set(ratio)
}

// cachingBackend is a Backend that interposes a cache above an existing
// backend.
type cachingBackend struct {
	inner Backend

	cacheSize    uint64
	maxCacheSize uint64

	blockDataByNumber sync.Map
	blockByHashHex    sync.Map
	logsByBlockNumber sync.Map

	txByHashHex        sync.Map
	receiptByTxHashHex sync.Map

	lastIndexedRound       uint64
	lastRetainedRound      uint64
	lastRetainedRoundValid uint64

	metricsBlockCache   cacheMetrics
	metricsTxCache      cacheMetrics
	metricsLogCache     cacheMetrics
	metricsReceiptCache cacheMetrics

	trackMetrics bool
}

// SetObserver sets the intrusive backend observer.
func (cb *cachingBackend) SetObserver(
	ob BackendObserver,
) {
	panic("indexer: caching backend does not support an observer")
}

func (cb *cachingBackend) OnBlockIndexed(
	bd *BlockData,
) {
	// A new block was indexed, insert it into the various caches.
	cb.blockDataByNumber.Store(bd.Block.Round, bd)
	cb.blockByHashHex.Store(bd.Block.Hash, bd.Block)
	cb.cacheLogsByBlockNumber(bd.Block.Round, bd.Receipts)

	cb.cacheTxes(bd.Block.Round, bd.UniqueTxes)
	cb.cacheReceipts(bd.Block.Round, bd.Receipts)
	cb.cacheSize++

	if bd.Block.Round > atomic.LoadUint64(&cb.lastIndexedRound) {
		atomic.StoreUint64(&cb.lastIndexedRound, bd.Block.Round)
	}

	// If the cache is larger than the max size after the insert, prune
	// the oldest rounds till the cache is at the maximum size again.
	if cb.cacheSize > cb.maxCacheSize {
		// Note: This is rather wasteful, but it can happen concurrently
		// with lookups to the newly cached data, and the cache doesn't
		// need to be all that large to service the vast majority of
		// requests.
		rounds := make([]uint64, 0, cb.cacheSize)
		cb.blockDataByNumber.Range(func(key, value interface{}) bool {
			rounds = append(rounds, key.(uint64))
			return true
		})
		sort.Slice(rounds, func(i, j int) bool {
			return rounds[i] < rounds[j]
		})
		for _, idx := range rounds {
			cb.pruneCache(idx)
			if cb.cacheSize <= cb.maxCacheSize {
				break
			}
		}
	}
}

func (cb *cachingBackend) OnLastRetainedRound(
	round uint64,
) {
	atomic.StoreUint64(&cb.lastRetainedRound, round)
}

func (cb *cachingBackend) Index(
	ctx context.Context,
	oasisBlock *block.Block,
	txResults []*client.TransactionWithResults,
	blockGasLimit uint64,
) error {
	return cb.inner.Index(ctx, oasisBlock, txResults, blockGasLimit)
}

func (cb *cachingBackend) Prune(
	ctx context.Context,
	round uint64,
) error {
	// Yes, this does not prune entries from the cache.  It would be
	// relatively expensive to do, serving data that has been pruned
	// from the backing store is harmless, and the cache has it's own
	// eviction policy.
	return cb.inner.Prune(ctx, round)
}

func (cb *cachingBackend) QueryBlockRound(
	ctx context.Context,
	blockHash ethcommon.Hash,
) (uint64, error) {
	blk, ok := cb.cachedBlockByHash(blockHash)
	if ok {
		return blk.Round, nil
	}

	return cb.inner.QueryBlockRound(ctx, blockHash)
}

func (cb *cachingBackend) QueryBlockHash(
	ctx context.Context,
	round uint64,
) (ethcommon.Hash, error) {
	blockNumber, err := cb.blockNumberFromRound(ctx, round)
	if err != nil {
		return ethcommon.Hash{}, err
	}
	blk, ok := cb.cachedBlockByNumber(blockNumber)
	if ok {
		return ethcommon.HexToHash(blk.Hash), nil
	}

	return cb.inner.QueryBlockHash(ctx, round)
}

func (cb *cachingBackend) QueryLastIndexedRound(
	ctx context.Context,
) (uint64, error) {
	lastIndexed := atomic.LoadUint64(&cb.lastIndexedRound)
	if lastIndexed != 0 {
		return lastIndexed, nil
	}

	// Don't do a CAS loop, this is updated by watching the indexer
	// process blocks, so this will always take the fast path unless
	// the indexer is not actively indexing at all.
	return cb.inner.QueryLastIndexedRound(ctx)
}

func (cb *cachingBackend) QueryLastRetainedRound(
	ctx context.Context,
) (uint64, error) {
	lastRetained := atomic.LoadUint64(&cb.lastRetainedRound)
	if atomic.LoadUint64(&cb.lastRetainedRoundValid) == 0 {
		// If Prune has never been called, this will be 0 with a success.
		// If the db is busted, this will be 0 with an error.  Do a CAS
		// loop so that it is possible to distinguish between the cases.
		var err error
		for {
			prevLastRetained := lastRetained
			lastRetained, err = cb.inner.QueryLastRetainedRound(ctx)
			if err != nil {
				return 0, err
			}

			if atomic.CompareAndSwapUint64(&cb.lastRetainedRound, prevLastRetained, lastRetained) {
				atomic.StoreUint64(&cb.lastRetainedRoundValid, 1)
				break
			}
		}
	}

	return lastRetained, nil
}

func (cb *cachingBackend) QueryTransaction(
	ctx context.Context,
	ethTxHash ethcommon.Hash,
) (*model.Transaction, error) {
	tx, ok := cb.cachedTxByHash(ethTxHash)
	if ok {
		return tx, nil
	}

	// This is updated by watching the indexer, as transactions can
	// get replaced under eth semantics.
	return cb.inner.QueryTransaction(ctx, ethTxHash)
}

func (cb *cachingBackend) GetBlockByRound(
	ctx context.Context,
	round uint64,
) (*model.Block, error) {
	blockNumber, err := cb.blockNumberFromRound(ctx, round)
	if err != nil {
		return nil, err
	}
	blk, ok := cb.cachedBlockByNumber(blockNumber)
	if ok {
		return blk, nil
	}

	return cb.inner.GetBlockByRound(ctx, blockNumber)
}

func (cb *cachingBackend) GetBlockByHash(
	ctx context.Context,
	blockHash ethcommon.Hash,
) (*model.Block, error) {
	blk, ok := cb.cachedBlockByHash(blockHash)
	if ok {
		return blk, nil
	}

	return cb.inner.GetBlockByHash(ctx, blockHash)
}

func (cb *cachingBackend) GetBlockTransactionCountByRound(
	ctx context.Context,
	round uint64,
) (int, error) {
	blockNumber, err := cb.blockNumberFromRound(ctx, round)
	if err != nil {
		return 0, err
	}
	blk, ok := cb.cachedBlockByNumber(blockNumber)
	if ok {
		return len(blk.Transactions), nil
	}

	return cb.inner.GetBlockTransactionCountByRound(ctx, blockNumber)
}

func (cb *cachingBackend) GetBlockTransactionCountByHash(
	ctx context.Context,
	blockHash ethcommon.Hash,
) (int, error) {
	blk, ok := cb.cachedBlockByHash(blockHash)
	if ok {
		return len(blk.Transactions), nil
	}

	return cb.inner.GetBlockTransactionCountByHash(ctx, blockHash)
}

func (cb *cachingBackend) GetTransactionByBlockHashAndIndex(
	ctx context.Context,
	blockHash ethcommon.Hash,
	txIndex int,
) (*model.Transaction, error) {
	blk, ok := cb.cachedBlockByHash(blockHash)
	if ok {
		l := len(blk.Transactions)
		switch {
		case l == 0:
			return nil, errors.New("the block doesn't have any transactions")
		case l-1 < txIndex:
			return nil, errors.New("index out of range")
		default:
			return blk.Transactions[txIndex], nil
		}
	}

	return cb.inner.GetTransactionByBlockHashAndIndex(ctx, blockHash, txIndex)
}

func (cb *cachingBackend) GetTransactionReceipt(
	ctx context.Context,
	txHash ethcommon.Hash,
) (map[string]interface{}, error) {
	if receipt, ok := cb.cachedReceiptByTxHash(txHash); ok {
		return db2EthReceipt(receipt), nil
	}
	return cb.inner.GetTransactionReceipt(ctx, txHash)
}

func (cb *cachingBackend) BlockNumber(
	ctx context.Context,
) (uint64, error) {
	// The underlying backend has a separate notion of BlockNumber and
	// LastIndexedRound for historical reasons, and even uses two separate
	// queries, but we can just use the last indexed round.
	lastIndexed := atomic.LoadUint64(&cb.lastIndexedRound)
	switch lastIndexed {
	case 0:
		// The storage backend just propagates the SQL error, but do better.
		return 0, storage.ErrNoRoundsIndexed
	default:
		return lastIndexed, nil
	}
}

func (cb *cachingBackend) GetLogs(
	ctx context.Context,
	startRound uint64,
	endRound uint64,
) ([]*model.Log, error) {
	// This uses BETWEEN, so inclusive on both ends.  Additionally,
	// only bother to attempt to service this from the cache iff the
	// range can possibly be in the cache.
	//
	// Queries for data that is too old, will bail after the first
	// cache lookup, so there is no need to examine the range itself.
	if reqSize := endRound - startRound + 1; reqSize < cb.maxCacheSize {
		logs := make([]*model.Log, 0, reqSize)
		var ok bool
		for i := startRound; i <= endRound; i++ {
			var blockLogs []*model.Log
			blockLogs, ok = cb.cachedLogsByNumber(i)
			if !ok {
				break
			}
			logs = append(logs, blockLogs...)
		}
		if ok {
			if cb.trackMetrics {
				cb.metricsLogCache.OnHit()
			}
			return logs, nil
		}
	}

	if cb.trackMetrics {
		cb.metricsLogCache.OnMiss()
	}
	return cb.inner.GetLogs(ctx, startRound, endRound)
}

func (cb *cachingBackend) WatchBlocks(ctx context.Context, buffer int64) (<-chan *BlockData, pubsub.ClosableSubscription, error) {
	return cb.inner.WatchBlocks(ctx, buffer)
}

func (cb *cachingBackend) pruneCache(
	blockNumber uint64,
) {
	bd, ok := cb.cachedBlockDataByNumber(blockNumber)
	if !ok {
		return
	}

	if bd.Block.Round != blockNumber {
		panic("indexer: cached entry block height != queried height")
	}

	cb.blockByHashHex.Delete(bd.Block.Hash)
	cb.logsByBlockNumber.Delete(bd.Block.Round)

	// Note: Load followed by delete is safe since the only routine that
	// mutates those caches is the caller.
	for i := range bd.UniqueTxes {
		txHash := bd.UniqueTxes[i].Hash
		if untypedTxEntry, ok := cb.txByHashHex.Load(txHash); ok {
			txEntry := untypedTxEntry.(*txCacheEntry)
			if txEntry.blockNumber == blockNumber {
				cb.txByHashHex.Delete(txHash)
			}
		}
	}
	for i := range bd.Receipts {
		txHash := bd.Receipts[i].TransactionHash
		if untypedReceiptEntry, ok := cb.receiptByTxHashHex.Load(txHash); ok {
			receiptEntry := untypedReceiptEntry.(*receiptCacheEntry)
			if receiptEntry.blockNumber == blockNumber {
				cb.receiptByTxHashHex.Delete(txHash)
			}
		}
	}

	cb.cacheSize--
}

func (cb *cachingBackend) cacheTxes(
	blockNumber uint64,
	txes []*model.Transaction,
) {
	for i := range txes {
		entry := &txCacheEntry{
			tx:          txes[i],
			blockNumber: blockNumber,
		}
		cb.txByHashHex.Store(entry.tx.Hash, entry)
	}
}

func (cb *cachingBackend) cacheReceipts(
	blockNumber uint64,
	receipts []*model.Receipt,
) {
	for i := range receipts {
		entry := &receiptCacheEntry{
			receipt:     receipts[i],
			blockNumber: blockNumber,
		}
		cb.receiptByTxHashHex.Store(receipts[i].TransactionHash, entry)
	}
}

func (cb *cachingBackend) cacheLogsByBlockNumber(
	blockNumber uint64,
	receipts []*model.Receipt,
) {
	var logs []*model.Log
	for i := range receipts {
		logs = append(logs, receipts[i].Logs...)
	}
	cb.logsByBlockNumber.Store(blockNumber, logs)
}

func (cb *cachingBackend) blockNumberFromRound(
	ctx context.Context,
	round uint64,
) (uint64, error) {
	// Duplicated so that we can used the cache.
	if round == client.RoundLatest {
		return cb.BlockNumber(ctx)
	}
	return round, nil
}

func (cb *cachingBackend) cachedBlockDataByNumber(
	blockNumber uint64,
) (*BlockData, bool) {
	untypedBlockData, ok := cb.blockDataByNumber.Load(blockNumber)
	if ok {
		return untypedBlockData.(*BlockData), true
	}
	return nil, false
}

func (cb *cachingBackend) cachedBlockByNumber(
	blockNumber uint64,
) (*model.Block, bool) {
	if entry, ok := cb.cachedBlockDataByNumber(blockNumber); ok {
		if cb.trackMetrics {
			cb.metricsBlockCache.OnHit()
		}
		return entry.Block, true
	}

	if cb.trackMetrics {
		cb.metricsBlockCache.OnMiss()
	}
	return nil, false
}

func (cb *cachingBackend) cachedBlockByHash(
	blockHash ethcommon.Hash,
) (*model.Block, bool) {
	untypedBlock, ok := cb.blockByHashHex.Load(blockHash.Hex())
	if ok {
		if cb.trackMetrics {
			cb.metricsBlockCache.OnHit()
		}
		return untypedBlock.(*model.Block), true
	}

	if cb.trackMetrics {
		cb.metricsBlockCache.OnMiss()
	}
	return nil, false
}

func (cb *cachingBackend) cachedTxByHash(
	ethTxHash ethcommon.Hash,
) (*model.Transaction, bool) {
	untypedEntry, ok := cb.txByHashHex.Load(ethTxHash.Hex())
	if ok {
		if cb.trackMetrics {
			cb.metricsTxCache.OnHit()
		}
		return untypedEntry.(*txCacheEntry).tx, true
	}

	if cb.trackMetrics {
		cb.metricsTxCache.OnMiss()
	}
	return nil, false
}

func (cb *cachingBackend) cachedReceiptByTxHash(
	ethTxHash ethcommon.Hash,
) (*model.Receipt, bool) {
	untypedEntry, ok := cb.receiptByTxHashHex.Load(ethTxHash.Hex())
	if ok {
		if cb.trackMetrics {
			cb.metricsReceiptCache.OnHit()
		}
		return untypedEntry.(*receiptCacheEntry).receipt, true
	}

	if cb.trackMetrics {
		cb.metricsReceiptCache.OnMiss()
	}
	return nil, false
}

func (cb *cachingBackend) cachedLogsByNumber(
	blockNumber uint64,
) ([]*model.Log, bool) {
	untypedLogs, ok := cb.logsByBlockNumber.Load(blockNumber)
	if ok {
		return untypedLogs.([]*model.Log), true
	}
	return nil, false
}

func (cb *cachingBackend) metricsWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(periodicMetricsInterval):
			cb.metricsBlockCache.Accumulate("blocks")
			cb.metricsTxCache.Accumulate("transactions")
			cb.metricsReceiptCache.Accumulate("transaction_receipts")
			cb.metricsLogCache.Accumulate("transaction_logs")
		}
	}
}

func newCachingBackend(
	ctx context.Context,
	backend Backend,
	cfg *conf.CacheConfig,
) Backend {
	const defaultBlockCacheSize = 128 // In blocks

	if cfg == nil {
		cfg = new(conf.CacheConfig)
	}
	if cfg.BlockSize == 0 {
		cfg.BlockSize = defaultBlockCacheSize
	}

	cb := &cachingBackend{
		inner:        backend,
		maxCacheSize: cfg.BlockSize,
		trackMetrics: cfg.Metrics,
	}

	cb.inner.SetObserver(cb)

	// Try to initialize the last indexed/retained rounds.  Failures
	// are ok.
	var err error
	cb.lastIndexedRound, _ = backend.QueryLastIndexedRound(ctx)
	if cb.lastRetainedRound, err = backend.QueryLastRetainedRound(ctx); err == nil {
		cb.lastRetainedRoundValid = 1
	}

	// TODO: This could warm up the caches by doing db queries.

	if cfg.Metrics {
		go cb.metricsWorker(ctx)
	}

	return cb
}
