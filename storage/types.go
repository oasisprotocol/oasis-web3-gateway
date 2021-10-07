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

	// GetTransactionRoundAndIndex queries transaction round and index by transaction hash.
	GetTransactionRoundAndIndex(ethTxHash string) (uint64, uint32, error)

	// GetTransactionByRoundAndIndex queries ethereum transaction by round and index.
	GetTransactionByRoundAndIndex(round uint64, index uint32) (*model.EthTransaction, error)

	// GetContinuesIndexedRound query continues indexed block round.
	GetContinuesIndexedRound() (uint64, error)

	// GetEthTransaction queries ethereum transaction by hash.
	GetEthTransaction(hash string) (*model.EthTransaction, error)
}
