package eth

import (
	"context"
	"errors"
	"math/big"

	"github.com/oasisprotocol/oasis-core/go/common/cbor"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-core/go/common/quantity"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"

	"github.com/starfishlabs/oasis-evm-web3-gateway/indexer"
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	ethrpc "github.com/ethereum/go-ethereum/rpc"

	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/accounts"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/evm"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"

	"github.com/starfishlabs/oasis-evm-web3-gateway/rpc/utils"
	"github.com/starfishlabs/oasis-evm-web3-gateway/server"
)

const (
	// Callable methods.
	methodCreate = "evm.Create"
	methodCall   = "evm.Call"
)

var (
	ErrInternalQuery = errors.New("internal query error")
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
	config  server.Config
	Logger  *logging.Logger
}

// NewPublicAPI creates an instance of the public ETH Web3 API.
func NewPublicAPI(
	ctx context.Context,
	client client.RuntimeClient,
	logger *logging.Logger,
	config server.Config,
	backend indexer.Backend,
) *PublicAPI {
	return &PublicAPI{
		ctx:     ctx,
		client:  client,
		config:  config,
		Logger:  logger,
		backend: backend,
	}
}

func (api *PublicAPI) getRPCBlock(oasisBlock *block.Block) (map[string]interface{}, error) {
	bhash, _ := oasisBlock.Header.IORoot.MarshalBinary()
	blockNum := oasisBlock.Header.Round
	ethTxs := ethtypes.Transactions{}
	var gasUsed uint64
	var oasisLogs []*Log
	var logs []*ethtypes.Log
	txResults, err := api.client.GetTransactionsWithResults(api.ctx, blockNum)
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

		gasUsed += uint64(ethTx.Gas())
		ethTxs = append(ethTxs, ethTx)

		resEvents := item.Events
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

		logs = logs2EthLogs(oasisLogs, oasisBlock.Header.Round, common.BytesToHash(bhash), ethTx.Hash(), uint32(txIndex))
	}

	res, err := utils.ConvertToEthBlock(oasisBlock, ethTxs, logs, gasUsed)

	if err != nil {
		api.Logger.Debug("Failed to ConvertToEthBlock", "height", blockNum, "error", err.Error())
		return nil, err
	}
	return res, nil
}

// GetBlockByNumber returns the block identified by number.
func (api *PublicAPI) GetBlockByNumber(number string, _ bool) (map[string]interface{}, error) {
	api.Logger.Debug("eth_getBlockByNumber", "number", number)
	bigNumber := big.NewInt(0)
	bigNumber.SetString(number, 10)
	blockNum := bigNumber.Uint64()

	resBlock, err := api.client.GetBlock(api.ctx, blockNum)
	if err != nil {
		api.Logger.Error("GetBlock failed", "number", blockNum)
		return nil, err
	}

	return api.getRPCBlock(resBlock)
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

// ChainId return the EIP-155  chain id for the current network
func (api *PublicAPI) ChainId() (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(int64(api.config.ChainId))), nil
}

// GasPrice returns a suggestion for a gas price for legacy transactions.
func (api *PublicAPI) GasPrice() (*hexutil.Big, error) {
	coremod := core.NewV1(api.client)
	mgp, err := coremod.MinGasPrice(api.ctx)
	if err != nil {
		api.Logger.Error("Qeury MinGasPrice failed", "error", err)
		return nil, err
	}
	nativeMGP := mgp[types.NativeDenomination]
	return (*hexutil.Big)(nativeMGP.ToBigInt()), nil
}

// GetBlockTransactionCountByHash returns the number of transactions in the block identified by hash.
func (api *PublicAPI) GetBlockTransactionCountByHash(blockHash common.Hash) *hexutil.Uint {
	api.Logger.Debug("eth_getBlockTransactionCountByHash", "hash", blockHash.Hex())

	round, err := api.backend.QueryBlockRound(blockHash)
	if err != nil {
		api.Logger.Error("Matched block error, block hash: ", blockHash)
	}

	blk, err := api.client.GetBlock(api.ctx, round)
	if err != nil {
		api.Logger.Error("Matched block error, block round: ", round)
	}

	resTxs, err := api.client.GetTransactions(api.ctx, blk.Header.Round)
	if err != nil {
		api.Logger.Error("Call GetTransactions error")
	}

	// TODO: only filter  the eth transactions ?
	n := hexutil.Uint(len(resTxs))
	return &n
}

// GetTransactionCount returns the number of transactions the given address has sent for the given block number.
func (api *PublicAPI) GetTransactionCount(ethaddr common.Address, blockNum string) (*hexutil.Uint64, error) {
	api.Logger.Debug("eth_getTransactionCount", "address", ethaddr.Hex(), "blockNumber", blockNum)
	accountsMod := accounts.NewV1(api.client)
	accountsAddr := types.NewAddressRaw(types.AddressV0Secp256k1EthContext, ethaddr[:])
	nonce, err := accountsMod.Nonce(api.ctx, client.RoundLatest, accountsAddr)

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
func (api *PublicAPI) Call(args utils.TransactionArgs, _ ethrpc.BlockNumberOrHash, _ *utils.StateOverride) (hexutil.Bytes, error) {
	var (
		amount   = []byte{0}
		input    = []byte{}
		sender   = common.Address{1}
		gasPrice = []byte{1}
		// This gas cap should be enough for SimulateCall an ethereum transaction
		gas uint64 = 30_000_000
	)

	if args.To == nil {
		return []byte{}, errors.New("to address not specified")
	}
	if args.GasPrice != nil {
		gasPrice = args.GasPrice.ToInt().Bytes()
	}
	if args.Gas != nil {
		gas = uint64(*args.Gas)
	}
	if args.Value != nil {
		amount = args.Value.ToInt().Bytes()
	}
	if args.Data != nil {
		input = *args.Data
	}
	if args.From != nil {
		sender = *args.From
	}

	res, err := evm.NewV1(api.client).SimulateCall(
		api.ctx,
		gasPrice,
		gas,
		sender.Bytes(),
		args.To.Bytes(),
		amount,
		input)

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

	err := api.client.SubmitTxNoWait(api.ctx, &utx)
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

// GetBlockByHash returns the block identified by hash.
func (api *PublicAPI) GetBlockByHash(blockHash common.Hash, fullTx bool) (map[string]interface{}, error) {
	api.Logger.Debug("eth_getBlockByHash", "hash", blockHash.Hex(), "full", fullTx)
	round, err := api.backend.QueryBlockRound(blockHash)
	if err != nil {
		api.Logger.Error("Matched block error, block hash: ", blockHash)
		return nil, ErrInternalQuery
	}

	blk, err := api.client.GetBlock(api.ctx, round)
	if err != nil {
		api.Logger.Error("Matched block error, block round: ", round)
		return nil, err
	}

	return api.getRPCBlock(blk)
}

func (api *PublicAPI) getRPCTransaction(dbTx *model.Transaction) (*utils.RPCTransaction, error) {
	dbTxRef, err := api.backend.QueryTransactionRef(dbTx.Hash)
	if err != nil {
		api.Logger.Error("Failed to QueryTransactionRef", "hash", dbTx.Hash)
		return nil, ErrInternalQuery
	}
	blockHash := common.HexToHash(dbTxRef.BlockHash)
	txIndex := hexutil.Uint64(dbTxRef.Index)
	resTx, err := utils.NewRPCTransaction(dbTx, blockHash, dbTxRef.Round, txIndex)
	if err != nil {
		api.Logger.Error("Failed to NewRPCTransaction", "hash", dbTx.Hash)
	}
	return resTx, nil
}

// GetTransactionByHash returns the transaction identified by hash.
func (api *PublicAPI) GetTransactionByHash(hash common.Hash) (*utils.RPCTransaction, error) {
	api.Logger.Debug("eth_getTransactionByHash", "hash", hash.Hex())

	dbTx, err := api.backend.QueryTransaction(hash)
	if err != nil {
		api.Logger.Error("Failed to QueryTransaction", "hash", hash.String())
		return nil, ErrInternalQuery
	}

	if dbTx == nil {
		err = errors.New("transaction not found with the hash: " + hash.Hex())
		return nil, err
	}
	return api.getRPCTransaction(dbTx)
}

// GetTransactionByBlockHashAndIndex returns the transaction for the given block hash and index.
func (api *PublicAPI) GetTransactionByBlockHashAndIndex(blockHash common.Hash, index uint32) (*utils.RPCTransaction, error) {
	round, err := api.backend.QueryBlockRound(blockHash)
	if err != nil {
		api.Logger.Error("Matched block error, block hash: ", blockHash)
		return nil, ErrInternalQuery
	}

	blk, err := api.client.GetBlock(api.ctx, round)
	if err != nil {
		api.Logger.Error("Matched block error, block round: ", round)
		return nil, err
	}

	txs, err := api.client.GetTransactions(api.ctx, blk.Header.Round)
	if err != nil {
		api.Logger.Error("Call GetTransactions error")
		return nil, err
	}

	if index >= uint32(len(txs)) {
		return nil, errors.New("out of tx index")
	}

	dbTx, err := api.backend.Decode(txs[index])
	if err != nil {
		api.Logger.Error("Call Decode error")
		return nil, err
	}

	return api.getRPCTransaction(dbTx)
}

// GetTransactionByBlockNumberAndIndex returns the transaction identified by number and index.
func (api *PublicAPI) GetTransactionByBlockNumberAndIndex(blockNum ethrpc.BlockNumber, index uint32) (*utils.RPCTransaction, error) {
	api.Logger.Debug("eth_getTransactionByBlockNumberAndIndex", "number", blockNum, "index", index)
	blockHash, err := api.backend.QueryBlockHash(uint64(blockNum))
	if err != nil {
		api.Logger.Error("Failed to QueryBlockHash", "number", blockNum)
		return nil, ErrInternalQuery
	}

	return api.GetTransactionByBlockHashAndIndex(blockHash, index)
}

// GetTransactionReceipt returns the transaction receipt by hash.
func (api *PublicAPI) GetTransactionReceipt(txHash common.Hash) (map[string]interface{}, error) {
	api.Logger.Debug("eth_getTransactionReceipt", "hash", txHash.Hex())
	txRef, err := api.backend.QueryTransactionRef(txHash.String())
	if err != nil {
		api.Logger.Error("failed query transaction round and index", "hash", txHash.Hex(), "error", err.Error())
		return nil, ErrInternalQuery
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
	logs := logs2EthLogs(oasisLogs, txRef.Round, common.HexToHash(txRef.BlockHash), txHash, txRef.Index)
	//blockNum:=new(big.Int).SetUint64(txRef.Round).String()
	receipt := map[string]interface{}{
		"status":            hexutil.Uint(status),
		"cumulativeGasUsed": hexutil.Uint64(cumulativeGasUsed),
		"logsBloom":         ethtypes.BytesToBloom(ethtypes.LogsBloom(logs)),
		"logs":              logs,
		"transactionHash":   txHash.Hex(),
		"gasUsed":           hexutil.Uint64(ethTx.Gas),
		"type":              hexutil.Uint64(ethTx.Type),
		"blockHash":         txRef.BlockHash,
		"blockNumber":       hexutil.Uint64(txRef.Round),
		"transactionIndex":  hexutil.Uint64(txRef.Index),
		"from":              ethTx.FromAddr,
		"to":                ethTx.ToAddr,
	}
	if logs == nil {
		receipt["logs"] = [][]*ethtypes.Log{}
	}
	if len(ethTx.ToAddr) == 0 && txResults[txRef.Index].Result.IsSuccess() {
		var out []byte
		if err := cbor.Unmarshal(txResults[txRef.Index].Result.Ok, &out); err != nil {
			return nil, err
		}
		receipt["contractAddress"] = common.BytesToAddress(out)
	}
	api.Logger.Debug("eth_getTransactionReceipt end")
	return receipt, nil
}

// logs2EthLogs casts the Oasis Logs to a slice of Ethereum Logs.
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

func (api *PublicAPI) GetBlockHash(number string, _ bool) (common.Hash, error) {
	bigNumber := big.NewInt(0)
	bigNumber.SetString(number, 10)
	blockNum := bigNumber.Uint64()

	return api.backend.QueryBlockHash(blockNum)
}

func (api *PublicAPI) BlockNumber() (hexutil.Uint64, error) {
	api.Logger.Debug("eth_getBlockNumber start")
	blk, err := api.client.GetBlock(api.ctx, client.RoundLatest)
	if err != nil {
		api.Logger.Debug("eth_getBlockNumber get the latest number", "err", err)
		return 0, err
	}

	api.Logger.Debug("eth_getBlockNumber get the current number", "blockNumber", blk.Header.Round)
	return hexutil.Uint64(blk.Header.Round), nil
}

// Accounts returns the list of accounts available to this node.
func (api *PublicAPI) Accounts() ([]common.Address, error) {
	api.Logger.Debug("eth_accounts")

	addresses := make([]common.Address, 0) // return [] instead of nil if empty

	return addresses, nil
}

// Mining returns whether or not this node is currently mining. Always false.
func (api *PublicAPI) Mining() bool {
	api.Logger.Debug("eth_mining")
	return false
}

// Hashrate returns the current node's hashrate. Always 0.
func (api *PublicAPI) Hashrate() hexutil.Uint64 {
	api.Logger.Debug("eth_hashrate")
	return 0
}
