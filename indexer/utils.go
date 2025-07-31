package indexer

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/oasisprotocol/oasis-core/go/common/cbor"
	"github.com/oasisprotocol/oasis-core/go/common/quantity"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/accounts"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/evm"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"

	"github.com/oasisprotocol/oasis-web3-gateway/db/model"
	"github.com/oasisprotocol/oasis-web3-gateway/filters"
	"github.com/oasisprotocol/oasis-web3-gateway/storage"
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

// Taken from https://github.com/ethereum/go-ethereum/blob/6b6261b51fa46f7ed36d0589db641c14569ce036/core/types/bloom9.go#L118-L129
// Which was removed in: https://github.com/ethereum/go-ethereum/pull/31129 from go-ethereum.
func logsBloom(logs []*ethtypes.Log) []byte {
	var bin ethtypes.Bloom
	for _, log := range logs {
		bin.Add(log.Address.Bytes())
		for _, b := range log.Topics {
			bin.Add(b[:])
		}
	}
	return bin[:]
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
	txsGas []txGas,
	results []types.CallResult,
	blockGasLimit uint64,
	baseFee *big.Int,
) (*model.Block, []*model.Transaction, []*model.Receipt, []*model.Log, error) {
	dbLogs, txLogs := ethToModelLogs(logs)

	encoded := block.Header.EncodedHash()
	bhash := common.HexToHash(encoded.Hex()).String()
	bprehash := block.Header.PreviousHash.Hex()
	bshash := block.Header.StateRoot.Hex()
	logsBloom := ethtypes.BytesToBloom(logsBloom(logs))
	bloomData, _ := logsBloom.MarshalText()
	bloomHex := hex.EncodeToString(bloomData)
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
		Number:      block.Header.Round,
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
		ethTxHex := ethTx.Hash().Hex()
		v, r, s := ethTx.RawSignatureValues()
		chainID := ethTx.ChainId()
		if chainID != nil && chainID.Cmp(big.NewInt(0)) == 0 {
			// Legacy transactions don't have a chain ID but `ChainId()` returns 0, which is invalid
			// for `LatestSignerForChainId` which expects a null in that case (zero causes a panic).
			// https://github.com/ethereum/go-ethereum/issues/31653
			chainID = nil
		}
		signer := ethtypes.LatestSignerForChainID(chainID)
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
			Hash:       ethTxHex,
			Type:       ethTx.Type(),
			ChainID:    ethTx.ChainId().String(),
			Status:     uint(txsStatus[idx]),
			BlockHash:  bhash,
			Round:      block.Header.Round,
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
		cumulativeGasUsed += txsGas[idx].used
		receipt := &model.Receipt{
			Status:            uint(txsStatus[idx]),
			CumulativeGasUsed: cumulativeGasUsed,
			Logs:              txLogs[ethTxHex],
			LogsBloom:         bloomHex,
			TransactionHash:   ethTxHex,
			BlockHash:         bhash,
			GasUsed:           txsGas[idx].used,
			EffectiveGasPrice: txsGas[idx].effectivePrice.String(),
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
		Round:        block.Header.Round,
		Header:       innerHeader,
		Transactions: innerTxs,
	}

	return innerBlock, innerTxs, innerReceipts, dbLogs, nil
}

type txGas struct {
	used           uint64
	effectivePrice *big.Int
}

// StoreBlockData parses oasis block and stores in db.
func (ib *indexBackend) StoreBlockData(ctx context.Context, oasisBlock *block.Block, txResults []*client.TransactionWithResults, coreParameters *core.Parameters) error { //nolint: gocyclo
	encoded := oasisBlock.Header.EncodedHash()
	bhash := common.HexToHash(encoded.Hex())
	blockNum := oasisBlock.Header.Round

	ethTxs := ethtypes.Transactions{}
	logs := []*ethtypes.Log{}
	txsStatus := []uint8{}
	txsGas := []txGas{}
	results := []types.CallResult{}
	var medianTransactionGasPrice *quantity.Quantity

	// Decode tx and get logs
	for txIndex, item := range txResults {
		oasisTx := item.Tx
		if len(oasisTx.AuthProofs) != 1 || oasisTx.AuthProofs[0].Module != "evm.ethereum.v0" {
			// Skip non-Ethereum transactions unless this is the median transaction (or after it, and we don't have the median price yet).
			if txIndex >= (len(txResults)/2)-1 && medianTransactionGasPrice == nil {
				// Try parsing the transaction to extract the gas price.
				var tx types.Transaction
				if err := cbor.Unmarshal(oasisTx.Body, &tx); err != nil {
					ib.logger.Warn("failed to unmarshal last transaction in the block", "err", err)
					continue
				}
				medianTransactionGasPrice = tx.AuthInfo.Fee.GasPrice()
			}
			continue
		}

		// Decode the Ethereum transaction.
		ethTx := &ethtypes.Transaction{}
		err := ethTx.UnmarshalBinary(oasisTx.Body)
		if err != nil {
			ib.logger.Error("Failed to decode UnverifiedTransaction", "height", blockNum, "index", txIndex, "err", err)
			return err
		}

		// If we are after median tx, and there's no median transaction gas price (parsing failed), set median to current gas price.
		if txIndex > (len(txResults)/2)-1 && medianTransactionGasPrice == nil {
			var gasPrice quantity.Quantity
			if err = gasPrice.FromBigInt(ethTx.GasPrice()); err != nil {
				ib.logger.Warn("failed to decode gas price", "err", err)
			} else {
				medianTransactionGasPrice = &gasPrice
			}
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
		txGasSpent := big.NewInt(0)
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
			case event.Module == accounts.ModuleName && event.Code == accounts.TransferEventCode:
				// Parse fee transfer event, to obtain fee payment.
				events, err := accounts.DecodeEvent(event)
				if err != nil {
					ib.logger.Error("failed to decode accounts transfer event", "index", eventIndex, "err", err)
					continue
				}
				for _, ev := range events {
					ae, ok := ev.(*accounts.Event)
					if !ok {
						ib.logger.Error("invalid accounts event", "event", ev, "index", eventIndex)
						continue
					}
					te := ae.Transfer
					if te == nil {
						ib.logger.Error("invalid accounts transfer event", "event", ev, "index", eventIndex)
						continue
					}
					// Skip transfers that are not to the fee accumulator.
					if !te.To.Equal(accounts.FeeAccumulatorAddress) {
						continue
					}
					// Skip non native denom.
					if !te.Amount.Denomination.IsNative() {
						continue
					}
					txGasSpent.Add(txGasSpent, te.Amount.Amount.ToBigInt())
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

		var txEffectivePrice *big.Int
		switch {
		case txGasSpent.Cmp(big.NewInt(0)) > 0:
			// If there was a fee payment event observed, calculate effective gas price.
			txEffectivePrice = new(big.Int).Div(txGasSpent, new(big.Int).SetUint64(txGasUsed))
		default:
			// In old runtimes, there was no transfer events emitted for fee payments, just use gas price in those cases.
			// Gas price should likely be correct since those runtimes also didn't support EIP-1559 transactions, so
			// gas price should match effective gas price.
			txEffectivePrice = ethTx.GasPrice()
		}
		txsGas = append(txsGas, txGas{used: txGasUsed, effectivePrice: txEffectivePrice})
	}

	// Convert to DB models.
	var minGasPrice big.Int
	var blockGasLimit uint64
	if coreParameters != nil {
		mgp := coreParameters.MinGasPrice[types.NativeDenomination]
		minGasPrice = *mgp.ToBigInt()
		blockGasLimit = coreParameters.MaxBatchGas
	}
	blk, txs, receipts, dbLogs, err := blockToModels(oasisBlock, ethTxs, logs, txsStatus, txsGas, results, blockGasLimit, &minGasPrice)
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
		Block:                     blk,
		Receipts:                  upsertedReceipts,
		UniqueTxes:                upsertedTxes,
		MedianTransactionGasPrice: medianTransactionGasPrice,
	}

	// Notify the observer if any.
	if ib.observer != nil {
		ib.observer.OnBlockIndexed(bd)
	}
	ib.blockNotifier.Broadcast(bd)

	return nil
}

// db2EthReceipt converts model.Receipt to the GetTransactionReceipt format.
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

	effectiveGasPrice, _ := new(big.Int).SetString(dbReceipt.EffectiveGasPrice, 10)
	receipt := map[string]interface{}{
		"status":            hexutil.Uint(dbReceipt.Status),
		"cumulativeGasUsed": hexutil.Uint64(dbReceipt.CumulativeGasUsed),
		"logsBloom":         ethtypes.BytesToBloom(logsBloom(ethLogs)),
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
		"effectiveGasPrice": (*hexutil.Big)(effectiveGasPrice),
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
