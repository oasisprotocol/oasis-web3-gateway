package indexer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/ristretto"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/oasisprotocol/emerald-web3-gateway/conf"
	"github.com/oasisprotocol/emerald-web3-gateway/db/model"
	"github.com/oasisprotocol/emerald-web3-gateway/storage"
)

const periodicMetricsInterval = 60 * time.Second

var (
	// Cache hit metrics.
	metricCacheHits     = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "oasis_emerald_web3_gateway_cache_hits", Help: "Number of cache hits."}, []string{"cache"})
	metricCacheMisses   = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "oasis_emerald_web3_gateway_cache_misses", Help: "Number of cache misses."}, []string{"cache"})
	metricCacheHitRatio = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "oasis_emerald_web3_cache_hit_ratio", Help: "Percent of Hits over all accesses (Hits + Misses)."}, []string{"cache"})
	// Cache keys metrics.
	metricCacheKeysAdded   = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "oasis_emerald_web3_cache_keys_added", Help: "Number of times an item was added to the cache."}, []string{"cache"})
	metricCacheKeysUpdated = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "oasis_emerald_web3_cache_keys_updated", Help: "Number of times a cache item was updated."}, []string{"cache"})
	metricCacheKeysEvicted = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "oasis_emerald_web3_cache_keys_evicted", Help: "Total number of cache keys evicted."}, []string{"cache"})
	// Mean,Min,Max of item life expectancies.
	//
	// These metrics should be replaced by a single prometheus histogram (or a statistic) metric,
	// but there is no way to transform the underlying Histogram the cache uses to a prometheus histogram.
	metricCacheExpectancyMean = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "oasis_emerald_web3_cache_life_expectancy_mean", Help: "Mean item life expectancy in seconds."}, []string{"cache"})
	metricCacheExpectancyMin  = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "oasis_emerald_web3_cache_life_expectancy_min", Help: "Min item life expectancy in seconds."}, []string{"cache"})
	metricCacheExpectancyMax  = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "oasis_emerald_web3_cache_life_expectancy_max", Help: "Max item life expectancy in seconds."}, []string{"cache"})
)

// cachingBackend is a Backend that interposes a cache above an existing
// backend.
type cachingBackend struct {
	inner Backend

	blockByNumber      *ristretto.Cache
	blockByHashHex     sync.Map
	txByHashHex        *ristretto.Cache
	receiptByTxHashHex *ristretto.Cache
	logsByBlockNumber  sync.Map

	lastIndexedRound       uint64
	lastRetainedRound      uint64
	lastRetainedRoundValid uint64
}

// SetObserver sets the intrusive backend observer.
func (cb *cachingBackend) SetObserver(
	ob BackendObserver,
) {
	panic("indexer: caching backend does not support an observer")
}

func (cb *cachingBackend) OnBlockIndexed(
	blk *model.Block,
	upsertedTxes []*model.Transaction,
	upsertedReceipts []*model.Receipt,
) {
	// A new block was indexed, insert it into the cache.
	cb.cacheBlock(blk)

	// WARNING: Don't even think of de-duplicating the per-block
	// transaction data with the transactions in the tx cache.
	//
	// Transactions/receipts can get replaced and that would do
	// bad things in certain edge cases.
	for i := range upsertedTxes {
		cb.cacheTx(upsertedTxes[i])
	}
	for i := range upsertedReceipts {
		cb.cacheReceipt(upsertedReceipts[i])
	}
	cb.cacheLogsByBlockNumber(blk.Round, upsertedReceipts)

	if blk.Round > atomic.LoadUint64(&cb.lastIndexedRound) {
		atomic.StoreUint64(&cb.lastIndexedRound, blk.Round)
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

	if blk, err = cb.inner.GetBlockByRound(ctx, blockNumber); err == nil {
		cb.cacheBlock(blk)
	}
	return blk, err
}

func (cb *cachingBackend) GetBlockByHash(
	ctx context.Context,
	blockHash ethcommon.Hash,
) (*model.Block, error) {
	blk, ok := cb.cachedBlockByHash(blockHash)
	if ok {
		return blk, nil
	}

	var err error
	if blk, err = cb.inner.GetBlockByHash(ctx, blockHash); err == nil {
		cb.cacheBlock(blk)
	}
	return blk, err
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
	dbReceipt, ok := cb.cachedReceiptByTxHash(txHash)
	if ok {
		return db2EthReceipt(dbReceipt), nil
	}

	// This is updated by watching the indexer, as transactions (and
	// their receipts) can get replaced under eth semantics.
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
	const smallRangeMax = 16

	// This uses BETWEEN, so inclusive on both ends.  Additionally,
	// scanning the map is relatively expensive, so limit the range
	// we service from the cache to something "reasonable".
	if reqSize := endRound - startRound + 1; reqSize <= smallRangeMax {
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
			return logs, nil
		}
	}

	return cb.inner.GetLogs(ctx, startRound, endRound)
}

func (cb *cachingBackend) cacheBlock(
	blk *model.Block,
) {
	const costBlock = 1
	if cb.blockByNumber.Set(blk.Round, blk, costBlock) {
		cb.blockByHashHex.Store(blk.Hash, blk)
	}
}

func (cb *cachingBackend) cacheTx(
	tx *model.Transaction,
) {
	cb.txByHashHex.Set(tx.Hash, tx, int64(tx.Size()))
}

func (cb *cachingBackend) cacheReceipt(
	receipt *model.Receipt,
) {
	// TODO/yawning: At some point this should store the
	// map[string]interface{} instead to save some s11n overhead,
	// but calculating the cost annoying (especially accurately).
	cb.receiptByTxHashHex.Set(receipt.TransactionHash, receipt, int64(receipt.Size()))
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

func (cb *cachingBackend) cachedBlockByNumber(
	blockNumber uint64,
) (*model.Block, bool) {
	untypedBlk, ok := cb.blockByNumber.Get(blockNumber)
	if ok {
		return untypedBlk.(*model.Block), true
	}
	return nil, false
}

func (cb *cachingBackend) cachedBlockByHash(
	blockHash ethcommon.Hash,
) (*model.Block, bool) {
	untypedBlk, ok := cb.blockByHashHex.Load(blockHash.Hex())
	if ok {
		return untypedBlk.(*model.Block), true
	}
	return nil, false
}

func (cb *cachingBackend) cachedTxByHash(
	ethTxHash ethcommon.Hash,
) (*model.Transaction, bool) {
	untypedTx, ok := cb.txByHashHex.Get(ethTxHash.Hex())
	if ok {
		return untypedTx.(*model.Transaction), true
	}
	return nil, false
}

func (cb *cachingBackend) cachedReceiptByTxHash(
	ethTxHash ethcommon.Hash,
) (*model.Receipt, bool) {
	untypedReceipt, ok := cb.receiptByTxHashHex.Get(ethTxHash.Hex())
	if ok {
		return untypedReceipt.(*model.Receipt), true
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

func (cb *cachingBackend) onRejectOrEvictBlock(
	item *ristretto.Item,
) {
	blk := item.Value.(*model.Block)
	cb.blockByHashHex.Delete(blk.Hash)
	cb.logsByBlockNumber.Delete(blk.Round)
}

func collectPromMetrics(cache string, m *ristretto.Metrics) {
	// Hits.
	metricCacheHits.WithLabelValues(cache).Set(float64(m.Hits()))
	metricCacheMisses.WithLabelValues(cache).Set(float64(m.Misses()))
	metricCacheHitRatio.WithLabelValues(cache).Set(m.Ratio())

	// Keys.
	metricCacheKeysAdded.WithLabelValues(cache).Set(float64(m.KeysAdded()))
	metricCacheKeysUpdated.WithLabelValues(cache).Set(float64(m.KeysUpdated()))
	metricCacheKeysEvicted.WithLabelValues(cache).Set(float64(m.KeysEvicted()))

	// Life expectancy.
	expectancy := m.LifeExpectancySeconds()
	metricCacheExpectancyMean.WithLabelValues(cache).Set(expectancy.Mean())
	metricCacheExpectancyMin.WithLabelValues(cache).Set(float64(expectancy.Min))
	metricCacheExpectancyMax.WithLabelValues(cache).Set(float64(expectancy.Max))
}

func (cb *cachingBackend) metricsWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(periodicMetricsInterval):
			collectPromMetrics("blocks", cb.blockByNumber.Metrics)
			collectPromMetrics("transactions", cb.txByHashHex.Metrics)
			collectPromMetrics("transaction_receipts", cb.receiptByTxHashHex.Metrics)
		}
	}
}

func newCachingBackend(
	ctx context.Context,
	backend Backend,
	cfg *conf.CacheConfig,
) (Backend, error) {
	const (
		defaultBlockCacheSize   = 128                // In blocks
		defaultTxCacheSize      = 50 * 1024 * 1024   // In bytes
		defaultReceiptCacheSize = defaultTxCacheSize // In bytes

		bufferItems = 64 // Per documentation
	)

	if cfg == nil {
		cfg = new(conf.CacheConfig)
	}
	if cfg.BlockSize == 0 {
		cfg.BlockSize = defaultBlockCacheSize
	}
	if cfg.TxSize == 0 {
		cfg.TxSize = defaultTxCacheSize
	}
	if cfg.TxReceiptSize == 0 {
		cfg.TxReceiptSize = defaultReceiptCacheSize
	}

	cb := &cachingBackend{
		inner: backend,
	}

	// Block cache (by block number aka round), accounting in blocks.
	blockByNumber, err := ristretto.NewCache(&ristretto.Config{
		NumCounters:        int64(cfg.BlockSize * 10), // 10x MaxCost
		MaxCost:            int64(cfg.BlockSize),
		BufferItems:        bufferItems,
		OnEvict:            cb.onRejectOrEvictBlock,
		OnReject:           cb.onRejectOrEvictBlock,
		IgnoreInternalCost: true,
		Metrics:            cfg.Metrics,
	})
	if err != nil {
		return nil, fmt.Errorf("indexer: failed to create block by number cache: %w", err)
	}
	cb.blockByNumber = blockByNumber

	// Transaction cache (by transaction hash in hex), accounting in bytes.
	txByHashHex, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: int64(cfg.TxSize * 10),
		MaxCost:     int64(cfg.TxSize),
		BufferItems: bufferItems,
		Metrics:     cfg.Metrics,
	})
	if err != nil {
		return nil, fmt.Errorf("indexer: failed to create tx by hash cache: %w", err)
	}
	cb.txByHashHex = txByHashHex

	// Transaction receipt cache (by transaction hash in hex), accounting in bytes.
	receiptByTxHashHex, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: int64(cfg.TxReceiptSize * 10),
		MaxCost:     int64(cfg.TxReceiptSize),
		BufferItems: bufferItems,
		Metrics:     cfg.Metrics,
	})
	if err != nil {
		return nil, fmt.Errorf("indexer: failed to create receipt by tx hash cache: %w", err)
	}
	cb.receiptByTxHashHex = receiptByTxHashHex

	cb.inner.SetObserver(cb)

	// Try to initialize the last indexed/retained rounds.  Failures
	// are ok.
	cb.lastIndexedRound, _ = backend.QueryLastIndexedRound(ctx)
	if cb.lastRetainedRound, err = backend.QueryLastRetainedRound(ctx); err == nil {
		cb.lastRetainedRoundValid = 1
	}

	if cfg.Metrics {
		go cb.metricsWorker(ctx)
	}

	go func() {
		<-ctx.Done()
		blockByNumber.Close()
		txByHashHex.Close()
		receiptByTxHashHex.Close()
	}()

	return cb, nil
}
