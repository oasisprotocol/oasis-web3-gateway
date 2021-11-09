package eth

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/filters"
	"github.com/ethereum/go-ethereum/rlp"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-core/go/common/quantity"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/accounts"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/evm"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"

	"github.com/starfishlabs/oasis-evm-web3-gateway/indexer"
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
	"github.com/starfishlabs/oasis-evm-web3-gateway/rpc/utils"
)

var (
	ErrInternalQuery              = errors.New("internal query error")
	ErrBlockNotFound              = errors.New("block not found")
	ErrTransactionNotFound        = errors.New("transaction not found")
	ErrTransactionReceiptNotFound = errors.New("transaction receipt not found")
	ErrOutOfIndex                 = errors.New("out of index")
	ErrMalformedTransaction       = errors.New("malformed transaction")
)

// PublicAPI is the eth_ prefixed set of APIs in the Web3 JSON-RPC spec.
type PublicAPI struct {
	ctx     context.Context
	client  client.RuntimeClient
	backend indexer.Backend
	chainID uint32
	Logger  *logging.Logger
}

// NewPublicAPI creates an instance of the public ETH Web3 API.
func NewPublicAPI(
	ctx context.Context,
	client client.RuntimeClient,
	logger *logging.Logger,
	chainID uint32,
	backend indexer.Backend,
) *PublicAPI {
	return &PublicAPI{
		ctx:     ctx,
		client:  client,
		chainID: chainID,
		Logger:  logger,
		backend: backend,
	}
}

// roundParamFromBlockNum converts special BlockNumber values to the corresponding special round numbers.
func (api *PublicAPI) roundParamFromBlockNum(blockNum ethrpc.BlockNumber) (uint64, error) {
	switch blockNum {
	case ethrpc.PendingBlockNumber:
		// Oasis does not expose a pending block. Use the latest.
		return client.RoundLatest, nil
	case ethrpc.LatestBlockNumber:
		return client.RoundLatest, nil
	case ethrpc.EarliestBlockNumber:
		lrBlk, err := api.client.GetLastRetainedBlock(api.ctx)
		if err != nil {
			return 0, fmt.Errorf("failed to get last retained block: %w", err)
		}
		// TODO: Take Web3 pruning into account.
		return lrBlk.Header.Round, nil
	default:
		return uint64(blockNum), nil
	}
}

// GetBlockByNumber returns the block identified by number.
func (api *PublicAPI) GetBlockByNumber(blockNum ethrpc.BlockNumber, _ bool) (map[string]interface{}, error) {
	api.Logger.Debug("eth_getBlockByNumber", "number", blockNum)

	round, err := api.roundParamFromBlockNum(blockNum)
	if err != nil {
		err = fmt.Errorf("convert block number to round: %w", err)
		api.Logger.Error("GetBlock failed", "number", blockNum, "error", err.Error())
		return nil, err
	}

	blk, err := api.backend.GetBlockByNumber(round)
	if err != nil {
		api.Logger.Error("GetBlock failed", "number", blockNum, "error", err.Error())
		// Block doesn't exist, by web3 spec an empty response should be returned, not an error.
		return nil, ErrInternalQuery
	}

	return utils.ConvertToEthBlock(blk), nil
}

// GetBlockTransactionCountByNumber returns the number of transactions in the block.
func (api *PublicAPI) GetBlockTransactionCountByNumber(blockNum ethrpc.BlockNumber) (hexutil.Uint, error) {
	api.Logger.Debug("eth_getBlockTransactionCountByNumber", "number", blockNum.Int64())

	round, err := api.roundParamFromBlockNum(blockNum)
	if err != nil {
		err = fmt.Errorf("convert block number to round: %w", err)
		api.Logger.Error("GetBlock failed", "number", blockNum, "error", err.Error())
		return 0, err
	}
	n, err := api.backend.GetBlockTransactionCountByNumber(round)
	if err != nil {
		api.Logger.Error("Query failed", "number", blockNum, "error", err.Error())
		return 0, ErrInternalQuery
	}

	return hexutil.Uint(n), nil
}

func (api *PublicAPI) GetStorageAt(address common.Address, position hexutil.Big, blockNum ethrpc.BlockNumber) (hexutil.Big, error) {
	api.Logger.Debug("eth_getStorage", "address", address, "position", position, "block_num", blockNum)

	round, err := api.roundParamFromBlockNum(blockNum)
	if err != nil {
		err = fmt.Errorf("convert block number to round: %w", err)
		api.Logger.Error("GetBlock failed", "number", blockNum, "error", err.Error())
		return hexutil.Big{}, err
	}

	// EVM module takes index as H256, which needs leading zeros.
	position256 := make([]byte, 32)
	// Unmarshalling to hexutil.Big rejects overlong inputs. Verify in `TestRejectOverlong`.
	position.ToInt().FillBytes(position256)

	ethmod := evm.NewV1(api.client)
	res, err := ethmod.Storage(api.ctx, round, address[:], position256)
	if err != nil {
		api.Logger.Error("failed to query storage", "err", err)
		return hexutil.Big{}, ErrInternalQuery
	}
	// Some apps expect no leading zeros, so output as big integer.
	var resultBI big.Int
	resultBI.SetBytes(res)
	return hexutil.Big(resultBI), nil
}

// GetBalance returns the provided account's balance up to the provided block number.
func (api *PublicAPI) GetBalance(address common.Address, blockNum ethrpc.BlockNumber) (*hexutil.Big, error) {
	ethmod := evm.NewV1(api.client)
	round, err := api.roundParamFromBlockNum(blockNum)
	if err != nil {
		return nil, fmt.Errorf("convert block number to round: %w", err)
	}
	res, err := ethmod.Balance(api.ctx, round, address[:])
	if err != nil {
		api.Logger.Error("Get balance failed")
		return nil, err
	}

	return (*hexutil.Big)(res.ToBigInt()), nil
}

// ChainId return the EIP-155  chain id for the current network
func (api *PublicAPI) ChainId() (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(int64(api.chainID))), nil
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
func (api *PublicAPI) GetBlockTransactionCountByHash(blockHash common.Hash) (hexutil.Uint, error) {
	api.Logger.Debug("eth_getBlockTransactionCountByHash", "hash", blockHash.Hex())

	n, err := api.backend.GetBlockTransactionCountByHash(blockHash)
	if err != nil {
		api.Logger.Error("Query failed", "hash", blockHash, "error", err.Error())
		return 0, ErrInternalQuery
	}

	return hexutil.Uint(n), nil
}

// GetTransactionCount returns the number of transactions the given address has sent for the given block number.
func (api *PublicAPI) GetTransactionCount(ethAddr common.Address, blockNum ethrpc.BlockNumber) (*hexutil.Uint64, error) {
	api.Logger.Debug("eth_getTransactionCount", "address", ethAddr.Hex(), "blockNumber", blockNum)
	accountsMod := accounts.NewV1(api.client)
	accountsAddr := types.NewAddressRaw(types.AddressV0Secp256k1EthContext, ethAddr[:])

	round, err := api.roundParamFromBlockNum(blockNum)
	if err != nil {
		return nil, fmt.Errorf("convert block number to round: %w", err)
	}
	nonce, err := accountsMod.Nonce(api.ctx, round, accountsAddr)
	if err != nil {
		return nil, err
	}

	return (*hexutil.Uint64)(&nonce), nil
}

// GetCode returns the contract code at the given address and block number.
func (api *PublicAPI) GetCode(address common.Address, blockNum ethrpc.BlockNumber) (hexutil.Bytes, error) {
	ethmod := evm.NewV1(api.client)
	round, err := api.roundParamFromBlockNum(blockNum)
	if err != nil {
		api.Logger.Error("GetBlock failed", "number", blockNum, "error", err.Error())
		return nil, fmt.Errorf("convert block number to round: %w", err)
	}
	res, err := ethmod.Code(api.ctx, round, address[:])
	if err != nil {
		return nil, err
	}

	return res, nil
}

// Call executes the given transaction on the state for the given block number.
// this function doesn't make any changes in the evm state of blockchain
func (api *PublicAPI) Call(args utils.TransactionArgs, blockNum ethrpc.BlockNumber, _ *utils.StateOverride) (hexutil.Bytes, error) {
	var (
		amount   = []byte{0}
		input    = []byte{}
		sender   = common.Address{1}
		gasPrice = []byte{1}
		// This gas cap should be enough for SimulateCall an ethereum transaction
		gas uint64 = 30_000_000
	)
	round, err := api.roundParamFromBlockNum(blockNum)
	if err != nil {
		api.Logger.Error("GetBlock failed", "number", blockNum, "error", err.Error())
		return nil, fmt.Errorf("convert block number to round: %w", err)
	}

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
		round,
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

	return res, nil
}

// SendRawTransaction send a raw Ethereum transaction.
func (api *PublicAPI) SendRawTransaction(data hexutil.Bytes) (common.Hash, error) {
	api.Logger.Debug("eth_sendRawTransaction", "length", len(data))

	// Decode the Ethereum transaction.
	ethTx := &ethtypes.Transaction{}
	if err := rlp.DecodeBytes(data, ethTx); err != nil {
		api.Logger.Error("failed to decode raw transaction data", "err", err.Error())
		return common.Hash{}, ErrMalformedTransaction
	}

	// Generate an Ethereum transaction that is handled by the EVM module.
	utx := types.UnverifiedTransaction{
		Body: data,
		AuthProofs: []types.AuthProof{
			{Module: "evm.ethereum.v0"},
		},
	}

	err := api.client.SubmitTxNoWait(api.ctx, &utx)
	if err != nil {
		api.Logger.Warn("failed to submit transaction", "err", err.Error())
		return ethTx.Hash(), err
	}

	return ethTx.Hash(), nil
}

// EstimateGas returns an estimate of gas usage for the given transaction .
func (api *PublicAPI) EstimateGas(args utils.TransactionArgs, blockNum *ethrpc.BlockNumber) (hexutil.Uint64, error) {
	api.Logger.Debug("eth_estimateGas", "args", args, "block_num", blockNum)
	if args.From == nil {
		// This may make sense if from not specified
		args.From = &common.Address{}
	}
	if args.Gas == nil {
		// The gas cap is enough for current ethereum transaction
		gasCap := hexutil.Uint64(30_000_000)
		args.Gas = &gasCap
	}
	if args.GasPrice == nil {
		args.GasPrice = (*hexutil.Big)(big.NewInt(1))
	}
	if args.Value == nil {
		args.Value = (*hexutil.Big)(big.NewInt(0))
	}
	if args.Data == nil {
		args.Data = (*hexutil.Bytes)(&[]byte{})
	}

	var q quantity.Quantity
	totalFee := new(big.Int).Mul(args.GasPrice.ToInt(), new(big.Int).SetUint64(uint64(*args.Gas)))
	q.FromBigInt(totalFee)
	amount := types.NewBaseUnits(q, types.NativeDenomination)

	ethTxValue := args.Value.ToInt().Bytes()
	ethTxData := args.Data

	var tx *types.Transaction
	caller := types.NewAddressRaw(types.AddressV0Secp256k1EthContext, args.From[:])
	round := client.RoundLatest

	if blockNum != nil {
		var err error
		round, err = api.roundParamFromBlockNum(*blockNum)
		if err != nil {
			err = fmt.Errorf("convert block number to round: %w", err)
			api.Logger.Error("Failed to EstimateGas", "error", err.Error())
			return 0, err
		}
	}
	if args.To == nil {
		// evm.create
		tx = evm.NewV1(api.client).Create(ethTxValue, *ethTxData).SetFeeAmount(amount).SetFeeGas(uint64(*args.Gas)).GetTransaction()
	} else {
		// evm.call
		tx = evm.NewV1(api.client).Call(args.To.Bytes(), ethTxValue, *ethTxData).SetFeeAmount(amount).SetFeeGas(uint64(*args.Gas)).GetTransaction()
	}

	gas, err := core.NewV1(api.client).EstimateGasForCaller(api.ctx, round, caller, tx)
	if err != nil {
		api.Logger.Error("Failed to EstimateGas", "error", err.Error())
		return 0, ErrInternalQuery
	}

	api.Logger.Debug("eth_estimateGas result", "gas", gas)

	return hexutil.Uint64(gas), nil
}

// GetBlockByHash returns the block identified by hash.
func (api *PublicAPI) GetBlockByHash(blockHash common.Hash, fullTx bool) (map[string]interface{}, error) {
	api.Logger.Debug("eth_getBlockByHash", "hash", blockHash.Hex(), "full", fullTx)
	blk, err := api.backend.GetBlockByHash(blockHash)
	if err != nil {
		api.Logger.Error("Matched block error, block hash: ", blockHash)
		// Block doesn't exist, by web3 spec an empty response should be returned, not an error.
		return nil, ErrInternalQuery
	}

	return utils.ConvertToEthBlock(blk), nil
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
		api.Logger.Error("Failed to NewRPCTransaction", "hash", dbTx.Hash, "error", err.Error())
		return nil, ErrInternalQuery
	}
	return resTx, nil
}

// GetTransactionByHash returns the transaction identified by hash.
func (api *PublicAPI) GetTransactionByHash(hash common.Hash) (*utils.RPCTransaction, error) {
	api.Logger.Debug("eth_getTransactionByHash", "hash", hash.Hex())

	dbTx, err := api.backend.QueryTransaction(hash)
	if err != nil {
		api.Logger.Error("Failed to QueryTransaction", "hash", hash.String(), "error", err.Error())
		// Transaction doesn't exist, by web3 spec an empty response should be returned, not an error.
		return nil, nil
	}

	return api.getRPCTransaction(dbTx)
}

// GetTransactionByBlockHashAndIndex returns the transaction for the given block hash and index.
func (api *PublicAPI) GetTransactionByBlockHashAndIndex(blockHash common.Hash, index uint32) (*utils.RPCTransaction, error) {
	api.Logger.Debug("eth_getTransactionByBlockHashAndIndex", "hash", blockHash.Hex(), "index", index)

	dbBlock, err := api.backend.GetBlockByHash(blockHash)
	if err != nil {
		api.Logger.Error("Matched block error, block hash: ", blockHash)
		// Block doesn't exist, by web3 spec an empty response should be returned, not an error.
		return nil, nil
	}
	l := len(dbBlock.Transactions)
	if uint32(l) <= index {
		api.Logger.Error("out of index", "tx len:", l, "index:", index)
		return nil, ErrOutOfIndex
	}

	return api.getRPCTransaction(dbBlock.Transactions[index])
}

// GetTransactionByBlockNumberAndIndex returns the transaction identified by number and index.
func (api *PublicAPI) GetTransactionByBlockNumberAndIndex(blockNum ethrpc.BlockNumber, index uint32) (*utils.RPCTransaction, error) {
	api.Logger.Debug("eth_getTransactionByBlockNumberAndIndex", "number", blockNum, "index", index)
	round, err := api.roundParamFromBlockNum(blockNum)
	if err != nil {
		err = fmt.Errorf("convert block number to round: %w", err)
		api.Logger.Error("Failed to QueryBlockHash", "number", blockNum, "error", err.Error())
		return nil, err
	}
	blockHash, err := api.backend.QueryBlockHash(round)
	if err != nil {
		api.Logger.Error("Failed to QueryBlockHash", "number", blockNum, "error", err.Error())
		// Block doesn't exist, by web3 spec an empty response should be returned, not an error.
		return nil, nil
	}

	return api.GetTransactionByBlockHashAndIndex(blockHash, index)
}

// GetTransactionReceipt returns the transaction receipt by hash.
func (api *PublicAPI) GetTransactionReceipt(txHash common.Hash) (map[string]interface{}, error) {
	api.Logger.Debug("eth_getTransactionReceipt", "hash", txHash.Hex())
	//dbTx, err := api.backend.QueryTransaction(txHash)
	//if err != nil {
	//	api.Logger.Error("failed query transaction round and index", "hash", txHash.Hex(), "error", err.Error())
	//	// Transaction doesn't exist, don't return an error, but empty response.
	//	return nil, nil
	//}
	//// all tx results in block.
	//txResults, err := api.client.GetTransactionsWithResults(api.ctx, dbTx.Round)
	//if err != nil {
	//	api.Logger.Error("failed to get transaction results", "round", dbTx.Round, "error", err.Error())
	//	return nil, err
	//}
	//
	//// filter out all eth tx results.
	//ethTxResults := []*client.TransactionWithResults{}
	//for _, res := range txResults {
	//	utx := res.Tx
	//	if len(utx.AuthProofs) != 1 || utx.AuthProofs[0].Module != "evm.ethereum.v0" {
	//		continue
	//	}
	//	ethTxResults = append(ethTxResults, res)
	//}
	//if len(ethTxResults) == 0 || len(ethTxResults)-1 < int(dbTx.Index) {
	//	return nil, errors.New("out of index")
	//}
	//
	//utx := txResults[dbTx.Index].Tx
	//ethTx, err := api.backend.Decode(&utx)
	//if err != nil {
	//	api.Logger.Error("decode utx error", err.Error())
	//	return nil, err
	//}
	//
	//// cumulativeGasUsed
	//cumulativeGasUsed := uint64(0)
	//for i := 0; i <= int(dbTx.Index) && i < len(ethTxResults); i++ {
	//	tx, err := api.backend.Decode(&txResults[i].Tx)
	//	if err != nil {
	//		api.Logger.Error("decode utx error", err.Error())
	//		return nil, err
	//	}
	//	cumulativeGasUsed += tx.Gas
	//}
	//
	//// status
	//status := uint8(0)
	//if txResults[dbTx.Index].Result.IsSuccess() {
	//	status = uint8(ethtypes.ReceiptStatusSuccessful)
	//} else {
	//	status = uint8(ethtypes.ReceiptStatusFailed)
	//}
	//
	//// logs
	//oasisLogs := []*indexer.Log{}
	//for i, ev := range txResults[dbTx.Index].Events {
	//	if ev.Code == 1 {
	//		log := &indexer.Log{}
	//		if err := cbor.Unmarshal(ev.Value, log); err != nil {
	//			api.Logger.Error("failed unmarshal event value", "index", i)
	//			continue
	//		}
	//		oasisLogs = append(oasisLogs, log)
	//	}
	//}
	//logs := indexer.Logs2EthLogs(oasisLogs, dbTx.Round, common.HexToHash(dbTx.BlockHash), txHash, dbTx.Index)
	//receipt := map[string]interface{}{
	//	"status":            hexutil.Uint(status),
	//	"cumulativeGasUsed": hexutil.Uint64(cumulativeGasUsed),
	//	"logsBloom":         ethtypes.BytesToBloom(ethtypes.LogsBloom(logs)),
	//	"logs":              logs,
	//	"transactionHash":   txHash.Hex(),
	//	"gasUsed":           hexutil.Uint64(ethTx.Gas),
	//	"type":              hexutil.Uint64(ethTx.Type),
	//	"blockHash":         dbTx.BlockHash,
	//	"blockNumber":       hexutil.Uint64(dbTx.Round),
	//	"transactionIndex":  hexutil.Uint64(dbTx.Index),
	//	"from":              ethTx.FromAddr,
	//	"to":                ethTx.ToAddr,
	//}
	//if logs == nil {
	//	receipt["logs"] = []*ethtypes.Log{}
	//}
	//if len(ethTx.ToAddr) == 0 && txResults[dbTx.Index].Result.IsSuccess() {
	//	var out []byte
	//	if err := cbor.Unmarshal(txResults[dbTx.Index].Result.Ok, &out); err != nil {
	//		return nil, err
	//	}
	//	receipt["contractAddress"] = common.BytesToAddress(out)
	//}

	receipt, err := api.backend.GetTransactionReceipt(txHash)
	if err != nil {
		api.Logger.Error("failed to get transaction receipt, err:", err)
		return nil, nil
	}

	return receipt, nil
}

func (api *PublicAPI) GetLogs(filter filters.FilterCriteria) ([]*ethtypes.Log, error) {
	api.Logger.Debug("eth_getLogs", "filter", filter)

	startRoundInclusive := client.RoundLatest
	endRoundInclusive := client.RoundLatest
	if filter.BlockHash != nil {
		round, err := api.backend.QueryBlockRound(*filter.BlockHash)
		if err != nil {
			return nil, fmt.Errorf("query block round: %w", err)
		}
		startRoundInclusive = round
		endRoundInclusive = round
	} else {
		if filter.FromBlock != nil {
			round, err := api.roundParamFromBlockNum(ethrpc.BlockNumber(filter.FromBlock.Int64()))
			if err != nil {
				return nil, fmt.Errorf("convert from block number to round: %w", err)
			}
			startRoundInclusive = round
		}
		if filter.ToBlock != nil {
			round, err := api.roundParamFromBlockNum(ethrpc.BlockNumber(filter.ToBlock.Int64()))
			if err != nil {
				return nil, fmt.Errorf("convert to block number to round: %w", err)
			}
			endRoundInclusive = round
		}
	}

	// Warning: this is unboundedly expensive
	//var ethLogs []*ethtypes.Log
	//for round := startRoundInclusive; ; /* see explicit break */ round++ {
	//	dbLogs, err := api.backend.GetLogs(*filter.BlockHash)
	//	if err != nil {
	//		api.Logger.Error("get logs, err:", err)
	//		continue
	//	}
	//	// TODO: filter addresses and topics
	//	blockLogs := utils.DbLogs2EthLogs(dbLogs)
	//	ethLogs = append(ethLogs, blockLogs...)
	//
	//	if round == endRoundInclusive {
	//		break
	//	}
	//}

	ethLogs := []*ethtypes.Log{}
	dbLogs, err := api.backend.GetLogs(*filter.BlockHash, startRoundInclusive, endRoundInclusive)
	if err != nil {
		return ethLogs, nil
	}
	blockLogs := utils.DbLogs2EthLogs(dbLogs)
	ethLogs = append(ethLogs, blockLogs...)

	api.Logger.Debug("eth_getLogs response", "resp", ethLogs)

	return ethLogs, nil
}

func (api *PublicAPI) GetBlockHash(blockNum ethrpc.BlockNumber, _ bool) (common.Hash, error) {
	round, err := api.roundParamFromBlockNum(blockNum)
	if err != nil {
		return [32]byte{}, fmt.Errorf("convert block number to round: %w", err)
	}
	return api.backend.QueryBlockHash(round)
}

func (api *PublicAPI) BlockNumber() (hexutil.Uint64, error) {
	api.Logger.Debug("eth_getBlockNumber start")
	blockNumber, err := api.backend.BlockNumber()
	if err != nil {
		api.Logger.Error("failed to get the latest block number", "err:", err)
		return 0, err
	}
	api.Logger.Debug("eth_getBlockNumber get the current number", "blockNumber:", blockNumber)

	return hexutil.Uint64(blockNumber), nil
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
