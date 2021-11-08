package indexer

import (
	"encoding/hex"
	"errors"
	"sync"

	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/oasisprotocol/oasis-core/go/common"
	"github.com/oasisprotocol/oasis-core/go/common/crypto/hash"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
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

	// QueryTransaction queries ethereum transaction by hash.
	QueryTransaction(ethTxHash ethcommon.Hash) (*model.Transaction, error)

	// Decode decodes an unverified transaction into ethereum transaction.
	Decode(utx *types.UnverifiedTransaction) (*model.Transaction, error)
}

type GetEthInfoBackend interface {
	GetBlockByNumber(number uint64) (*model.Block, error)
	GetBlockByHash(blockHash ethcommon.Hash) (*model.Block, error)
	GetBlockTransactionCountByNumber(number uint64) (int, error)
	GetBlockTransactionCountByHash(blockHash ethcommon.Hash) (int, error)
	GetTransactionByBlockHashAndIndex(blockHash ethcommon.Hash, txIndex int) (*model.Transaction, error)
	// GetTransactionReceipt(txHash ethcommon.Hash) (map[string]interface{}, error)
	BlockNumber() (uint64, error)
	GetLogs(blockHash ethcommon.Hash) ([]*model.Log, error)
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

func (p *psqlBackend) Decode(utx *types.UnverifiedTransaction) (*model.Transaction, error) {
	ethTx := &ethtypes.Transaction{}
	if err := rlp.DecodeBytes(utx.Body, ethTx); err != nil {
		return nil, err
	}
	v, r, s := ethTx.RawSignatureValues()
	signer := ethtypes.LatestSignerForChainID(ethTx.ChainId())
	from, _ := signer.Sender(ethTx)
	innerTx := &model.Transaction{
		Hash:     ethTx.Hash().String(),
		Gas:      ethTx.Gas(),
		Nonce:    ethTx.Nonce(),
		FromAddr: from.String(),
		Value:    ethTx.Value().String(),
		Data:     hex.EncodeToString(ethTx.Data()),
		V:        v.String(),
		R:        r.String(),
		S:        s.String(),
	}
	to := ethTx.To()
	if to == nil {
		innerTx.ToAddr = ""
	} else {
		innerTx.ToAddr = to.Hex()
	}
	accList := []model.AccessTuple{}
	if ethTx.Type() == ethtypes.AccessListTxType || ethTx.Type() == ethtypes.DynamicFeeTxType {
		for _, tuple := range ethTx.AccessList() {
			addr := tuple.Address.String()
			keys := []string{}
			for _, k := range tuple.StorageKeys {
				keys = append(keys, k.String())
			}
			accList = append(accList, model.AccessTuple{
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
			innerTx.AccessList = []model.AccessTuple{}
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

	return innerTx, nil
}

func (p *psqlBackend) DecodeUtx(utx *types.UnverifiedTransaction, blockHash string, round uint64, idx uint32) (*model.TransactionRef, *model.Transaction, error) {
	ethTx, err := p.Decode(utx)
	if err != nil {
		return nil, nil, err
	}
	ethTx.Index = idx
	ethTx.Round = round
	ethTx.BlockHash = blockHash

	txRef := &model.TransactionRef{
		EthTxHash: ethTx.Hash,
		Index:     idx,
		Round:     round,
		BlockHash: blockHash,
	}

	return txRef, ethTx, nil
}

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
	ethBlock, err := p.generateEthBlock(oasisBlock, txResults)
	if err != nil {
		p.logger.Error("generateEthBlock failed", "err", err)
		return err
	}
	err = p.storage.Store(ethBlock)
	if err != nil {
		return err
	}

	// txIndex excludes non-ethereum transactions.
	txIndex := uint32(0)
	for _, item := range txResults {
		utx := item.Tx
		if len(utx.AuthProofs) != 1 || utx.AuthProofs[0].Module != "evm.ethereum.v0" {
			// Skip non-Ethereum transactions.
			continue
		}
		txRef, ethTx, err := p.DecodeUtx(&utx, blockHash.String(), round, txIndex)
		if err != nil {
			p.logger.Error("failed to decode transaction", "err", err, "round", round, "index", txIndex)
			return err
		}
		if err := p.storage.Store(ethTx); err != nil {
			return err
		}
		txIndex++

		// remove later
		if err := p.storage.Store(txRef); err != nil {
			return err
		}
	}

	p.logger.Info("indexed block", "round", round)

	return nil
}

func (p *psqlBackend) UpdateLastIndexedRound(round uint64) error {
	p.storeIndexedRound(round)
	return nil
}

func (p *psqlBackend) Prune(round uint64) error {
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

	return nil
}

func (p *psqlBackend) QueryBlockRound(blockHash ethcommon.Hash) (uint64, error) {
	round, err := p.storage.GetBlockRound(blockHash.String())
	if err != nil {
		p.logger.Error("Can't find matched block")
		return 0, err
	}

	return round, nil
}

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

func (p *psqlBackend) storeIndexedRound(round uint64) {
	p.indexedRoundMutex.Lock()
	r := &model.ContinuesIndexedRound{
		Tip:   model.Continues,
		Round: round,
	}

	p.storage.Update(r)
	p.indexedRoundMutex.Unlock()
}

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

func (p *psqlBackend) QueryTransaction(ethTxHash ethcommon.Hash) (*model.Transaction, error) {
	tx, err := p.storage.GetTransaction(ethTxHash.String())
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (p *psqlBackend) QueryTransactionRef(hash string) (*model.TransactionRef, error) {
	return p.storage.GetTransactionRef(hash)
}

func (p *psqlBackend) GetBlockByNumber(number uint64) (*model.Block, error) {
	blk, err := p.storage.GetBlockByNumber(number)
	if err != nil {
		return nil, err
	}

	return blk, nil
}

func (p *psqlBackend) GetBlockByHash(blockHash ethcommon.Hash) (*model.Block, error) {
	blk, err := p.storage.GetBlockByHash(blockHash.String())
	if err != nil {
		return nil, err
	}

	return blk, nil
}

func (p *psqlBackend) GetBlockTransactionCountByNumber(number uint64) (int, error) {
	return p.storage.GetBlockTransactionCountByNumber(number)
}

func (p *psqlBackend) GetBlockTransactionCountByHash(blockHash ethcommon.Hash) (int, error) {
	return p.storage.GetBlockTransactionCountByHash(blockHash.String())
}

func (p *psqlBackend) GetTransactionByBlockHashAndIndex(blockHash ethcommon.Hash, txIndex int) (*model.Transaction, error) {
	return p.storage.GetBlockTransaction(blockHash.String(), txIndex)
}

//func (p *psqlBackend) GetTransactionReceipt(txHash ethcommon.Hash) (map[string]interface{}, error) {
//	return p.storage.GetTransactionReceipt(txHash.String())
//}

func (p *psqlBackend) BlockNumber() (uint64, error) {
	return p.storage.GetBlockNumber()
}

func (p *psqlBackend) GetLogs(blockHash ethcommon.Hash) ([]*model.Log, error) {
	return p.storage.GetLogs(blockHash.String())
}

func (p *psqlBackend) Close() {
	p.logger.Info("Psql backend closed!")
}

func newPsqlBackend(runtimeID common.Namespace, storage storage.Storage) (Backend, error) {
	b := &psqlBackend{
		runtimeID:         runtimeID,
		logger:            logging.GetLogger("gateway/indexer/backend"),
		storage:           storage,
		indexedRoundMutex: new(sync.Mutex),
	}
	b.logger.Info("New psql backend")

	return b, nil
}

func NewPsqlBackend() BackendFactory {
	return newPsqlBackend
}
