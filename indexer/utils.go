package indexer

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"

	"github.com/fxamacker/cbor/v2"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
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
) (map[string]interface{}, error) {
	bprehash, _ := block.Header.PreviousHash.MarshalBinary()
	bshash, _ := block.Header.StateRoot.MarshalBinary()
	bloom := ethtypes.BytesToBloom(ethtypes.LogsBloom(logs))

	var btxhash common.Hash
	if len(transactions) == 0 {
		btxhash = EmptyRootHash
	} else {
		btxhash = ethtypes.DeriveSha(transactions, trie.NewStackTrie(nil))
	}

	basefee := big.NewInt(10)
	number := big.NewInt(0)
	number.SetUint64(block.Header.Round)
	header := &ethtypes.Header{
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
		BaseFee:     basefee,
	}

	bhash := header.Hash()
	res := map[string]interface{}{
		"header":          header,
		"uncles":          []common.Hash{},
		"transactions":    transactions,
		"hash":            bhash.Hex(),
		"size":            hexutil.Uint64(defaultSize),
		"totalDifficulty": (*hexutil.Big)(big.NewInt(0)),
	}

	return res, nil
}

func (p *psqlBackend) generateEthBlock(oasisBlock *block.Block, txResults []*client.TransactionWithResults) (map[string]interface{}, error) {
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
