package storage

import (
	"context"
	"database/sql"
	"github.com/oasisprotocol/oasis-core/go/common/crypto/hash"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
)

type Query interface {
	// GetBlockRoundByHash queries block round by block hash.
	GetBlockRoundByHash(ctx context.Context, blockHash hash.Hash) (uint64, error)

	// GetBlockByHash queries block by block hash.
	GetBlockByHash(ctx context.Context, blockHash hash.Hash) (*block.Block, error)

	// GetBlockHashByRound queries block hash by round.
	GetBlockHashByRound(ctx context.Context, round uint64) (hash.Hash, error)

	// QueryBlockByRound queries block by round.
	QueryBlockByRound(ctx context.Context, round uint64) (*block.Block, error)

	// QueryTxByIndex queries tx by block round and index.
	QueryTxByIndex(ctx context.Context, round uint64, index uint32) (hash.Hash, error)
}

type Store interface {
	// StoreBlock stores block hash, round and block.
	StoreBlock(ctx context.Context, blockHash hash.Hash, round uint64, block *block.Block) error

	// StoreTx stores tx hash, round and tx index.
	StoreTx(ctx context.Context, txHash hash.Hash, round uint64, index uint32) error
}

type Storage interface {
	Query
	Store
}

type PostDb struct {
	Db *sql.DB
}
