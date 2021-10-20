package utils

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
)

// ConvertToEthBlock returns a JSON-RPC compatible Ethereum block from a given Oasis block and its block result.
func ConvertToEthBlock(
	block *block.Block,
	transactions interface{},
	logs []*ethtypes.Log,
) (map[string]interface{}, error) {
	// TODO tx releated
	bhash, _ := block.Header.IORoot.MarshalBinary()
	bprehash, _ := block.Header.PreviousHash.MarshalBinary()
	bshash, _ := block.Header.StateRoot.MarshalBinary()
	btxhash, _ := block.Header.MessagesHash.MarshalBinary()
	bloom := ethtypes.BytesToBloom(ethtypes.LogsBloom(logs))

	res := map[string]interface{}{
		"number":     hexutil.Uint64(block.Header.Round),
		"hash":       hexutil.Bytes(bhash),
		"parentHash": common.BytesToHash(bprehash),
		"nonce":      ethtypes.BlockNonce{},
		"sha3Uncles": ethtypes.EmptyUncleHash,
		"logsBloom":  bloom,
		"stateRoot":  hexutil.Bytes(bshash),
		// "miner":      validatorAddr,
		"mixHash":    common.Hash{},
		"difficulty": (*hexutil.Big)(big.NewInt(0)),
		"extraData":  "0x",
		//"size":             hexutil.Uint64(size),
		//"gasLimit":         hexutil.Uint64(gasLimit),
		//"gasUsed":          (*hexutil.Big)(gasUsed),
		"timestamp":        hexutil.Uint64(block.Header.Timestamp),
		"transactionsRoot": hexutil.Bytes(btxhash),
		"receiptsRoot":     ethtypes.EmptyRootHash,

		"uncles":          []common.Hash{},
		"transactions":    transactions,
		"totalDifficulty": (*hexutil.Big)(big.NewInt(0)),
	}

	return res, nil
}

// ConstructRPCTransaction returns a transaction that will serialize to the RPC representation.
func ConstructRPCTransaction(
	ethTx *ethtypes.Transaction,
	blockHash common.Hash,
	blockNumber,
	index uint64,
) (*RPCTransaction, error) {

	signer := ethtypes.LatestSignerForChainID(ethTx.ChainId())
	from, _ := ethtypes.Sender(signer, ethTx)
	v, r, s := ethTx.RawSignatureValues()

	result := &RPCTransaction{
		Type:     hexutil.Uint64(ethTx.Type()),
		From:     from,
		Gas:      hexutil.Uint64(ethTx.Gas()),
		GasPrice: (*hexutil.Big)(ethTx.GasPrice()),
		Hash:     ethTx.Hash(),
		Input:    hexutil.Bytes(ethTx.Data()),
		Nonce:    hexutil.Uint64(ethTx.Nonce()),
		To:       ethTx.To(),
		Value:    (*hexutil.Big)(ethTx.Value()),
		V:        (*hexutil.Big)(v),
		R:        (*hexutil.Big)(r),
		S:        (*hexutil.Big)(s),
	}

	if blockHash != (common.Hash{}) {
		result.BlockHash = &blockHash
		result.BlockNumber = (*hexutil.Big)(new(big.Int).SetUint64(blockNumber))
		result.TransactionIndex = (*hexutil.Uint64)(&index)
	}

	switch ethTx.Type() {
	case ethtypes.AccessListTxType:
		al := ethTx.AccessList()
		result.Accesses = &al
		result.ChainID = (*hexutil.Big)(ethTx.ChainId())
	case ethtypes.DynamicFeeTxType:
		al := ethTx.AccessList()
		result.Accesses = &al
		result.ChainID = (*hexutil.Big)(ethTx.ChainId())
		result.GasFeeCap = (*hexutil.Big)(ethTx.GasFeeCap())
		result.GasTipCap = (*hexutil.Big)(ethTx.GasTipCap())
		result.GasPrice = (*hexutil.Big)(ethTx.GasFeeCap())
	}

	return result, nil
}
