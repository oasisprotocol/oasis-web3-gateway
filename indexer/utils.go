package indexer

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/fxamacker/cbor/v2"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/evm"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"

	"github.com/oasisprotocol/emerald-web3-gateway/db/model"
	"github.com/oasisprotocol/emerald-web3-gateway/filters"
	"github.com/oasisprotocol/emerald-web3-gateway/storage"
)

var defaultValidatorAddr = "0x0000000000000000000000000000000088888888"

// Log is the Oasis Log.
type Log struct {
	Address common.Address `json:"address"`
	Topics  []common.Hash  `json:"topics"`
	Data    []byte         `json:"data"`
}

// Logs2EthLogs converts logs in db to ethereum logs.
func Logs2EthLogs(logs []*Log, round uint64, blockHash, txHash common.Hash, startIndex uint, txIndex uint32) []*ethtypes.Log {
	ethLogs := []*ethtypes.Log{}
	for i, log := range logs {
		ethLog := &ethtypes.Log{
			Address:     log.Address,
			Topics:      log.Topics,
			Data:        log.Data,
			BlockNumber: round,
			TxHash:      txHash,
			TxIndex:     uint(txIndex),
			BlockHash:   blockHash,
			Index:       startIndex + uint(i),
			Removed:     false,
		}
		ethLogs = append(ethLogs, ethLog)
	}
	return ethLogs
}

// convertToEthFormat converts oasis block to ethereum block.
func convertToEthFormat(
	block *block.Block,
	transactions ethtypes.Transactions,
	logsBloom ethtypes.Bloom,
	txsStatus []uint8,
	txsGasUsed []uint64,
	results []types.CallResult,
	blockGasLimit uint64,
) (*model.Block, []*model.Transaction, []*model.Receipt, error) {
	encoded := block.Header.EncodedHash()
	bhash := common.HexToHash(encoded.Hex()).String()
	bprehash := block.Header.PreviousHash.Hex()
	bshash := block.Header.StateRoot.Hex()
	bloomData, _ := logsBloom.MarshalText()
	baseFee := big.NewInt(10)
	number := big.NewInt(0)
	number.SetUint64(block.Header.Round)
	var btxHash string
	if len(transactions) == 0 {
		btxHash = ethtypes.EmptyRootHash.Hex()
	} else {
		btxHash = block.Header.IORoot.Hex()
	}

	innerHeader := &model.Header{
		ParentHash:  bprehash,
		UncleHash:   ethtypes.EmptyUncleHash.Hex(),
		Coinbase:    common.HexToAddress(defaultValidatorAddr).Hex(),
		Root:        bshash,
		TxHash:      btxHash,
		ReceiptHash: ethtypes.EmptyRootHash.Hex(),
		Bloom:       string(bloomData),
		Difficulty:  big.NewInt(0).String(),
		Number:      number.Uint64(),
		GasLimit:    blockGasLimit,
		Time:        uint64(block.Header.Timestamp),
		Extra:       "",
		MixDigest:   "",
		Nonce:       ethtypes.BlockNonce{}.Uint64(),
		BaseFee:     baseFee.String(),
	}

	innerTxs := make([]*model.Transaction, 0, len(transactions))
	innerReceipts := make([]*model.Receipt, 0, len(transactions))
	cumulativeGasUsed := uint64(0)
	for idx, ethTx := range transactions {
		v, r, s := ethTx.RawSignatureValues()
		signer := ethtypes.LatestSignerForChainID(ethTx.ChainId())
		from, _ := signer.Sender(ethTx)
		ethAccList := ethTx.AccessList()
		accList := make([]model.AccessTuple, 0, len(ethAccList))
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
			Status:     uint(txsStatus[idx]),
			BlockHash:  bhash,
			Round:      number.Uint64(),
			Index:      uint32(idx),
			Gas:        ethTx.Gas(),
			GasPrice:   ethTx.GasPrice().String(),
			GasTipCap:  ethTx.GasTipCap().String(),
			GasFeeCap:  ethTx.GasFeeCap().String(),
			Nonce:      ethTx.Nonce(),
			FromAddr:   from.Hex(),
			Value:      ethTx.Value().String(),
			Data:       hex.EncodeToString(ethTx.Data()),
			AccessList: accList,
			V:          v.String(),
			R:          r.String(),
			S:          s.String(),
		}
		to := ethTx.To()
		if to == nil {
			tx.ToAddr = ""
		} else {
			tx.ToAddr = to.Hex()
		}
		innerTxs = append(innerTxs, tx)

		// receipts
		cumulativeGasUsed += txsGasUsed[idx]
		receipt := &model.Receipt{
			Status:            uint(txsStatus[idx]),
			CumulativeGasUsed: cumulativeGasUsed,
			LogsBloom:         hex.EncodeToString(bloomData),
			TransactionHash:   ethTx.Hash().Hex(),
			BlockHash:         bhash,
			GasUsed:           txsGasUsed[idx],
			Type:              uint(ethTx.Type()),
			Round:             block.Header.Round,
			TransactionIndex:  uint64(idx),
			FromAddr:          tx.FromAddr,
			ToAddr:            tx.ToAddr,
		}
		if len(receipt.Logs) == 0 {
			receipt.Logs = []*model.Log{}
		}
		if tx.ToAddr == "" && results[idx].IsSuccess() {
			var out []byte
			if err := cbor.Unmarshal(results[idx].Ok, &out); err != nil {
				return nil, nil, nil, err
			}
			receipt.ContractAddress = common.BytesToAddress(out).Hex()
		}
		innerReceipts = append(innerReceipts, receipt)
	}
	innerHeader.GasUsed = cumulativeGasUsed

	innerBlock := &model.Block{
		Hash:         bhash,
		Round:        number.Uint64(),
		Header:       innerHeader,
		Transactions: innerTxs,
	}

	return innerBlock, innerTxs, innerReceipts, nil
}

// StoreBlockData parses oasis block and stores in db.
func (ib *indexBackend) StoreBlockData(oasisBlock *block.Block, txResults []*client.TransactionWithResults, blockGasLimit uint64) error { // nolint: gocyclo
	encoded := oasisBlock.Header.EncodedHash()
	bhash := common.HexToHash(encoded.Hex())
	blockNum := oasisBlock.Header.Round

	ethTxs := ethtypes.Transactions{}
	logs := []*ethtypes.Log{}
	txsStatus := []uint8{}
	txsGasUsed := []uint64{}
	results := []types.CallResult{}

	// Decode tx and get logs
	for txIndex, item := range txResults {
		oasisTx := item.Tx
		if len(oasisTx.AuthProofs) != 1 || oasisTx.AuthProofs[0].Module != "evm.ethereum.v0" {
			// Skip non-Ethereum transactions.
			continue
		}

		// Decode the Ethereum transaction.
		ethTx := &ethtypes.Transaction{}
		err := rlp.DecodeBytes(oasisTx.Body, ethTx)
		if err != nil {
			ib.logger.Error("Failed to decode UnverifiedTransaction", "height", blockNum, "index", txIndex, "err", err)
			return err
		}

		var status uint8
		if txResults[txIndex].Result.IsSuccess() {
			status = uint8(ethtypes.ReceiptStatusSuccessful)
		} else {
			status = uint8(ethtypes.ReceiptStatusFailed)
		}
		txsStatus = append(txsStatus, status)
		results = append(results, item.Result)
		ethTxs = append(ethTxs, ethTx)

		var txGasUsed uint64
		var oasisLogs []*Log
		resEvents := item.Events
		for eventIndex, event := range resEvents {
			switch {
			case event.Module == evm.ModuleName && event.Code == 1:
				// Manually parse EVM logs to also handle the legacy format which is not supported by the sdk.
				var logs []*Log
				if err = cbor.Unmarshal(event.Value, &logs); err != nil {
					ib.logger.Debug("Failed to unmarshal event value, trying legacy format next", "index", eventIndex, "err", err)

					// Emerald events value format changed in https://github.com/oasisprotocol/oasis-sdk/pull/675.
					// Try the legacy format in case the new format unmarshalling failed.
					var log Log
					if lerr := cbor.Unmarshal(event.Value, &log); lerr != nil {
						ib.logger.Error("Failed to unmarshal event value", "index", eventIndex, "legacy_err", lerr, "err", err)
						// The legacy fallback failed, likely the event not in legacy format, return the original unmarshal error.
						return err
					}
					oasisLogs = append(oasisLogs, &log)
					continue
				}
				oasisLogs = append(oasisLogs, logs...)
			case event.Module == core.ModuleName:
				events, err := core.DecodeEvent(event)
				if err != nil {
					ib.logger.Error("failed to decode core event", "index", eventIndex, "err", err)
					return err
				}
				for _, ev := range events {
					ce, ok := ev.(*core.Event)
					if !ok {
						ib.logger.Error("invalid core event", "event", ev, "index", eventIndex)
						return fmt.Errorf("invalid core event: %v", ev)
					}
					if ce.GasUsed == nil {
						continue
					}
					// Shouldn't happen unless something is terribly wrong.
					if txGasUsed != 0 {
						ib.logger.Error("multiple gas used events for a transaction", "prev_used", txGasUsed, "gas_used", ce, "index", eventIndex)
						return fmt.Errorf("multiple GasUsedEvent for a transaction")
					}
					txGasUsed = ce.GasUsed.Amount
				}
			default:
				// Ignore any other events.
			}
		}
		logs = append(logs, Logs2EthLogs(oasisLogs, blockNum, bhash, ethTx.Hash(), uint(len(logs)), uint32(txIndex))...)

		// Emerald GasUsed events were added in version 7.0.0.
		// Default to using gas limit, which was the behaviour before.
		if txGasUsed == 0 {
			ib.logger.Debug("no GasUsedEvent for a transaction, defaulting to gas limit", "transaction_index", txIndex, "round", oasisBlock.Header.Round)
			txGasUsed = ethTx.Gas()
		}
		txsGasUsed = append(txsGasUsed, txGasUsed)
	}
	dbLogs := eth2DbLogs(logs)

	// Convert to eth block, transactions and receipts.
	logsBloom := ethtypes.BytesToBloom(ethtypes.LogsBloom(logs))
	blk, txs, receipts, err := convertToEthFormat(oasisBlock, ethTxs, logsBloom, txsStatus, txsGasUsed, results, blockGasLimit)
	if err != nil {
		ib.logger.Debug("Failed to ConvertToEthBlock", "height", blockNum, "err", err)
		return err
	}

	if err = ib.storage.RunInTransaction(ib.ctx, func(s storage.Storage) error {
		// Store txs.
		for _, tx := range txs {
			switch tx.Status {
			case uint(ethtypes.ReceiptStatusFailed):
				// In the Emerald Paratime it can happen that an already committed
				// transaction is re-proposed at a later block. The duplicate transaction fails,
				// but it is still included in the block. The gateway should drop these
				// transactions to remain compatible with ETH semantics.
				if err = s.InsertIfNotExists(ib.ctx, tx); err != nil {
					return err
				}
			default:
				if err = s.Insert(ib.ctx, tx); err != nil {
					return err
				}
			}
		}

		// Store receipts.
		for _, receipt := range receipts {
			switch receipt.Status {
			case uint(ethtypes.ReceiptStatusFailed):
				// In the Emerald Paratime it can happen that an already committed
				// transaction is re-proposed at a later block. The duplicate transaction fails,
				// but it is still included in the block. The gateway should drop these
				// transactions to remain compatible with ETH semantics.
				if err = s.InsertIfNotExists(ib.ctx, receipt); err != nil {
					return err
				}
			default:
				if err = s.Insert(ib.ctx, receipt); err != nil {
					return err
				}
			}
		}

		// Store logs.
		for _, log := range dbLogs {
			if err = s.Insert(ib.ctx, log); err != nil {
				ib.logger.Error("Failed to store logs", "height", blockNum, "log", log, "err", err)
				return err
			}
		}

		// Store block.
		if err = s.Insert(ib.ctx, blk); err != nil {
			return err
		}

		// Update last indexed round.
		r := &model.IndexedRoundWithTip{
			Tip:   model.Continues,
			Round: blk.Round,
		}
		return s.Upsert(ib.ctx, r)
	}); err != nil {
		return err
	}

	// Send event to event system.
	chainEvent := filters.ChainEvent{
		Block: blk,
		Hash:  bhash,
		Logs:  dbLogs,
	}
	ib.subscribe.ChainChan() <- chainEvent
	ib.logger.Debug("sent chain event to event system", "height", blockNum)

	return nil
}

// eth2DbLogs converts ethereum log to model log.
func eth2DbLogs(ethLogs []*ethtypes.Log) []*model.Log {
	res := make([]*model.Log, 0, len(ethLogs))

	for _, log := range ethLogs {
		topics := make([]string, 0, len(log.Topics))
		for _, tp := range log.Topics {
			topics = append(topics, tp.Hex())
		}

		res = append(res, &model.Log{
			Address:   log.Address.Hex(),
			Topics:    topics,
			Data:      hex.EncodeToString(log.Data),
			Round:     log.BlockNumber,
			BlockHash: log.BlockHash.Hex(),
			TxHash:    log.TxHash.Hex(),
			TxIndex:   log.TxIndex,
			Index:     log.Index,
			Removed:   log.Removed,
		})
	}

	return res
}
