package indexer

import (
	"encoding/hex"
	"errors"
	"sync"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/oasisprotocol/oasis-core/go/common"
	"github.com/oasisprotocol/oasis-core/go/common/crypto/hash"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"

	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
	"github.com/starfishlabs/oasis-evm-web3-gateway/storage"
)

var GetLastRetainedError = errors.New("Get last retained round error in db.")

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
type BackendFactory func(runtimeID common.Namespace, storage storage.Storage) (Backend, error)

// QueryableBackend is the read-only indexer backend interface.
type QueryableBackend interface {
	// QueryBlockRound queries block round by block hash.
	QueryBlockRound(blockHash ethcommon.Hash) (uint64, error)

	// QueryBlockHash queries block hash by round.
	QueryBlockHash(round uint64) (ethcommon.Hash, error)

	// QueryTransactionRef returns block hash, round and index of the transaction.
	QueryTransactionRef(ethTxHash string) (*model.TransactionRef, error)

	// QueryLastIndexedRound query continues indexed block round.
	QueryLastIndexedRound() uint64

	// QueryLastRetainedRound query the minimum round not pruned.
	QueryLastRetainedRound() (uint64, error)

	// QueryTransaction queries ethereum transaction by hash.
	QueryTransaction(ethTxHash ethcommon.Hash) (*model.Transaction, error)
}

// GetEthInfoBackend is a backend for handling ethereum data.
type GetEthInfoBackend interface {
	GetBlockByNumber(number uint64) (*model.Block, error)
	GetBlockByHash(blockHash ethcommon.Hash) (*model.Block, error)
	GetBlockTransactionCountByNumber(number uint64) (int, error)
	GetBlockTransactionCountByHash(blockHash ethcommon.Hash) (int, error)
	GetTransactionByBlockHashAndIndex(blockHash ethcommon.Hash, txIndex int) (*model.Transaction, error)
	GetTransactionReceipt(txHash ethcommon.Hash) (map[string]interface{}, error)
	BlockNumber() (uint64, error)
	GetLogs(blockHash ethcommon.Hash, startRound, endRound uint64) ([]*model.Log, error)
}

// Backend is the indexer backend interface.
type Backend interface {
	QueryableBackend
	GetEthInfoBackend

	// Index indexes a block.
	Index(
		oasisBlock *block.Block,
		txResults []*client.TransactionWithResults,
	) error

	// Prune removes indexed data for rounds equal to or earlier than the passed round.
	Prune(round uint64) error

	// UpdateLastIndexedRound updates the last indexed round metadata.
	UpdateLastIndexedRound(round uint64) error

	// Close performs backend-specific cleanup. The backend should not be used anymore after calling
	// this method.
	Close()
}

type psqlBackend struct {
	runtimeID         common.Namespace
	logger            *logging.Logger
	storage           storage.Storage
	indexedRoundMutex *sync.Mutex
}

// Index indexes oasis block.
func (p *psqlBackend) Index(oasisBlock *block.Block, txResults []*client.TransactionWithResults) error {
	round := oasisBlock.Header.Round
	blockHash := ethcommon.HexToHash(oasisBlock.Header.EncodedHash().Hex())

	// oasis block round <-> oasis block hash, maybe remove later
	blockRef := &model.BlockRef{
		Round: oasisBlock.Header.Round,
		Hash:  blockHash.String(),
	}
	p.storage.Store(blockRef)

	// oasis block -> eth block, store eth block
	err := p.StoreBlockData(oasisBlock, txResults)
	if err != nil {
		p.logger.Error("generateEthBlock failed", "err", err)
		return err
	}

	p.logger.Info("indexed block", "round", round)

	return nil
}

// UpdateLastIndexedRound updates the last indexed round.
func (p *psqlBackend) UpdateLastIndexedRound(round uint64) error {
	return p.storeIndexedRound(round)
}

// Prune prunes data in db.
func (p *psqlBackend) Prune(round uint64) error {
	if err := p.storeLastRetainedRound(round); err != nil {
		return err
	}

	if err := p.storage.Delete(new(model.BlockRef), round); err != nil {
		return err
	}

	if err := p.storage.Delete(new(model.Block), round); err != nil {
		return err
	}

	if err := p.storage.Delete(new(model.Log), round); err != nil {
		return err
	}

	if err := p.storage.Delete(new(model.Transaction), round); err != nil {
		return err
	}

	if err := p.storage.Delete(new(model.TransactionRef), round); err != nil {
		return err
	}

	if err := p.storage.Delete(new(model.Receipt), round); err != nil {
		return err
	}

	return nil
}

// QueryBlockRound returns block number by block hash.
func (p *psqlBackend) QueryBlockRound(blockHash ethcommon.Hash) (uint64, error) {
	round, err := p.storage.GetBlockRound(blockHash.String())
	if err != nil {
		p.logger.Error("Can't find matched block")
		return 0, err
	}

	return round, nil
}

// QueryBlockHash returns block by block hash.
func (p *psqlBackend) QueryBlockHash(round uint64) (ethcommon.Hash, error) {
	var blockHash string
	var err error
	switch round {
	case RoundLatest:
		blockHash, err = p.storage.GetLatestBlockHash()
	default:
		blockHash, err = p.storage.GetBlockHash(round)
	}

	if err != nil {
		p.logger.Error("indexer error", "err", err)
		return ethcommon.Hash{}, err
	}
	return ethcommon.HexToHash(blockHash), nil
}

// storeIndexedRound stores indexed round.
func (p *psqlBackend) storeIndexedRound(round uint64) error {
	p.indexedRoundMutex.Lock()
	defer p.indexedRoundMutex.Unlock()

	r := &model.IndexedRoundWithTip{
		Tip:   model.Continues,
		Round: round,
	}

	return p.storage.Update(r)
}

// QueryLastIndexedRound returns the last indexed round.
func (p *psqlBackend) QueryLastIndexedRound() uint64 {
	p.indexedRoundMutex.Lock()
	indexedRound, err := p.storage.GetContinuesIndexedRound()
	if err != nil {
		p.indexedRoundMutex.Unlock()
		return 0
	}
	p.indexedRoundMutex.Unlock()

	return indexedRound
}

// storeLastRetainedRound stores the last retained round.
func (p *psqlBackend) storeLastRetainedRound(round uint64) error {
	r := &model.IndexedRoundWithTip{
		Tip:   model.LastRetained,
		Round: round,
	}

	return p.storage.Update(r)
}

// QueryLastRetainedRound returns the last retained round.
func (p *psqlBackend) QueryLastRetainedRound() (uint64, error) {
	lastRetainedRound, err := p.storage.GetLastRetainedRound()
	if err != nil {
		return 0, GetLastRetainedError
	}
	return lastRetainedRound, nil
}

// QueryTransaction returns transaction by transaction hash.
func (p *psqlBackend) QueryTransaction(txHash ethcommon.Hash) (*model.Transaction, error) {
	tx, err := p.storage.GetTransaction(txHash.String())
	if err != nil {
		return nil, err
	}
	return tx, nil
}

// QueryTransactionRef returns TransactionRef by transaction hash.
func (p *psqlBackend) QueryTransactionRef(hash string) (*model.TransactionRef, error) {
	return p.storage.GetTransactionRef(hash)
}

// GetBlockByNumber returns a block by block number.
func (p *psqlBackend) GetBlockByNumber(number uint64) (*model.Block, error) {
	blk, err := p.storage.GetBlockByNumber(number)
	if err != nil {
		return nil, err
	}

	return blk, nil
}

// GetBlockByHash returns a block by bock hash.
func (p *psqlBackend) GetBlockByHash(blockHash ethcommon.Hash) (*model.Block, error) {
	blk, err := p.storage.GetBlockByHash(blockHash.String())
	if err != nil {
		return nil, err
	}

	return blk, nil
}

// GetBlockTransactionCountByNumber returns the count of block transactions by block number.
func (p *psqlBackend) GetBlockTransactionCountByNumber(number uint64) (int, error) {
	return p.storage.GetBlockTransactionCountByNumber(number)
}

// GetBlockTransactionCountByHash returns the count of block transactions by block hash.
func (p *psqlBackend) GetBlockTransactionCountByHash(blockHash ethcommon.Hash) (int, error) {
	return p.storage.GetBlockTransactionCountByHash(blockHash.String())
}

// GetTransactionByBlockHashAndIndex returns transaction by the block hash and transaction index.
func (p *psqlBackend) GetTransactionByBlockHashAndIndex(blockHash ethcommon.Hash, txIndex int) (*model.Transaction, error) {
	return p.storage.GetBlockTransaction(blockHash.String(), txIndex)
}

// GetTransactionReceipt returns the receipt for the given tx.
func (p *psqlBackend) GetTransactionReceipt(txHash ethcommon.Hash) (map[string]interface{}, error) {
	dbReceipt, err := p.storage.GetTransactionReceipt(txHash.String())
	if err != nil {
		return nil, err
	}

	ethLogs := []*ethtypes.Log{}
	for _, dbLog := range dbReceipt.Logs {
		topics := []ethcommon.Hash{}
		for _, dbTopic := range dbLog.Topics {
			tp := ethcommon.HexToHash(dbTopic)
			topics = append(topics, tp)
		}

		data, _ := hex.DecodeString(dbLog.Data)
		log := &ethtypes.Log{
			Address:     ethcommon.HexToAddress(dbLog.Address),
			Topics:      topics,
			Data:        data,
			BlockNumber: dbLog.Round,
			TxHash:      ethcommon.HexToHash(dbLog.TxHash),
			TxIndex:     dbLog.TxIndex,
			BlockHash:   ethcommon.HexToHash(dbLog.BlockHash),
			Index:       dbLog.Index,
			Removed:     dbLog.Removed,
		}

		ethLogs = append(ethLogs, log)
	}

	receipt := map[string]interface{}{
		"status":            hexutil.Uint(dbReceipt.Status),
		"cumulativeGasUsed": hexutil.Uint64(dbReceipt.CumulativeGasUsed),
		"logsBloom":         ethtypes.BytesToBloom(ethtypes.LogsBloom(ethLogs)),
		"logs":              ethLogs,
		"transactionHash":   dbReceipt.TransactionHash,
		"gasUsed":           hexutil.Uint64(dbReceipt.GasUsed),
		"type":              hexutil.Uint64(dbReceipt.Type),
		"blockHash":         dbReceipt.BlockHash,
		"blockNumber":       hexutil.Uint64(dbReceipt.Round),
		"transactionIndex":  hexutil.Uint64(dbReceipt.TransactionIndex),
		"from":              nil,
		"to":                nil,
		"contractAddress":   nil,
	}
	if dbReceipt.FromAddr != "" {
		receipt["from"] = dbReceipt.FromAddr
	}
	if dbReceipt.ToAddr != "" {
		receipt["to"] = dbReceipt.ToAddr
	}
	if dbReceipt.ContractAddress != "" {
		receipt["contractAddress"] = dbReceipt.ContractAddress
	}
	return receipt, nil
}

// BlockNumber returns the latest block.
func (p *psqlBackend) BlockNumber() (uint64, error) {
	return p.storage.GetLatestBlockNumber()
}

// GetLogs returns logs from db.
func (p *psqlBackend) GetLogs(blockHash ethcommon.Hash, startRound, endRound uint64) ([]*model.Log, error) {
	return p.storage.GetLogs(blockHash.String(), startRound, endRound)
}

// Close closes postgresql backend.
func (p *psqlBackend) Close() {
	p.logger.Info("Psql backend closed!")
}

// newPsqlBackend creates a Backend.
func newPsqlBackend(runtimeID common.Namespace, storage storage.Storage) (Backend, error) {
	b := &psqlBackend{
		runtimeID:         runtimeID,
		logger:            logging.GetLogger("indexer"),
		storage:           storage,
		indexedRoundMutex: new(sync.Mutex),
	}
	b.logger.Info("New psql backend")

	return b, nil
}

// NewPsqlBackend returns a PsqlBackend.
func NewPsqlBackend() BackendFactory {
	return newPsqlBackend
}
