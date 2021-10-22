package eth

import (
	"context"
	"errors"
	"math/big"

	"github.com/oasisprotocol/oasis-core/go/common/cbor"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/starfishlabs/oasis-evm-web3-gateway/indexer"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	ethrpc "github.com/ethereum/go-ethereum/rpc"

	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-core/go/common/quantity"

	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/accounts"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/evm"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"

	"github.com/starfishlabs/oasis-evm-web3-gateway/rpc/utils"
)

const (
	// Callable methods.
	methodCreate = "evm.Create"
	methodCall   = "evm.Call"
)

// Log is the Oasis Log.
type Log struct {
	Address common.Address
	Topics  []common.Hash
	Data    []byte
}

// PublicAPI is the eth_ prefixed set of APIs in the Web3 JSON-RPC spec.
type PublicAPI struct {
	ctx     context.Context
	client  client.RuntimeClient
	backend indexer.Backend
	Logger  *logging.Logger
}

// NewPublicAPI creates an instance of the public ETH Web3 API.
func NewPublicAPI(
	ctx context.Context,
	client client.RuntimeClient,
	logger *logging.Logger,
	backend indexer.Backend,
) *PublicAPI {
	return &PublicAPI{
		ctx:     ctx,
		client:  client,
		Logger:  logger,
		backend: backend,
	}
}

// GetBlockByNumber returns the block identified by number.
func (api *PublicAPI) GetBlockByNumber(blockNum uint64) (map[string]interface{}, error) {
	api.Logger.Debug("eth_getBlockByNumber", "number", blockNum)
	resBlock, err := api.client.GetBlock(api.ctx, blockNum)
	if err != nil {
		api.Logger.Error("GetBlock failed", "number", blockNum)
	}

	bhash, _ := resBlock.Header.IORoot.MarshalBinary()
	ethRPCTxs := []interface{}{}
	var oasisLogs []*Log
	var logs []*ethtypes.Log
	txResults, err := api.client.GetTransactionsWithResults(api.ctx, resBlock.Header.Round)
	if err != nil {
		api.Logger.Error("Failed to get transaction results", "number", blockNum, "error", err.Error())
		return nil, err
	}

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
			api.Logger.Error("Failed to decode UnverifiedTransaction", "height", blockNum, "index", txIndex, "error", err.Error())
			continue
		}

		rpcTx, err := utils.ConstructRPCTransaction(
			ethTx,
			common.BytesToHash(bhash),
			uint64(resBlock.Header.Round),
			uint64(txIndex),
		)
		if err != nil {
			api.Logger.Error("Failed to ConstructRPCTransaction", "hash", ethTx.Hash().Hex(), "error", err.Error())
			continue
		}

		ethRPCTxs = append(ethRPCTxs, rpcTx)
		var resEvents []*types.Event
		resEvents = item.Events
		for eventIndex, event := range resEvents {
			if event.Code == 1 {
				log := &Log{}
				if err := cbor.Unmarshal(event.Value, log); err != nil {
					api.Logger.Error("Failed to unmarshal event value", "index", eventIndex)
					continue
				}
				oasisLogs = append(oasisLogs, log)
			}
		}

		logs = logs2EthLogs(oasisLogs, resBlock.Header.Round, common.BytesToHash(bhash), rpcTx.Hash, uint32(txIndex))
	}

	res, err := utils.ConvertToEthBlock(resBlock, ethRPCTxs, logs)
	if err != nil {
		api.Logger.Debug("Failed to ConvertToEthBlock", "height", blockNum, "error", err.Error())
		return nil, err
	}

	return res, nil
}

// GetBlockTransactionCountByNumber returns the number of transactions in the block.
func (api *PublicAPI) GetBlockTransactionCountByNumber(blockNum ethrpc.BlockNumber) *hexutil.Uint {
	resTxs, err := api.client.GetTransactions(api.ctx, (uint64)(blockNum))
	if err != nil {
		api.Logger.Error("Get Transactions failed", "number", blockNum)
	}

	// TODO: only filter  the eth transactions ?
	n := hexutil.Uint(len(resTxs))
	return &n
}

// GetBalance returns the provided account's balance up to the provided block number.
func (api *PublicAPI) GetBalance(address common.Address, blockNrOrHash ethrpc.BlockNumberOrHash) (*hexutil.Big, error) {
	ethmod := evm.NewV1(api.client)
	res, err := ethmod.Balance(api.ctx, address[:])

	if err != nil {
		api.Logger.Error("Get balance failed")
		return nil, err
	}

	return (*hexutil.Big)(res.ToBigInt()), nil
}

// GetTransactionCount returns the number of transactions the given address has sent for the given block number.
func (api *PublicAPI) GetTransactionCount(ethaddr common.Address, blockNrOrHash ethrpc.BlockNumberOrHash) (*hexutil.Uint64, error) {
	accountsMod := accounts.NewV1(api.client)
	accountsAddr := types.NewAddressRaw(types.AddressV0Secp256k1EthContext, ethaddr[:])
	nonce, err := accountsMod.Nonce(api.ctx, client.RoundLatest, (types.Address)(accountsAddr))

	if err != nil {
		return nil, err
	}

	return (*hexutil.Uint64)(&nonce), nil
}

// GetCode returns the contract code at the given address and block number.
func (api *PublicAPI) GetCode(address common.Address, blockNrOrHash ethrpc.BlockNumberOrHash) (hexutil.Bytes, error) {
	ethmod := evm.NewV1(api.client)
	res, err := ethmod.Code(api.ctx, address[:])

	if err != nil {
		return nil, err
	}

	return res, nil
}

// Call executes the given transaction on the state for the given block number.
// this function doesn't make any changes in the evm state of blockchain
func (api *PublicAPI) Call(args utils.TransactionArgs) (hexutil.Bytes, error) {
	api.Logger.Debug("eth_call", "from", args.From, "to", args.To, "input", args.Input, "value", args.Value)

	res, err := evm.NewV1(api.client).SimulateCall(
		api.ctx,
		args.GasPrice.ToInt().Bytes(),
		uint64(*args.Gas),
		args.From.Bytes(),
		args.To.Bytes(),
		args.Value.ToInt().Bytes(),
		*args.Data)

	if err != nil {
		api.Logger.Error("Failed to execute SimulateCall", "error", err.Error())
		return nil, err
	}

	return (hexutil.Bytes)(res), nil
}

// SendRawTransaction send a raw Ethereum transaction.
func (api *PublicAPI) SendRawTransaction(data hexutil.Bytes) (common.Hash, error) {
	api.Logger.Debug("eth_sendRawTransaction", "length", len(data))

	// Generate an Ethereum transaction that is handled by the EVM module.
	utx := types.UnverifiedTransaction{
		Body: data,
		AuthProofs: []types.AuthProof{
			{Module: "evm.ethereum.v0"},
		},
	}

	_, err := api.client.SubmitTx(api.ctx, &utx)
	if err != nil {
		api.Logger.Error("Failed to SubmitTx", "error", err.Error())
	}

	// Decode the Ethereum transaction.
	ethTx := &ethtypes.Transaction{}
	err = rlp.DecodeBytes(data, ethTx)
	if err != nil {
		api.Logger.Error("Failed to decode rawtx data", "error", err.Error())
	}

	return ethTx.Hash(), nil
}

// EstimateGas returns an estimate of gas usage for the given transaction .
func (api *PublicAPI) EstimateGas(args utils.TransactionArgs, blockNum ethrpc.BlockNumber) (hexutil.Uint64, error) {
	api.Logger.Debug("eth_estimateGas")

	var q quantity.Quantity
	totalFee := new(big.Int).Mul(args.GasPrice.ToInt(), new(big.Int).SetUint64(uint64(*args.Gas)))
	q.FromBigInt(totalFee)
	amount := types.NewBaseUnits(q, types.NativeDenomination)

	fee := &types.Fee{
		Amount: amount,
		Gas:    uint64(*args.Gas),
	}

	ethTxValue := args.Value.ToInt().Bytes()
	ethTxData := args.Data

	var tx *types.Transaction

	if args.To == nil {
		// evm.create
		tx = types.NewTransaction(fee, methodCreate, &evm.Create{
			Value:    ethTxValue,
			InitCode: *ethTxData,
		})
	} else {
		// evm.call
		tx = types.NewTransaction(fee, methodCall, &evm.Call{
			Address: args.To.Bytes(), // contract address, ignore non-contract call
			Value:   ethTxValue,
			Data:    *ethTxData,
		})
	}

	gas, err := core.NewV1(api.client).EstimateGas(api.ctx, uint64(blockNum), tx)
	if err != nil {
		api.Logger.Error("Failed to EstimateGas", "error", err.Error())
		return 0, err
	}

	return hexutil.Uint64(gas), nil
}

// GetTransactionByHash returns the transaction identified by hash.
func (api *PublicAPI) GetTransactionByHash(hash common.Hash) (*utils.RPCTransaction, error) {
	api.Logger.Debug("eth_getTransactionByHash", "hash", hash.Hex())
	dbTxRef, err := api.backend.QueryTransactionRef(hash.String())
	if err != nil {
		api.Logger.Error("Failed to QueryTransactionRef", "hash", hash.String())
	}
	dbTx, err := api.backend.QueryTransaction(hash)
	if err != nil {
		api.Logger.Error("Failed to QueryTransaction", "hash", hash.String())
	}

	to := common.HexToAddress(dbTx.To)

	gasPrice, _ := new(big.Int).SetString(dbTx.GasPrice, 10)
	gasFee, _ := new(big.Int).SetString(dbTx.GasFeeCap, 10)
	gasTip, _ := new(big.Int).SetString(dbTx.GasTipCap, 10)
	value, _ := new(big.Int).SetString(dbTx.Value, 10)
	chainID, _ := new(big.Int).SetString(dbTx.ChainID, 10)
	v, _ := new(big.Int).SetString(dbTx.V, 10)
	r, _ := new(big.Int).SetString(dbTx.R, 10)
	s, _ := new(big.Int).SetString(dbTx.S, 10)

	resTx := &utils.RPCTransaction{
		From:      common.HexToAddress(dbTx.From),
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
		// Accesses: dbTx.AccessList,
		ChainID: (*hexutil.Big)(chainID),
		V:       (*hexutil.Big)(v),
		R:       (*hexutil.Big)(r),
		S:       (*hexutil.Big)(s),
	}

	blockHash := common.HexToHash(dbTxRef.BlockHash)
	txIndex := hexutil.Uint64(dbTxRef.Index)

	if blockHash != (common.Hash{}) {
		resTx.BlockHash = &blockHash
		resTx.BlockNumber = (*hexutil.Big)(new(big.Int).SetUint64(dbTxRef.Round))
		resTx.TransactionIndex = &txIndex
	}

	return resTx, nil
}

// GetTransactionByBlockNumberAndIndex returns the transaction identified by number and index.
func (api *PublicAPI) GetTransactionByBlockNumberAndIndex(blockNum ethrpc.BlockNumber, idx hexutil.Uint) (*utils.RPCTransaction, error) {
	api.Logger.Debug("eth_getTransactionByBlockNumberAndIndex", "number", blockNum, "index", idx)

	resBlock, err := api.client.GetBlock(api.ctx, uint64(blockNum))
	if err != nil {
		api.Logger.Error("Failed to GetBlock", "number", blockNum)
	}

	bhash, _ := resBlock.Header.IORoot.MarshalBinary()
	ethRPCTxs := []*utils.RPCTransaction{}
	txResults, err := api.client.GetTransactionsWithResults(api.ctx, resBlock.Header.Round)
	if err != nil {
		api.Logger.Error("Failed to GetTransactions", "number", blockNum, "error", err.Error())
		return nil, err
	}

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
			api.Logger.Error("Failed to decode UnverifiedTransaction", "height", blockNum, "index", txIndex, "error", err.Error())
			continue
		}

		rpcTx, err := utils.ConstructRPCTransaction(
			ethTx,
			common.BytesToHash(bhash),
			uint64(resBlock.Header.Round),
			uint64(txIndex),
		)
		if err != nil {
			api.Logger.Error("Failed to ConstructRPCTransaction", "hash", ethTx.Hash().Hex(), "error", err.Error())
			continue
		}

		ethRPCTxs = append(ethRPCTxs, rpcTx)
	}

	i := int(idx)
	if i >= len(ethRPCTxs) {
		api.Logger.Info("block txs index out of bound", "index", i)
		return nil, nil
	}

	return ethRPCTxs[i], nil
}

// GetTransactionReceipt returns the transaction receipt by hash.
func (api *PublicAPI) GetTransactionReceipt(hash common.Hash) (map[string]interface{}, error) {
	txRef, err := api.backend.QueryTransactionRef(hash.String())
	if err != nil {
		api.Logger.Error("failed query transaction round and index", "hash", hash.Hex(), "error", err.Error())
		return nil, err
	}
	txResults, err := api.client.GetTransactionsWithResults(api.ctx, txRef.Round)
	if err != nil {
		api.Logger.Error("failed to get transaction results", "round", txRef.Round, "error", err.Error())
		return nil, err
	}
	if len(txResults) == 0 || len(txResults)-1 < int(txRef.Index) {
		return nil, errors.New("out of index")
	}

	utx := txResults[txRef.Index].Tx
	ethTx, err := api.backend.Decode(&utx)
	if err != nil {
		api.Logger.Error("decode utx error", err.Error())
		return nil, err
	}

	// cumulativeGasUsed
	cumulativeGasUsed := uint64(0)
	for i := 0; i <= int(txRef.Index) && i < len(txResults); i++ {
		tx, err := api.backend.Decode(&txResults[i].Tx)
		if err != nil {
			api.Logger.Error("decode utx error", err.Error())
			return nil, err
		}
		cumulativeGasUsed += tx.Gas
	}

	// status
	status := uint8(0)
	if txResults[txRef.Index].Result.IsSuccess() {
		status = uint8(ethtypes.ReceiptStatusSuccessful)
	} else {
		status = uint8(ethtypes.ReceiptStatusFailed)
	}

	// logs
	oasisLogs := []*Log{}
	for i, ev := range txResults[txRef.Index].Events {
		if ev.Code == 1 {
			log := &Log{}
			if err := cbor.Unmarshal(ev.Value, log); err != nil {
				api.Logger.Error("failed unmarshal event value", "index", i)
				continue
			}
			oasisLogs = append(oasisLogs, log)
		}
	}
	logs := logs2EthLogs(oasisLogs, txRef.Round, common.HexToHash(txRef.BlockHash), hash, txRef.Index)

	receipt := map[string]interface{}{
		"status":            status,
		"cumulativeGasUsed": cumulativeGasUsed,
		"logsBloom":         ethtypes.BytesToBloom(ethtypes.LogsBloom(logs)),
		"logs":              logs,
		"transactionHash":   hash,
		"contractAddress":   "",
		"gasUsed":           ethTx.Gas,
		"type":              ethTx.Type,
		"blockHash":         txRef.BlockHash,
		"blockNumber":       txRef.Round,
		"transactionIndex":  txRef.Index,
		"from":              ethTx.From,
		"to":                ethTx.To,
	}
	if logs == nil {
		receipt["logs"] = [][]*ethtypes.Log{}
	}
	if len(ethTx.To) == 0 {
		receipt["contractAddress"] = crypto.CreateAddress(common.HexToAddress(ethTx.From), ethTx.Nonce).Hex()
	}

	return receipt, nil
}

// logs2EthLogs casts the Oasis Logs to a slice of Ethereum Logs.
func logs2EthLogs(logs []*Log, round uint64, blockHash, txHash common.Hash, txIndex uint32) []*ethtypes.Log {
	ethLogs := []*ethtypes.Log{}
	for i := range logs {
		ethLogs = append(ethLogs, toEthereum(logs[i], round, blockHash, txHash, uint(txIndex), uint(i)))
	}
	return ethLogs
}

//toEthereum returns the Ethereum type Log from an Oasis Log.
func toEthereum(log *Log, round uint64, blockHash, txHash common.Hash, txIndex, logIndex uint) *ethtypes.Log {
	topics := []common.Hash{}
	for i := range log.Topics {
		topics = append(topics, log.Topics[i])
	}

	return &ethtypes.Log{
		Address:     log.Address,
		Topics:      log.Topics,
		Data:        log.Data,
		BlockNumber: round,
		TxHash:      txHash,
		TxIndex:     txIndex,
		BlockHash:   blockHash,
		Index:       logIndex,
		Removed:     false,
	}
}
