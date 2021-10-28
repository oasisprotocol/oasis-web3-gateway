package storage

import (
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
)

type Storage interface {
	// Store stores data.
	Store(value interface{}) error

	// Update updates record.
	Update(value interface{}) error

	// Delete deletes all records with round less than the given round.
	Delete(table interface{}, round uint64) error

	// GetBlockRound queries block round by block hash.
	GetBlockRound(hash string) (uint64, error)

	// GetBlockHash queries block hash by block round.
	GetBlockHash(round uint64) (string, error)

	// GetLatestBlockHash returns the block hash of the latest indexed round.
	GetLatestBlockHash() (string, error)

	// GetTransactionRef returns block hash, round and index of the transaction.
	GetTransactionRef(ethTxHash string) (*model.TransactionRef, error)

	// GetTransactionByRoundAndIndex queries ethereum transaction by round and index.
	GetTransactionByRoundAndIndex(round uint64, index uint32) (*model.Transaction, error)

	// GetContinuesIndexedRound query continues indexed block round.
	GetContinuesIndexedRound() (uint64, error)

	// GetTransaction queries ethereum transaction by hash.
	GetTransaction(hash string) (*model.Transaction, error)
}
