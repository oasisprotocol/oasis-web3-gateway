package indexer

import (
	"encoding/hex"
	"math/big"

	"github.com/fxamacker/cbor/v2"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"

	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
)

var (
	defaultValidatorAddr = "0x0000000000000000000000000000000088888888"
	defaultSize          = 100
	defaultGasLimit      = 21000 * 1000
	EmptyRootHash        = common.HexToHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")
)

// Log is the Oasis Log.
type Log struct {
	Address common.Address
	Topics  []common.Hash
	Data    []byte
}

func logs2EthLogs(logs []*Log, round uint64, blockHash, txHash common.Hash, txIndex uint32) []*ethtypes.Log {
	ethLogs := []*ethtypes.Log{}
	for i := range logs {
		ethLog := &ethtypes.Log{
			Address:     logs[i].Address,
			Topics:      logs[i].Topics,
			Data:        logs[i].Data,
			BlockNumber: round,
			TxHash:      txHash,
			TxIndex:     uint(txIndex),
			BlockHash:   blockHash,
			Index:       uint(i),
			Removed:     false,
		}
		ethLogs = append(ethLogs, ethLog)
	}
	return ethLogs
}

func convertToEthBlock(
	block *block.Block,
	transactions ethtypes.Transactions,
	logs []*ethtypes.Log,
	gas uint64,
) (*model.Block, error) {
	bprehash, _ := block.Header.PreviousHash.MarshalBinary()
	bshash, _ := block.Header.StateRoot.MarshalBinary()
	bloom := ethtypes.BytesToBloom(ethtypes.LogsBloom(logs))

	var btxhash common.Hash
	if len(transactions) == 0 {
		btxhash = EmptyRootHash
	} else {
		btxhash = ethtypes.DeriveSha(transactions, trie.NewStackTrie(nil))
	}

	baseFee := big.NewInt(10)
	number := big.NewInt(0)
	number.SetUint64(block.Header.Round)
	bloomData, _ := bloom.MarshalText()
	ethHeader := &ethtypes.Header{
		ParentHash:  common.BytesToHash(bprehash),
		UncleHash:   ethtypes.EmptyUncleHash,
		Coinbase:    common.HexToAddress(defaultValidatorAddr),
		Root:        common.BytesToHash(bshash),
		TxHash:      btxhash,
		ReceiptHash: ethtypes.EmptyRootHash,
		Bloom:       bloom,
		Difficulty:  big.NewInt(0),
		Number:      number,
		GasLimit:    uint64(defaultGasLimit),
		GasUsed:     gas,
		Time:        uint64(block.Header.Timestamp),
		Extra:       []byte{},
		MixDigest:   common.Hash{},
		Nonce:       ethtypes.BlockNonce{},
		BaseFee:     baseFee,
	}
	bhash := ethHeader.Hash()

	innerHeader := &model.Header{
		ParentHash:  common.BytesToHash(bprehash).Hex(),
		UncleHash:   ethtypes.EmptyUncleHash.Hex(),
		Coinbase:    common.HexToAddress(defaultValidatorAddr).Hex(),
		Root:        common.BytesToHash(bshash).Hex(),
		TxHash:      btxhash.Hex(),
		ReceiptHash: ethtypes.EmptyRootHash.Hex(),
		Bloom:       string(bloomData),
		Difficulty:  big.NewInt(0).String(),
		Number:      number.String(),
		GasLimit:    uint64(defaultGasLimit),
		GasUsed:     gas,
		Time:        uint64(block.Header.Timestamp),
		Extra:       "",
		MixDigest:   "",
		Nonce:       ethtypes.BlockNonce{}.Uint64(),
		BaseFee:     baseFee.String(),
	}

	innerTxs := []*model.Transaction{}
	for _, ethTx := range transactions {
		r, s, v := ethTx.RawSignatureValues()
		signer := ethtypes.LatestSignerForChainID(ethTx.ChainId())
		from, _ := signer.Sender(ethTx)
		ethAccList := ethTx.AccessList()
		accList := []model.AccessTuple{}
		for _, ethAcc := range ethAccList {
			keys := []string{}
			for _, ethStorageKey := range ethAcc.StorageKeys {
				keys = append(keys, ethStorageKey.Hex())
			}
			acc := model.AccessTuple{
				Address:     ethAcc.Address.Hex(),
				StorageKeys: keys,
			}
			accList = append(accList, acc)
		}
		tx := &model.Transaction{
			Hash:       ethTx.Hash().Hex(),
			Type:       ethTx.Type(),
			ChainID:    ethTx.ChainId().String(),
			Gas:        ethTx.Gas(),
			GasPrice:   ethTx.GasPrice().String(),
			GasTipCap:  ethTx.GasTipCap().String(),
			GasFeeCap:  ethTx.GasFeeCap().String(),
			Nonce:      ethTx.Nonce(),
			FromAddr:   from.Hex(),
			ToAddr:     ethTx.To().Hex(),
			Value:      ethTx.Value().String(),
			Data:       hex.EncodeToString(ethTx.Data()),
			AccessList: accList,
			V:          v.String(),
			R:          r.String(),
			S:          s.String(),
		}
		innerTxs = append(innerTxs, tx)
	}
	innerBlock := &model.Block{
		Hash:         bhash.Hex(),
		Round:        number.Uint64(),
		Header:       innerHeader,
		Uncles:       []*model.Header{},
		Transactions: innerTxs,
	}
	return innerBlock, nil
}

func (p *psqlBackend) generateEthBlock(oasisBlock *block.Block, txResults []*client.TransactionWithResults) (*model.Block, error) {
	bhash, _ := oasisBlock.Header.IORoot.MarshalBinary()
	blockNum := oasisBlock.Header.Round
	ethTxs := ethtypes.Transactions{}
	var gasUsed uint64
	var logs []*ethtypes.Log

	for txIndex, item := range txResults {
		oasisTx := item.Tx
		if len(oasisTx.AuthProofs) != 1 || oasisTx.AuthProofs[0].Module != "evm.ethereum.v0" {
			// Skip non-Ethereum transactions.
			continue
		}

		// Extract raw Ethereum transaction for further processing.
		rawEthTx := oasisTx.Body
		// Decode the Ethereum transaction.
		ethTx := &ethtypes.Transaction{}
		err := rlp.DecodeBytes(rawEthTx, ethTx)
		if err != nil {
			p.logger.Error("Failed to decode UnverifiedTransaction", "height", blockNum, "index", txIndex, "error", err.Error())
			continue
		}

		gasUsed += uint64(ethTx.Gas())
		ethTxs = append(ethTxs, ethTx)

		var oasisLogs []*Log
		resEvents := item.Events
		for eventIndex, event := range resEvents {
			if event.Code == 1 {
				log := &Log{}
				if err := cbor.Unmarshal(event.Value, log); err != nil {
					p.logger.Error("Failed to unmarshal event value", "index", eventIndex)
					continue
				}
				oasisLogs = append(oasisLogs, log)
			}
		}

		logs = logs2EthLogs(oasisLogs, oasisBlock.Header.Round, common.BytesToHash(bhash), ethTx.Hash(), uint32(txIndex))
	}

	res, err := convertToEthBlock(oasisBlock, ethTxs, logs, gasUsed)
	if err != nil {
		p.logger.Debug("Failed to ConvertToEthBlock", "height", blockNum, "error", err.Error())
		return nil, err
	}
	return res, nil
}

func ConvertToOutBlock(block *model.Block) map[string]interface{} {
	v1 := big.NewInt(0)
	diff, _ := v1.SetString(block.Header.Difficulty, 10)

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

		"uncles":       block.Uncles,
		"transactions": block.Transactions,

		"hash": common.HexToHash(block.Hash),
		"size": hexutil.Uint64(defaultSize),

		"totalDifficulty": (*hexutil.Big)(big.NewInt(0)),
	}

	return res
}
