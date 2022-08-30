package indexer

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	"github.com/fxamacker/cbor/v2"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/oasisprotocol/oasis-core/go/common/quantity"
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

// ethToModelLogs converts ethereum logs to DB model logs.
func ethToModelLogs(ethLogs []*ethtypes.Log) ([]*model.Log, map[string][]*model.Log) {
	res := make([]*model.Log, 0, len(ethLogs))
	txLogs := make(map[string][]*model.Log)

	for _, log := range ethLogs {
		topics := make([]string, 0, len(log.Topics))
		for _, tp := range log.Topics {
			topics = append(topics, tp.Hex())
		}

		txHash := log.TxHash.Hex()
		log := &model.Log{
			Address:   log.Address.Hex(),
			Topics:    topics,
			Data:      hex.EncodeToString(log.Data),
			Round:     log.BlockNumber,
			BlockHash: log.BlockHash.Hex(),
			TxHash:    txHash,
			TxIndex:   log.TxIndex,
			Index:     log.Index,
			Removed:   log.Removed,
		}
		res = append(res, log)
		txLogs[txHash] = append(txLogs[txHash], log)
	}

	return res, txLogs
}

// blockToModels converts block data to DB model objects.
func blockToModels(
	block *block.Block,
	transactions ethtypes.Transactions,
	logs []*ethtypes.Log,
	txsStatus []uint8,
	txsGasUsed []uint64,
	results []types.CallResult,
	blockGasLimit uint64,
) (*model.Block, []*model.Transaction, []*model.Receipt, []*model.Log, error) {
	dbLogs, txLogs := ethToModelLogs(logs)

	encoded := block.Header.EncodedHash()
	bhash := common.HexToHash(encoded.Hex()).String()
	bprehash := block.Header.PreviousHash.Hex()
	bshash := block.Header.StateRoot.Hex()
	logsBloom := ethtypes.BytesToBloom(ethtypes.LogsBloom(logs))
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
			Logs:              txLogs[ethTx.Hash().Hex()],
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
				return nil, nil, nil, nil, err
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

	return innerBlock, innerTxs, innerReceipts, dbLogs, nil
}

// StoreBlockData parses oasis block and stores in db.
func (ib *indexBackend) StoreBlockData(ctx context.Context, oasisBlock *block.Block, txResults []*client.TransactionWithResults, blockGasLimit uint64) error { // nolint: gocyclo
	encoded := oasisBlock.Header.EncodedHash()
	bhash := common.HexToHash(encoded.Hex())
	blockNum := oasisBlock.Header.Round

	ethTxs := ethtypes.Transactions{}
	logs := []*ethtypes.Log{}
	txsStatus := []uint8{}
	txsGasUsed := []uint64{}
	results := []types.CallResult{}
	lastTransactionPrice := quantity.NewQuantity()

	// Decode tx and get logs
	for txIndex, item := range txResults {
		oasisTx := item.Tx
		if len(oasisTx.AuthProofs) != 1 || oasisTx.AuthProofs[0].Module != "evm.ethereum.v0" {
			// Skip non-Ethereum transactions unless this is the last transaction.
			if txIndex == len(txResults)-1 {
				// Try parsing the transaction to extract the gas price.
				var tx types.Transaction
				if err := cbor.Unmarshal(oasisTx.Body, &tx); err != nil {
					ib.logger.Warn("failed to unmarshal last transaction in the block", "err", err)
					continue
				}
				lastTransactionPrice = tx.AuthInfo.Fee.GasPrice()
			}
			continue
		}

		// Decode the Ethereum transaction.
		ethTx := &ethtypes.Transaction{}
		err := rlp.DecodeBytes(oasisTx.Body, ethTx)
		if err != nil {
			ib.logger.Error("Failed to decode UnverifiedTransaction", "height", blockNum, "index", txIndex, "err", err)
			return err
		}

		// Update the last transaction gas price. This is done for every transaction
		// since the last transaction could be a non-ethereum transaction and failing to
		// parse it should not be fatal.
		if err = lastTransactionPrice.FromBigInt(ethTx.GasPrice()); err != nil {
			ib.logger.Warn("failed to decode gas price", "err", err)
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

	// Convert to DB models.
	blk, txs, receipts, dbLogs, err := blockToModels(oasisBlock, ethTxs, logs, txsStatus, txsGasUsed, results, blockGasLimit)
	if err != nil {
		ib.logger.Debug("Failed to ConvertToEthBlock", "height", blockNum, "err", err)
		return err
	}

	// Store indexed models.
	upsertedTxes := make([]*model.Transaction, 0, len(txs))
	upsertedReceipts := make([]*model.Receipt, 0, len(receipts))
	if err = ib.storage.RunInTransaction(ctx, func(s storage.Storage) error {
		// Store txs.
		for i, tx := range txs {
			switch tx.Status {
			case uint(ethtypes.ReceiptStatusFailed):
				// In the Emerald Paratime it can happen that an already committed
				// transaction is re-proposed at a later block. The duplicate transaction fails,
				// but it is still included in the block. The gateway should drop these
				// transactions to remain compatible with ETH semantics.
				if err = s.InsertIfNotExists(ctx, tx); err != nil {
					return err
				}
			default:
				earlierTx, earlierTxErr := s.GetTransaction(ctx, tx.Hash)
				if earlierTxErr != nil {
					if !errors.Is(earlierTxErr, sql.ErrNoRows) {
						return err
					}
					// First encounter of this tx, continue to upsert.
				} else {
					if earlierTx.Status != uint(ethtypes.ReceiptStatusFailed) {
						ib.logger.Error("duplicate tx",
							"earlier_tx", earlierTx,
							"tx", tx,
						)
						return fmt.Errorf("duplicate tx hash %s in rounds %d and %d", tx.Hash, earlierTx.Round, tx.Round)
					}
					// Replacing a failed encounter of this tx, continue to upsert.
				}
				if upsertErr := s.Upsert(ctx, tx); upsertErr != nil {
					return upsertErr
				}
				upsertedTxes = append(upsertedTxes, txs[i])
			}
		}

		// Store receipts.
		for i, receipt := range receipts {
			switch receipt.Status {
			case uint(ethtypes.ReceiptStatusFailed):
				// In the Emerald Paratime it can happen that an already committed
				// transaction is re-proposed at a later block. The duplicate transaction fails,
				// but it is still included in the block. The gateway should drop these
				// transactions to remain compatible with ETH semantics.
				if err = s.InsertIfNotExists(ctx, receipt); err != nil {
					return err
				}
			default:
				earlierReceipt, earlierReceiptErr := s.GetTransactionReceipt(ctx, receipt.TransactionHash)
				if earlierReceiptErr != nil {
					if !errors.Is(earlierReceiptErr, sql.ErrNoRows) {
						return err
					}
					// First encounter of this receipt, continue to upsert.
				} else {
					if earlierReceipt.Status != uint(ethtypes.ReceiptStatusFailed) {
						ib.logger.Error("duplicate receipt",
							"earlier_receipt", earlierReceipt,
							"receipt", receipt,
						)
						return fmt.Errorf("duplicate receipt tx hash %s in rounds %d and %d", receipt.TransactionHash, earlierReceipt.Round, receipt.Round)
					}
					// Replacing a failed encounter of this receipt, continue to upsert.
				}
				if upsertErr := s.Upsert(ctx, receipt); upsertErr != nil {
					return upsertErr
				}
				upsertedReceipts = append(upsertedReceipts, receipts[i])
			}
		}

		// Store logs.
		for _, log := range dbLogs {
			if err = s.Insert(ctx, log); err != nil {
				ib.logger.Error("Failed to store logs", "height", blockNum, "log", log, "err", err)
				return err
			}
		}

		// Store block.
		if err = s.Insert(ctx, blk); err != nil {
			return err
		}

		// Update last indexed round.
		r := &model.IndexedRoundWithTip{
			Tip:   model.Continues,
			Round: blk.Round,
		}
		return s.Upsert(ctx, r)
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

	bd := &BlockData{
		Block:                blk,
		Receipts:             upsertedReceipts,
		UniqueTxes:           upsertedTxes,
		LastTransactionPrice: lastTransactionPrice,
	}

	// Notify the observer if any.
	if ib.observer != nil {
		ib.observer.OnBlockIndexed(bd)
	}
	ib.blockNotifier.Broadcast(bd)

	return nil
}

// db2EthReceipt converts model.Receipt to the GetTransactipReceipt format.
func db2EthReceipt(dbReceipt *model.Receipt) map[string]interface{} {
	ethLogs := make([]*ethtypes.Log, 0, len(dbReceipt.Logs))
	for _, dbLog := range dbReceipt.Logs {
		topics := make([]common.Hash, 0, len(dbLog.Topics))
		for _, dbTopic := range dbLog.Topics {
			tp := common.HexToHash(dbTopic)
			topics = append(topics, tp)
		}

		data, _ := hex.DecodeString(dbLog.Data)
		log := &ethtypes.Log{
			Address:     common.HexToAddress(dbLog.Address),
			Topics:      topics,
			Data:        data,
			BlockNumber: dbLog.Round,
			TxHash:      common.HexToHash(dbLog.TxHash),
			TxIndex:     dbLog.TxIndex,
			BlockHash:   common.HexToHash(dbLog.BlockHash),
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
	return receipt
}
