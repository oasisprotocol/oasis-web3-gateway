package storage

import (
	"context"
	"fmt"

	"github.com/oasisprotocol/oasis-web3-gateway/db/model"
)

// ErrNoRoundsIndexed is the error returned by last indexed round when no rounds
// are indexed.
var ErrNoRoundsIndexed = fmt.Errorf("no rounds indexed")

type Storage interface {
	// Insert inserts a record. On conflict the insert errors.
	Insert(ctx context.Context, value interface{}) error

	// InsertIfNotExists inserts a record if a record with same primary key does not exist.
	InsertIfNotExists(ctx context.Context, value interface{}) error

	// Upsert upserts a record.
	Upsert(ctx context.Context, value interface{}) error

	// Delete deletes all records with round less than the given round.
	Delete(ctx context.Context, table interface{}, round uint64) error

	// GetBlockRound returns block round by block hash.
	GetBlockRound(ctx context.Context, hash string) (uint64, error)

	// GetBlockHash returns block hash by block round.
	GetBlockHash(ctx context.Context, round uint64) (string, error)

	// GetLatestBlockHash returns the block hash of the latest indexed round.
	GetLatestBlockHash(ctx context.Context) (string, error)

	// GetLastIndexedRound query the last indexed round.
	GetLastIndexedRound(ctx context.Context) (uint64, error)

	// GetLastRetainedRound query the minimum round not pruned.
	GetLastRetainedRound(ctx context.Context) (uint64, error)

	// GetTransaction queries ethereum transaction by hash.
	GetTransaction(ctx context.Context, hash string) (*model.Transaction, error)

	// GetLatestBlockNumber returns the latest block round.
	GetLatestBlockNumber(ctx context.Context) (uint64, error)

	// GetBlockByHash returns the block for the given hash.
	GetBlockByHash(ctx context.Context, hash string) (*model.Block, error)

	// GetBlockByNumber returns the block for the given round.
	GetBlockByNumber(ctx context.Context, round uint64) (*model.Block, error)

	// GetBlockTransactionCountByNumber returns the transaction count of the block.
	GetBlockTransactionCountByNumber(ctx context.Context, round uint64) (int, error)

	// GetBlockTransactionCountByHash returns the transaction count of the block.
	GetBlockTransactionCountByHash(ctx context.Context, hash string) (int, error)

	// GetBlockTransaction returns the transaction of the block for the given index.
	GetBlockTransaction(ctx context.Context, blockHash string, txIndex int) (*model.Transaction, error)

	// GetTransactionReceipt returns the receipt of the transaction.
	GetTransactionReceipt(ctx context.Context, txHash string) (*model.Receipt, error)

	// GetLogs returns all logs between start and end rounds.
	GetLogs(ctx context.Context, startRound, endRound uint64) ([]*model.Log, error)

	// RunInTransaction runs a function in a transaction. If function
	// returns an error transaction is rolled back, otherwise transaction
	// is committed.
	RunInTransaction(ctx context.Context, fn func(Storage) error) error
}
