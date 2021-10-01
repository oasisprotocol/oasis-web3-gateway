package storage

import (
	"context"
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
)

type Storage interface {
	// Store stores data.
	Store(ctx context.Context, value interface{}) error

	// GetBlockRound queries block round by block hash.
	GetBlockRound(ctx context.Context, hash string) (uint64, error)

	// GetBlockHash queries block hash by block round.
	GetBlockHash(ctx context.Context, round uint64) (string, error)

	// GetTxResult queries oasis tx result by ethereum tx hash.
	GetTxResult(ctx context.Context, hash string) (*model.TxResult, error)
}
