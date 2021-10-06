package storage

import (
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
)

type Storage interface {
	// Store stores data.
	Store(value interface{}) error

	// GetBlockRound queries block round by block hash.
	GetBlockRound(hash string) (uint64, error)

	// GetBlockHash queries block hash by block round.
	GetBlockHash(round uint64) (string, error)

	// GetTxResult queries oasis tx result by ethereum tx hash.
	GetTxResult(hash string) (*model.TxResult, error)

	GetContinuesIndexedRound() (uint64, error)
}
