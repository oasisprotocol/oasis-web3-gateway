package indexer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/dgraph-io/ristretto"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"

	"github.com/oasisprotocol/emerald-web3-gateway/db/model"
	"github.com/oasisprotocol/emerald-web3-gateway/storage"
)

const costBlock = 1

// cachingBackend is a Backend that interposes a cache above an existing
// backend.
type cachingBackend struct {
	inner Backend

	blockByNumber  *ristretto.Cache
	blockByHashHex sync.Map

	lastIndexedRound  uint64
	lastRetainedRound uint64
}

// SetObserver sets the intrusive backend observer.
func (cb *cachingBackend) SetObserver(
	ob BackendObserver,
) {
	panic("indexer: caching backend does not support an observer")
}

func (cb *cachingBackend) OnBlockIndexed(
	blk *model.Block,
) {
	// A new block was indexed, insert it into the cache.
	cb.cacheBlock(blk)

	// The only place this is updated is here, so we can freely do a
	// non-atomic read.
	if blk.Round > cb.lastIndexedRound {
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
	// process blocks.
	return cb.inner.QueryLastIndexedRound(ctx)
}

func (cb *cachingBackend) QueryLastRetainedRound(
	ctx context.Context,
) (uint64, error) {
	lastRetained := atomic.LoadUint64(&cb.lastRetainedRound)
	if lastRetained != 0 {
		return lastRetained, nil
	}

	var err error
	for {
		prevLastRetained := lastRetained
		lastRetained, err = cb.inner.QueryLastRetainedRound(ctx)
		if err != nil {
			return 0, err
		}

		if atomic.CompareAndSwapUint64(&cb.lastRetainedRound, prevLastRetained, lastRetained) {
			break
		}
	}

	return lastRetained, nil
}

func (cb *cachingBackend) QueryTransaction(
	ctx context.Context,
	ethTxHash ethcommon.Hash,
) (*model.Transaction, error) {
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
	return cb.inner.GetTransactionReceipt(ctx, txHash)
}

func (cb *cachingBackend) BlockNumber(
	ctx context.Context,
) (uint64, error) {
	// XXX/yawning: I assume that this can just be the last indexed round?
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
	return cb.inner.GetLogs(ctx, startRound, endRound)
}

func (cb *cachingBackend) cacheBlock(
	blk *model.Block,
) {
	if cb.blockByNumber.Set(blk.Round, blk, costBlock) {
		cb.blockByHashHex.Store(blk.Hash, blk)
	}
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

func (cb *cachingBackend) onRejectOrEvictBlock(
	item *ristretto.Item,
) {
	blk := item.Value.(*model.Block)
	cb.blockByHashHex.Delete(blk.Hash)
}

func newCachingBackend(
	ctx context.Context,
	backend Backend,
) (Backend, error) {
	cb := &cachingBackend{
		inner: backend,
	}

	blockByNumber, err := ristretto.NewCache(&ristretto.Config{
		NumCounters:        1024 * 10, // 10x MaxCost
		MaxCost:            1024,      // TODO: Tune
		BufferItems:        64,        // Per documentation
		OnEvict:            cb.onRejectOrEvictBlock,
		OnReject:           cb.onRejectOrEvictBlock,
		IgnoreInternalCost: true,
	})
	if err != nil {
		return nil, fmt.Errorf("indexer: failed to create block by number cache: %w", err)
	}
	cb.blockByNumber = blockByNumber

	cb.inner.SetObserver(cb)

	// Try to initialize the last indexed/retained rounds.  Failures
	// are ok.
	cb.lastIndexedRound, _ = backend.QueryLastIndexedRound(ctx)
	cb.lastRetainedRound, _ = backend.QueryLastRetainedRound(ctx)

	go func() {
		<-ctx.Done()
		blockByNumber.Close()
	}()

	return cb, nil
}
