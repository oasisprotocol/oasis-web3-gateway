package indexer

import (
	"encoding/hex"
	"errors"
	"sync"

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
type BackendFactory func(dataDir string, runtimeID common.Namespace, storage storage.Storage) (Backend, error)

// QueryableBackend is the read-only indexer backend interface.
type QueryableBackend interface {
	// QueryBlockRound queries block round by block hash.
	QueryBlockRound(blockHash hash.Hash) (uint64, error)

	// QueryBlockHash queries block hash by round.
	QueryBlockHash(round uint64) (hash.Hash, error)

	// QueryTransactionRoundAndIndex queries transaction round and index by transaction hash.
	QueryTransactionRoundAndIndex(ethTxHash string) (uint64, uint32, error)

	// QueryTransactionByRoundAndIndex queries ethereum transaction by round and index.
	QueryTransactionByRoundAndIndex(round uint64, index uint32) (*model.EthTransaction, error)

	// QueryIndexedRound query continues indexed block round.
	QueryIndexedRound() uint64

	// QueryEthTransaction queries ethereum transaction by hash.
	QueryEthTransaction(ethTxHash hash.Hash) (*model.EthTransaction, error)

	// Decode decodes an unverified transaction into ethereum transaction.
	Decode(utx *types.UnverifiedTransaction) (*model.EthTransaction, error)
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
	logger            *logging.Logger
	storage           storage.Storage
	indexedRoundMutex *sync.Mutex
}

func (p *psqlBackend) Decode(utx *types.UnverifiedTransaction) (*model.EthTransaction, error) {
	ethTx := &ethtypes.Transaction{}
	if err := rlp.DecodeBytes(utx.Body, ethTx); err != nil {
		return nil, err
	}
	v, r, s := ethTx.RawSignatureValues()
	innerTx := &model.EthTransaction{
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
		return nil, errors.New("unknown transaction type")
	}

	return nil, nil
}

func (p *psqlBackend) DecodeUtx(utx *types.UnverifiedTransaction, round uint64, idx uint32) (*model.TransactionRef, *model.EthTransaction, error) {
	ethTx, err := p.Decode(utx)
	if err != nil {
		return nil, nil, err
	}

	txRef := &model.TransactionRef{
		EthTxHash: ethTx.Hash,
		Index:     idx,
		Round:     round,
	}

	return txRef, ethTx, nil
}

func (p *psqlBackend) Index(
	round uint64,
	blockHash hash.Hash,
	txs []*types.UnverifiedTransaction,
) error {
	//block round <-> block hash
	blockRef := &model.BlockRef{
		Round: round,
		Hash:  blockHash.String(),
	}
	p.storage.Store(blockRef)

	for idx, utx := range txs {
		if len(utx.AuthProofs) != 1 || utx.AuthProofs[0].Module != "evm.ethereum.v0" {
			// Skip non-Ethereum transactions.
			continue
		}
		txRef, ethTx, err := p.DecodeUtx(utx, round, uint32(idx))
		if err != nil {
			p.logger.Error("decode transaction", err)
			continue
		}
		p.storage.Store(txRef)
		p.storage.Store(ethTx)
	}

	if p.QueryIndexedRound() == (round - 1) {
		p.storeIndexedRound(round)
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

func (p *psqlBackend) storeIndexedRound(round uint64) {
	p.indexedRoundMutex.Lock()
	r := &model.ContinuesIndexedRound{
		Round: round,
	}

	p.storage.Store(r)
	p.indexedRoundMutex.Unlock()
}

func (p *psqlBackend) QueryIndexedRound() uint64 {
	p.indexedRoundMutex.Lock()
	indexedRound, err := p.storage.GetContinuesIndexedRound()
	if err != nil {
		p.indexedRoundMutex.Unlock()
		p.storeIndexedRound(0)
		return 0
	}
	p.indexedRoundMutex.Unlock()

	return indexedRound
}

func (p *psqlBackend) QueryEthTransaction(ethTxHash hash.Hash) (*model.EthTransaction, error) {
	tx, err := p.storage.GetEthTransaction(ethTxHash.String())
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (p *psqlBackend) QueryTransactionRoundAndIndex(hash string) (uint64, uint32, error) {
	return p.storage.GetTransactionRoundAndIndex(hash)
}

func (p *psqlBackend) QueryTransactionByRoundAndIndex(round uint64, index uint32) (*model.EthTransaction, error) {
	return p.storage.GetTransactionByRoundAndIndex(round, index)
}

func (p *psqlBackend) Close() {
	p.logger.Info("Psql backend closed!")
}

func newPsqlBackend(storage storage.Storage) (Backend, error) {
	b := &psqlBackend{
		logger:            logging.GetLogger("gateway/indexer/backend"),
		storage:           storage,
		indexedRoundMutex: new(sync.Mutex),
	}
	b.logger.Info("New psql backend")

	return b, nil
}

func NewPsqlBackend(storage storage.Storage) (Backend, error) {
	return newPsqlBackend(storage)
}
