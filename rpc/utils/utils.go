package utils

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"

	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
)

var (
	defaultValidatorAddr = "0x0000000000000000000000000000000088888888"
	defaultSize          = 100
	defaultGasLimit      = 21000 * 1000
	EmptyRootHash        = common.HexToHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")
)

// ConvertToEthBlock returns a JSON-RPC compatible Ethereum Block from a given Oasis block and its block result.
func ConvertToEthBlock(
	block *block.Block,
	transactions []interface{},
	logs []*ethtypes.Log,
	gas uint64,
) (map[string]interface{}, error) {
	encoded := block.Header.EncodedHash()
	bHash, _ := encoded.MarshalBinary()
	bPrevHash, _ := block.Header.PreviousHash.MarshalBinary()
	bStateHash, _ := block.Header.StateRoot.MarshalBinary()

	bloom := ethtypes.BytesToBloom(ethtypes.LogsBloom(logs))
	gasUsed := big.NewInt(0).SetUint64(gas)
	btxHash, _ := block.Header.IORoot.MarshalBinary()

	res := map[string]interface{}{
		"number":           hexutil.Uint64(block.Header.Round),
		"hash":             common.BytesToHash(bHash),
		"parentHash":       common.BytesToHash(bPrevHash),
		"nonce":            ethtypes.BlockNonce{},
		"sha3Uncles":       ethtypes.EmptyUncleHash,
		"logsBloom":        bloom,
		"stateRoot":        hexutil.Bytes(bStateHash),
		"miner":            defaultValidatorAddr,
		"mixHash":          common.Hash{},
		"difficulty":       (*hexutil.Big)(big.NewInt(0)),
		"extraData":        "0x",
		"size":             hexutil.Uint64(defaultSize),
		"gasLimit":         hexutil.Uint64(defaultGasLimit),
		"gasUsed":          (*hexutil.Big)(gasUsed),
		"timestamp":        hexutil.Uint64(block.Header.Timestamp),
		"transactionsRoot": hexutil.Bytes(btxHash),
		"receiptsRoot":     ethtypes.EmptyRootHash,

		"uncles":          []common.Hash{},
		"transactions":    transactions,
		"totalDifficulty": (*hexutil.Big)(big.NewInt(0)),
	}

	return res, nil
}

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
