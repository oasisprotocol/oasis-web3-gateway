package utils

import (
	"encoding/hex"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/oasisprotocol/oasis-core/go/common/cbor"
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
	"math/big"
)

// NewRPCTransaction returns a transaction that will serialize to the RPC representation.
func NewRPCTransaction(
	dbTx *model.Transaction,
	blockHash common.Hash,
	blockNumber uint64,
	index hexutil.Uint64,
) (*RPCTransaction, error) {

	to := common.HexToAddress(dbTx.ToAddr)

	gasPrice, _ := new(big.Int).SetString(dbTx.GasPrice, 10)
	gasFee, _ := new(big.Int).SetString(dbTx.GasFeeCap, 10)
	gasTip, _ := new(big.Int).SetString(dbTx.GasTipCap, 10)
	value, _ := new(big.Int).SetString(dbTx.Value, 10)
	chainID, _ := new(big.Int).SetString(dbTx.ChainID, 10)
	v, _ := new(big.Int).SetString(dbTx.V, 10)
	r, _ := new(big.Int).SetString(dbTx.R, 10)
	s, _ := new(big.Int).SetString(dbTx.S, 10)

	var accesses ethtypes.AccessList
	for _, item := range dbTx.AccessList {
		var access ethtypes.AccessTuple
		access.Address = common.HexToAddress(item.Address)
		for _, keys := range item.StorageKeys {
			access.StorageKeys = append(access.StorageKeys, common.HexToHash(keys))
		}
		accesses = append(accesses, access)
	}

	resTx := &RPCTransaction{
		From:      common.HexToAddress(dbTx.FromAddr),
		Gas:       hexutil.Uint64(dbTx.Gas),
		GasPrice:  (*hexutil.Big)(gasPrice),
		GasFeeCap: (*hexutil.Big)(gasFee),
		GasTipCap: (*hexutil.Big)(gasTip),
		Hash:      common.HexToHash(dbTx.Hash),
		Input:     hexutil.Bytes(dbTx.Data),
		Nonce:     hexutil.Uint64(dbTx.Nonce),
		To:        &to,
		Value:     (*hexutil.Big)(value),
		Type:      hexutil.Uint64(dbTx.Type),
		Accesses:  &accesses,
		ChainID:   (*hexutil.Big)(chainID),
		V:         (*hexutil.Big)(v),
		R:         (*hexutil.Big)(r),
		S:         (*hexutil.Big)(s),
	}

	if blockHash != (common.Hash{}) {
		resTx.BlockHash = &blockHash
		resTx.BlockNumber = (*hexutil.Big)(new(big.Int).SetUint64(blockNumber))
		resTx.TransactionIndex = &index
	}

	return resTx, nil
}

func ConvertToEthBlock(block *model.Block, fullTx bool) map[string]interface{} {
	v1 := big.NewInt(0)
	diff, _ := v1.SetString(block.Header.Difficulty, 10)
	transactions := []interface{}{}

	if fullTx {
		transactions = append(transactions, block.Transactions)
	} else {
		for _, tx := range block.Transactions {
			transactions = append(transactions, tx.Hash)
		}
	}

	serialized := cbor.Marshal(block)

	res := map[string]interface{}{
		"parentHash":       common.HexToHash(block.Header.ParentHash),
		"sha3Uncles":       common.HexToHash(block.Header.UncleHash),
		"miner":            block.Header.Coinbase,
		"stateRoot":        common.HexToHash(block.Header.Root),
		"transactionsRoot": common.HexToHash(block.Header.TxHash),
		"receiptsRoot":     common.HexToHash(block.Header.ReceiptHash),
		"logsBloom":        block.Header.Bloom,
		"difficulty":       (*hexutil.Big)(diff),
		"number":           hexutil.Uint64(block.Round),
		"gasLimit":         hexutil.Uint64(block.Header.GasLimit),
		"gasUsed":          hexutil.Uint64(block.Header.GasUsed),
		"timestamp":        hexutil.Uint64(block.Header.Time),
		"extraData":        block.Header.Extra,
		"mixHash":          common.HexToHash(block.Header.MixDigest),
		"nonce":            ethtypes.EncodeNonce(block.Header.Nonce),
		"uncles":           block.Uncles,
		"transactions":     transactions,
		"hash":             common.HexToHash(block.Hash),
		"size":             hexutil.Uint64(len(serialized)),
		"totalDifficulty":  (*hexutil.Big)(big.NewInt(0)),
	}

	return res
}

func Db2EthLogs(dbLogs []*model.Log) []*ethtypes.Log {
	res := []*ethtypes.Log{}

	for _, log := range dbLogs {
		data, _ := hex.DecodeString(log.Data)
		topics := []common.Hash{}
		for _, tp := range log.Topics {
			topics = append(topics, common.HexToHash(tp))
		}

		ethLog := &ethtypes.Log{
			Address:     common.HexToAddress(log.Address),
			Topics:      topics,
			Data:        data,
			BlockNumber: log.Round,
			TxHash:      common.HexToHash(log.TxHash),
			TxIndex:     log.TxIndex,
			BlockHash:   common.HexToHash(log.BlockHash),
			Index:       log.Index,
			Removed:     log.Removed,
		}

		res = append(res, ethLog)
	}

	return res
}
