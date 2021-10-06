package indexer

import (
	"encoding/hex"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/oasisprotocol/oasis-core/go/common"
	"github.com/oasisprotocol/oasis-core/go/common/crypto/hash"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
	"github.com/starfishlabs/oasis-evm-web3-gateway/storage"
)

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

// BackendFactory is the indexer backend factory interface.
type BackendFactory func(
	dataDir string,
	runtimeID common.Namespace,
	storage storage.Storage) (Backend, error)

// QueryableBackend is the read-only indexer backend interface.
type QueryableBackend interface {
	// QueryBlockRound queries block round by block hash.
	QueryBlockRound(blockHash hash.Hash) (uint64, error)

	// QueryBlockHash queries block hash by round.
	QueryBlockHash(round uint64) (hash.Hash, error)

	// QueryTxResult queries oasis tx result by ethereum tx hash.
	QueryTxResult(ethTransactionHash hash.Hash) (*model.TxResult, error)

	// QueryEthTransaction queries ethereum transaction by hash.
	QueryEthTransaction(ethTxHash hash.Hash) (*model.EthTx, error)
}

// Backend is the indexer backend interface.
type Backend interface {
	QueryableBackend

	Index(
		round uint64,
		blockHash hash.Hash,
		txs []*types.UnverifiedTransaction,
	) error

	Close()
}

type psqlBackend struct {
	logger  *logging.Logger
	storage storage.Storage
}

func (p *psqlBackend) Index(
	round uint64,
	blockHash hash.Hash,
	txs []*types.UnverifiedTransaction,
) error {
	//block round <-> block hash
	blockRef := &model.Block{
		Round: round,
		Hash:  blockHash.String(),
	}
	p.storage.Store(blockRef)

	for idx, utx := range txs {
		if len(utx.AuthProofs) != 1 || utx.AuthProofs[0].Module != "evm.ethereum.v0" {
			// Skip non-Ethereum transactions.
			continue
		}

		// Extract raw Ethereum transaction for further processing.
		// Use standard libraries to decode the Ethereum transaction.
		ethTx := &ethtypes.Transaction{}
		if err := rlp.DecodeBytes(utx.Body, ethTx); err != nil {
			p.logger.Error("decode ethereum transaction", err)
			continue
		}

		txRef := &model.Transaction{
			EthTxHash: ethTx.Hash().String(),
			Result: &model.TxResult{
				//Hash:  ,
				Index: uint32(idx),
				Round: round,
			},
		}
		p.storage.Store(txRef)

		v, r, s := ethTx.RawSignatureValues()
		innerTx := &model.EthTx{
			Hash:  ethTx.Hash().String(),
			Gas:   ethTx.Gas(),
			Nonce: ethTx.Nonce(),
			To:    ethTx.To().String(),
			Value: ethTx.Value().String(),
			Data:  hex.EncodeToString(ethTx.Data()),
			V:     v.String(),
			R:     r.String(),
			S:     s.String(),
		}

		accList := []model.EthAccessTuple{}
		if ethTx.Type() == ethtypes.AccessListTxType || ethTx.Type() == ethtypes.DynamicFeeTxType {
			for _, tuple := range ethTx.AccessList() {
				addr := tuple.Address.String()
				keys := []string{}
				for _, k := range tuple.StorageKeys {
					keys = append(keys, k.String())
				}
				accList = append(accList, model.EthAccessTuple{
					Address:     addr,
					StorageKeys: keys,
				})
			}
		}

		switch ethTx.Type() {
		case ethtypes.LegacyTxType:
			{
				innerTx.Type = ethtypes.LegacyTxType
				innerTx.GasPrice = ethTx.GasPrice().String()
				innerTx.GasFeeCap = "0"
				innerTx.GasTipCap = "0"
				innerTx.ChainID = "0"
				innerTx.AccessList = []model.EthAccessTuple{}
			}
		case ethtypes.AccessListTxType:
			{
				innerTx.Type = ethtypes.AccessListTxType
				innerTx.GasPrice = ethTx.GasPrice().String()
				innerTx.ChainID = ethTx.ChainId().String()
				innerTx.AccessList = accList
				innerTx.GasFeeCap = "0"
				innerTx.GasTipCap = "0"
			}
		case ethtypes.DynamicFeeTxType:
			{
				innerTx.Type = ethtypes.DynamicFeeTxType
				innerTx.GasFeeCap = ethTx.GasFeeCap().String()
				innerTx.GasTipCap = ethTx.GasTipCap().String()
				innerTx.AccessList = accList
				innerTx.GasPrice = "0"
			}
		default:
			p.logger.Error("unknown ethereum transaction type")
			continue
		}

		p.storage.Store(innerTx)
	}

	return nil
}

func (p *psqlBackend) QueryBlockRound(blockHash hash.Hash) (uint64, error) {
	round, err := p.storage.GetBlockRound(blockHash.String())
	if err != nil {
		p.logger.Error("Can't find matched block")
		return 0, err
	}

	return round, nil
}

func (p *psqlBackend) QueryBlockHash(round uint64) (hash.Hash, error) {
	blockHash, err := p.storage.GetBlockHash(round)
	if err != nil {
		p.logger.Error("Indexer error!")
		return hash.Hash{}, err
	}

	return hash.NewFromBytes([]byte(blockHash)), nil
}

func (p *psqlBackend) QueryTxResult(ethTransactionHash hash.Hash) (*model.TxResult, error) {
	result, err := p.storage.GetTxResult(ethTransactionHash.String())
	if err != nil {
		p.logger.Error("Can't find matched transaction result")
		return nil, err
	}

	return result, nil
}

func (p *psqlBackend) QueryEthTransaction(ethTxHash hash.Hash) (*model.EthTx, error) {
	tx, err := p.storage.GetEthTransaction(ethTxHash.String())
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (p *psqlBackend) Close() {
	p.logger.Info("Psql backend closed!")
}

func newPsqlBackend(storage storage.Storage) (Backend, error) {
	b := &psqlBackend{
		logger:  logging.GetLogger("gateway/indexer/backend"),
		storage: storage,
	}
	b.logger.Info("New psql backend")

	return b, nil
}

func NewPsqlBackend(storage storage.Storage) (Backend, error) {
	return newPsqlBackend(storage)
}
