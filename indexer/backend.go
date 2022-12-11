package indexer

import (
	"context"
	"errors"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/oasisprotocol/oasis-core/go/common"
	"github.com/oasisprotocol/oasis-core/go/common/crypto/hash"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-core/go/common/pubsub"
	"github.com/oasisprotocol/oasis-core/go/common/quantity"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"

	"github.com/oasisprotocol/oasis-web3-gateway/db/model"
	"github.com/oasisprotocol/oasis-web3-gateway/filters"
	"github.com/oasisprotocol/oasis-web3-gateway/storage"
)

var ErrGetLastRetainedRound = errors.New("get last retained round error in db")

// Result is a query result.
type Result struct {
	// TxHash is the hash of the matched transaction.
	TxHash hash.Hash
	// TxIndex is the index of the matched transaction within the block.
	TxIndex uint32
}

// Results are query results.
//
// Map key is the round number and value is a list of transaction hashes
// that match the query.
type Results map[uint64][]Result

// QueryableBackend is the read-only indexer backend interface.
type QueryableBackend interface {
	// QueryBlockRound queries block round by block hash.
	QueryBlockRound(ctx context.Context, blockHash ethcommon.Hash) (uint64, error)

	// QueryBlockHash queries block hash by round.
	QueryBlockHash(ctx context.Context, round uint64) (ethcommon.Hash, error)

	// QueryLastIndexedRound query continues indexed block round.
	QueryLastIndexedRound(ctx context.Context) (uint64, error)

	// QueryLastRetainedRound query the minimum round not pruned.
	QueryLastRetainedRound(ctx context.Context) (uint64, error)

	// QueryTransaction queries ethereum transaction by hash.
	QueryTransaction(ctx context.Context, ethTxHash ethcommon.Hash) (*model.Transaction, error)
}

// GetEthInfoBackend is a backend for handling ethereum data.
type GetEthInfoBackend interface {
	GetBlockByRound(ctx context.Context, round uint64) (*model.Block, error)
	GetBlockByHash(ctx context.Context, blockHash ethcommon.Hash) (*model.Block, error)
	GetBlockTransactionCountByRound(ctx context.Context, round uint64) (int, error)
	GetBlockTransactionCountByHash(ctx context.Context, blockHash ethcommon.Hash) (int, error)
	GetTransactionByBlockHashAndIndex(ctx context.Context, blockHash ethcommon.Hash, txIndex int) (*model.Transaction, error)
	GetTransactionReceipt(ctx context.Context, txHash ethcommon.Hash) (map[string]interface{}, error)
	BlockNumber(ctx context.Context) (uint64, error)
	GetLogs(ctx context.Context, startRound, endRound uint64) ([]*model.Log, error)
}

// BlockWatcher is the block-watcher interface.
type BlockWatcher interface {
	// WatchBlocks returns a channel that produces a stream of indexed blocks.
	WatchBlocks(ctx context.Context, buffer int64) (<-chan *BlockData, pubsub.ClosableSubscription, error)
}

// Backend is the indexer backend interface.
type Backend interface {
	QueryableBackend
	GetEthInfoBackend
	BlockWatcher

	// Index indexes a block.
	Index(
		ctx context.Context,
		oasisBlock *block.Block,
		txResults []*client.TransactionWithResults,
		blockGasLimit uint64,
	) error

	// Prune removes indexed data for rounds equal to or earlier than the passed round.
	Prune(ctx context.Context, round uint64) error

	// SetObserver sets the intrusive backend observer.
	SetObserver(BackendObserver)
}

// BlockData contains all per block indexed data.
type BlockData struct {
	Block      *model.Block
	Receipts   []*model.Receipt
	UniqueTxes []*model.Transaction
	// LastTransactionPrice is the price of the last transaction in the runtime block in base units.
	// This can be different than the price of the last transaction in the `BlockData.Block`
	// as `BlockData.Block` contains only EVM transactions.
	// When https://github.com/oasisprotocol/oasis-web3-gateway/issues/84 is implemented
	// this will need to be persisted in the DB, so that instances without the indexer can
	// obtain this as well.
	LastTransactionPrice *quantity.Quantity
}

// BackendObserver is the intrusive backend observer interaface.
// TODO: BlockObserver could be replaced by using BlockWatcher.
type BackendObserver interface {
	OnBlockIndexed(*BlockData)
	OnLastRetainedRound(uint64)
}

type indexBackend struct {
	runtimeID     common.Namespace
	logger        *logging.Logger
	storage       storage.Storage
	blockNotifier *pubsub.Broker

	// TODO: "filter subscriber" can probably be migrated to using BlockWatcher.
	subscribe filters.SubscribeBackend
	// TODO: observer can probably be migrated to using BlockWatcher.
	observer BackendObserver
}

// SetObserver sets the intrusive backend observer.
func (ib *indexBackend) SetObserver(ob BackendObserver) {
	ib.observer = ob
}

// Index indexes oasis block.
func (ib *indexBackend) Index(ctx context.Context, oasisBlock *block.Block, txResults []*client.TransactionWithResults, blockGasLimit uint64) error {
	round := oasisBlock.Header.Round

	err := ib.StoreBlockData(ctx, oasisBlock, txResults, blockGasLimit)
	if err != nil {
		ib.logger.Error("generateEthBlock failed", "err", err)
		return err
	}

	ib.logger.Info("indexed block", "round", round)

	return nil
}

// Prune prunes data in db.
func (ib *indexBackend) Prune(ctx context.Context, round uint64) error {
	return ib.storage.RunInTransaction(ctx, func(s storage.Storage) error {
		if err := ib.storeLastRetainedRound(ctx, round); err != nil {
			return err
		}

		if err := ib.storage.Delete(ctx, new(model.Block), round); err != nil {
			return err
		}

		if err := ib.storage.Delete(ctx, new(model.Log), round); err != nil {
			return err
		}

		if err := ib.storage.Delete(ctx, new(model.Transaction), round); err != nil {
			return err
		}

		return ib.storage.Delete(ctx, new(model.Receipt), round)
	})
}

func (ib *indexBackend) WatchBlocks(ctx context.Context, buffer int64) (<-chan *BlockData, pubsub.ClosableSubscription, error) {
	typedCh := make(chan *BlockData)
	sub := ib.blockNotifier.SubscribeBuffered(buffer)
	sub.Unwrap(typedCh)

	return typedCh, sub, nil
}

// blockNumberFromRound converts a round to a blocknumber.
func (ib *indexBackend) blockNumberFromRound(ctx context.Context, round uint64) (number uint64, err error) {
	switch round {
	case client.RoundLatest:
		number, err = ib.BlockNumber(ctx)
	default:
		number = round
	}
	return
}

// QueryBlockRound returns block number for the provided hash.
func (ib *indexBackend) QueryBlockRound(ctx context.Context, blockHash ethcommon.Hash) (uint64, error) {
	round, err := ib.storage.GetBlockRound(ctx, blockHash.String())
	if err != nil {
		ib.logger.Error("Can't find matched block")
		return 0, err
	}

	return round, nil
}

// QueryBlockHash returns the block hash for the provided round.
func (ib *indexBackend) QueryBlockHash(ctx context.Context, round uint64) (ethcommon.Hash, error) {
	var blockHash string
	var err error
	switch round {
	case client.RoundLatest:
		blockHash, err = ib.storage.GetLatestBlockHash(ctx)
	default:
		blockHash, err = ib.storage.GetBlockHash(ctx, round)
	}

	if err != nil {
		ib.logger.Error("failed to query block hash", "err", err)
		return ethcommon.Hash{}, err
	}
	return ethcommon.HexToHash(blockHash), nil
}

// QueryLastIndexedRound returns the last indexed round.
func (ib *indexBackend) QueryLastIndexedRound(ctx context.Context) (uint64, error) {
	indexedRound, err := ib.storage.GetLastIndexedRound(ctx)
	if err != nil {
		return 0, err
	}

	return indexedRound, nil
}

// storeLastRetainedRound stores the last retained round.
func (ib *indexBackend) storeLastRetainedRound(ctx context.Context, round uint64) error {
	r := &model.IndexedRoundWithTip{
		Tip:   model.LastRetained,
		Round: round,
	}

	return ib.storage.Upsert(ctx, r)
}

// QueryLastRetainedRound returns the last retained round.
func (ib *indexBackend) QueryLastRetainedRound(ctx context.Context) (uint64, error) {
	lastRetainedRound, err := ib.storage.GetLastRetainedRound(ctx)
	if err != nil {
		return 0, ErrGetLastRetainedRound
	}
	return lastRetainedRound, nil
}

// QueryTransaction returns transaction by transaction hash.
func (ib *indexBackend) QueryTransaction(ctx context.Context, txHash ethcommon.Hash) (*model.Transaction, error) {
	tx, err := ib.storage.GetTransaction(ctx, txHash.String())
	if err != nil {
		return nil, err
	}
	return tx, nil
}

// GetBlockByRound returns a block for the provided round.
func (ib *indexBackend) GetBlockByRound(ctx context.Context, round uint64) (*model.Block, error) {
	blockNumber, err := ib.blockNumberFromRound(ctx, round)
	if err != nil {
		return nil, err
	}
	blk, err := ib.storage.GetBlockByNumber(ctx, blockNumber)
	if err != nil {
		return nil, err
	}

	return blk, nil
}

// GetBlockByHash returns a block by bock hash.
func (ib *indexBackend) GetBlockByHash(ctx context.Context, blockHash ethcommon.Hash) (*model.Block, error) {
	blk, err := ib.storage.GetBlockByHash(ctx, blockHash.String())
	if err != nil {
		return nil, err
	}

	return blk, nil
}

// GetBlockTransactionCountByRound returns the count of block transactions for the provided round.
func (ib *indexBackend) GetBlockTransactionCountByRound(ctx context.Context, round uint64) (int, error) {
	blockNumber, err := ib.blockNumberFromRound(ctx, round)
	if err != nil {
		return 0, err
	}
	return ib.storage.GetBlockTransactionCountByNumber(ctx, blockNumber)
}

// GetBlockTransactionCountByHash returns the count of block transactions by block hash.
func (ib *indexBackend) GetBlockTransactionCountByHash(ctx context.Context, blockHash ethcommon.Hash) (int, error) {
	return ib.storage.GetBlockTransactionCountByHash(ctx, blockHash.String())
}

// GetTransactionByBlockHashAndIndex returns transaction by the block hash and transaction index.
func (ib *indexBackend) GetTransactionByBlockHashAndIndex(ctx context.Context, blockHash ethcommon.Hash, txIndex int) (*model.Transaction, error) {
	return ib.storage.GetBlockTransaction(ctx, blockHash.String(), txIndex)
}

// GetTransactionReceipt returns the receipt for the given tx.
func (ib *indexBackend) GetTransactionReceipt(ctx context.Context, txHash ethcommon.Hash) (map[string]interface{}, error) {
	dbReceipt, err := ib.storage.GetTransactionReceipt(ctx, txHash.String())
	if err != nil {
		return nil, err
	}

	return db2EthReceipt(dbReceipt), nil
}

// BlockNumber returns the latest block.
func (ib *indexBackend) BlockNumber(ctx context.Context) (uint64, error) {
	return ib.storage.GetLatestBlockNumber(ctx)
}

// GetLogs returns logs from db.
func (ib *indexBackend) GetLogs(ctx context.Context, startRound, endRound uint64) ([]*model.Log, error) {
	return ib.storage.GetLogs(ctx, startRound, endRound)
}

// NewIndexBackend returns a new index backend.
func NewIndexBackend(runtimeID common.Namespace, storage storage.Storage, sb filters.SubscribeBackend) Backend {
	b := &indexBackend{
		runtimeID:     runtimeID,
		logger:        logging.GetLogger("indexer"),
		storage:       storage,
		subscribe:     sb,
		blockNotifier: pubsub.NewBroker(false),
	}

	b.logger.Info("New indexer backend")

	return b
}
