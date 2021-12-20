package utils

import (
	"bytes"
	"encoding/hex"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/oasisprotocol/oasis-core/go/common/cbor"

	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
)

// NewRPCTransaction returns a transaction that will serialize to the RPC representation.
func NewRPCTransaction(dbTx *model.Transaction) *RPCTransaction {
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
	blockHash := common.HexToHash(dbTx.BlockHash)
	txIndex := hexutil.Uint64(dbTx.Index)
	resTx := &RPCTransaction{
		Gas:              hexutil.Uint64(dbTx.Gas),
		GasPrice:         (*hexutil.Big)(gasPrice),
		GasFeeCap:        (*hexutil.Big)(gasFee),
		GasTipCap:        (*hexutil.Big)(gasTip),
		Hash:             common.HexToHash(dbTx.Hash),
		Input:            common.Hex2Bytes(dbTx.Data),
		Nonce:            hexutil.Uint64(dbTx.Nonce),
		Value:            (*hexutil.Big)(value),
		Type:             hexutil.Uint64(dbTx.Type),
		Accesses:         &accesses,
		ChainID:          (*hexutil.Big)(chainID),
		V:                (*hexutil.Big)(v),
		R:                (*hexutil.Big)(r),
		S:                (*hexutil.Big)(s),
		BlockHash:        &blockHash,
		BlockNumber:      (*hexutil.Big)(new(big.Int).SetUint64(dbTx.Round)),
		TransactionIndex: &txIndex,
	}

	if len(dbTx.FromAddr) == 0 {
		resTx.From = common.Address{}
	} else {
		resTx.From = common.HexToAddress(dbTx.FromAddr)
	}

	if len(dbTx.ToAddr) > 0 {
		to := common.HexToAddress(dbTx.ToAddr)
		resTx.To = &to
	}

	return resTx
}

// ConvertToEthBlock converts block in db to rpc response format of a block.
func ConvertToEthBlock(block *model.Block, fullTx bool) map[string]interface{} {
	transactions := []interface{}{}
	for _, dbTx := range block.Transactions {
		tx := NewRPCTransaction(dbTx)
		if fullTx {
			transactions = append(transactions, tx)
		} else {
			transactions = append(transactions, tx.Hash)
		}
	}

	header := DB2EthHeader(block)
	serialized := cbor.Marshal(block)

	res := map[string]interface{}{
		"parentHash":       header.ParentHash,
		"sha3Uncles":       header.UncleHash,
		"miner":            header.Coinbase,
		"stateRoot":        header.Root,
		"transactionsRoot": header.TxHash,
		"receiptsRoot":     header.ReceiptHash,
		"logsBloom":        header.Bloom,
		"difficulty":       (*hexutil.Big)(header.Difficulty),
		"number":           hexutil.Uint64(block.Round),
		"gasLimit":         hexutil.Uint64(header.GasLimit),
		"gasUsed":          hexutil.Uint64(header.GasUsed),
		"timestamp":        hexutil.Uint64(header.Time),
		"extraData":        header.Extra,
		"mixHash":          header.MixDigest,
		"nonce":            header.Nonce,
		"uncles":           []*model.Header{},
		"transactions":     transactions,
		"hash":             common.HexToHash(block.Hash),
		"size":             hexutil.Uint64(len(serialized)),
		"totalDifficulty":  (*hexutil.Big)(big.NewInt(0)),
	}

	return res
}

// DB2EthLogs converts log in db to ethereum log.
func DB2EthLogs(dbLogs []*model.Log) []*ethtypes.Log {
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

// DB2EthHeader converts block in db to ethereum header.
func DB2EthHeader(block *model.Block) *ethtypes.Header {
	v1 := big.NewInt(0)
	diff, _ := v1.SetString(block.Header.Difficulty, 10)
	noPrefix := block.Header.Bloom[2 : len(block.Header.Bloom)-1]
	bloomData, _ := hex.DecodeString(noPrefix)
	res := &ethtypes.Header{
		ParentHash:  common.HexToHash(block.Header.ParentHash),
		UncleHash:   common.HexToHash(block.Header.UncleHash),
		Coinbase:    common.HexToAddress(block.Header.Coinbase),
		Root:        common.HexToHash(block.Header.Root),
		TxHash:      common.HexToHash(block.Header.TxHash),
		ReceiptHash: common.HexToHash(block.Header.ReceiptHash),
		Bloom:       ethtypes.BytesToBloom(bloomData),
		Difficulty:  diff,
		Number:      new(big.Int).SetUint64(block.Round),
		GasLimit:    block.Header.GasLimit,
		GasUsed:     block.Header.GasUsed,
		Time:        block.Header.Time,
		Extra:       []byte(block.Header.Extra),
		MixDigest:   common.HexToHash(block.Header.MixDigest),
		Nonce:       ethtypes.EncodeNonce(block.Header.Nonce),
		// BaseFee was added by EIP-1559 and is ignored in legacy headers.
		BaseFee: big.NewInt(0),
	}

	return res
}

// TopicsMatch checks if event matches the filter topics.
//
// The Topic list restricts matches to particular event topics. Each event has a list
// of topics. Topics matches a prefix of that list. An empty element slice matches any
// topic. Non-empty elements represent an alternative that matches any of the
// contained topics.
//
// Examples:
// {} or nil          matches any topic list
// {{A}}              matches topic A in first position
// {{}, {B}}          matches any topic in first position AND B in second position
// {{A}, {B}}         matches topic A in first position AND B in second position
// {{A, B}, {C, D}}   matches topic (A OR B) in first position AND (C OR D) in second position.
func TopicsMatch(log *ethtypes.Log, filterTopics [][]common.Hash) bool {
	if len(filterTopics) > len(log.Topics) {
		// More topics in the filter than in the record. Cannot satisfy fhe filter.
		return false
	}

	for i, positionFilters := range filterTopics {
		if len(positionFilters) == 0 {
			// No topic at position i (wildcard), consider it a match.
			continue
		}

		// Check if the topic at position i matches the filter.
		positionMatch := false
		for _, topic := range positionFilters {
			if bytes.Equal(topic[:], log.Topics[i][:]) {
				positionMatch = true
				break
			}
		}
		if !positionMatch {
			// No match at position i.
			return false
		}
	}
	return true
}
