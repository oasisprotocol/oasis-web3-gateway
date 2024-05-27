package eth

import (
	"bytes"
	"context"
	"crypto/sha512"
	"database/sql"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/filters"
	ethrpc "github.com/ethereum/go-ethereum/rpc"

	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/crypto/signature/secp256k1"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/accounts"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/evm"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"

	"github.com/oasisprotocol/oasis-web3-gateway/archive"
	"github.com/oasisprotocol/oasis-web3-gateway/conf"
	"github.com/oasisprotocol/oasis-web3-gateway/gas"
	"github.com/oasisprotocol/oasis-web3-gateway/indexer"
	"github.com/oasisprotocol/oasis-web3-gateway/rpc/utils"
)

func estimateGasDummySigSpec() types.SignatureAddressSpec {
	pk := sha512.Sum512_256([]byte("estimateGas: dummy sigspec"))
	signer := secp256k1.NewSigner(pk[:])
	return types.NewSignatureAddressSpecSecp256k1Eth(signer.Public().(secp256k1.PublicKey))
}

var (
	ErrInternalError        = errors.New("internal error")
	ErrIndexOutOfRange      = errors.New("index out of range")
	ErrMalformedTransaction = errors.New("malformed transaction")
	ErrMalformedBlockNumber = errors.New("malformed blocknumber")
	ErrInvalidRequest       = errors.New("invalid request")

	// estimateGasSigSpec is a dummy signature spec used by the estimate gas method, as
	// otherwise transactions without signature would be underestimated.
	estimateGasSigSpec = estimateGasDummySigSpec()
)

// API is the eth_ prefixed set of APIs in the Web3 JSON-RPC spec.
type API interface {
	// GetBlockByNumber returns the block identified by number.
	GetBlockByNumber(ctx context.Context, blockNum ethrpc.BlockNumber, fullTx bool) (map[string]interface{}, error)
	// GetBlockTransactionCountByNumber returns the number of transactions in the block.
	GetBlockTransactionCountByNumber(ctx context.Context, blockNum ethrpc.BlockNumber) (hexutil.Uint, error)
	// GetStorageAt returns the storage value at the provided position.
	GetStorageAt(ctx context.Context, address common.Address, position hexutil.Big, blockNrOrHash ethrpc.BlockNumberOrHash) (hexutil.Big, error)
	// GetBalance returns the provided account's balance up to the provided block number.
	GetBalance(ctx context.Context, address common.Address, blockNrOrHash ethrpc.BlockNumberOrHash) (*hexutil.Big, error)
	// ChainId return the EIP-155 chain id for the current network.
	ChainId() (*hexutil.Big, error)
	// GasPrice returns a suggestion for a gas price for legacy transactions.
	GasPrice(ctx context.Context) (*hexutil.Big, error)
	// GetBlockTransactionCountByHash returns the number of transactions in the block identified by hash.
	GetBlockTransactionCountByHash(ctx context.Context, blockHash common.Hash) (hexutil.Uint, error)
	// GetTransactionCount returns the number of transactions the given address has sent for the given block number.
	GetTransactionCount(ctx context.Context, ethAddr common.Address, blockNrOrHash ethrpc.BlockNumberOrHash) (*hexutil.Uint64, error)
	// GetCode returns the contract code at the given address and block number.
	GetCode(ctx context.Context, address common.Address, blockNrOrHash ethrpc.BlockNumberOrHash) (hexutil.Bytes, error)
	// Call executes the given transaction on the state for the given block number.
	Call(ctx context.Context, args utils.TransactionArgs, blockNrOrHash ethrpc.BlockNumberOrHash, _ *utils.StateOverride) (hexutil.Bytes, error)
	// SendRawTransaction send a raw Ethereum transaction.
	SendRawTransaction(ctx context.Context, data hexutil.Bytes) (common.Hash, error)
	// EstimateGas returns an estimate of gas usage for the given transaction.
	EstimateGas(ctx context.Context, args utils.TransactionArgs, blockNum *ethrpc.BlockNumber) (hexutil.Uint64, error)
	// GetBlockByHash returns the block identified by hash.
	GetBlockByHash(ctx context.Context, blockHash common.Hash, fullTx bool) (map[string]interface{}, error)
	// GetTransactionByHash returns the transaction identified by hash.
	GetTransactionByHash(ctx context.Context, hash common.Hash) (*utils.RPCTransaction, error)
	// GetTransactionByBlockHashAndIndex returns the transaction for the given block hash and index.
	GetTransactionByBlockHashAndIndex(ctx context.Context, blockHash common.Hash, index hexutil.Uint) (*utils.RPCTransaction, error)
	// GetTransactionByBlockNumberAndIndex returns the transaction identified by number and index.
	GetTransactionByBlockNumberAndIndex(ctx context.Context, blockNum ethrpc.BlockNumber, index hexutil.Uint) (*utils.RPCTransaction, error)
	// GetTransactionReceipt returns the transaction receipt by hash.
	GetTransactionReceipt(ctx context.Context, txHash common.Hash) (map[string]interface{}, error)
	// GetLogs returns the ethereum logs.
	GetLogs(ctx context.Context, filter filters.FilterCriteria) ([]*ethtypes.Log, error)
	// GetBlockHash returns the block hash by the given number.
	GetBlockHash(ctx context.Context, blockNum ethrpc.BlockNumber, _ bool) (common.Hash, error)
	// BlockNumber returns the latest block number.
	BlockNumber(ctx context.Context) (hexutil.Uint64, error)
	// Accounts returns the list of accounts available to this node.
	Accounts() ([]common.Address, error)
	// Mining returns whether or not this node is currently mining.
	Mining() bool
	// Hashrate returns the current node's hashrate.
	Hashrate() hexutil.Uint64
	// Syncing returns false in case the node is currently not syncing with the network, otherwise
	// returns syncing information.
	Syncing(ctx context.Context) (interface{}, error)
}

type publicAPI struct {
	client         client.RuntimeClient
	archiveClient  *archive.Client
	backend        indexer.Backend
	gasPriceOracle gas.Backend
	chainID        uint32
	Logger         *logging.Logger
	methodLimits   *conf.MethodLimits
}

// NewPublicAPI creates an instance of the public ETH Web3 API.
func NewPublicAPI(
	client client.RuntimeClient,
	archiveClient *archive.Client,
	logger *logging.Logger,
	chainID uint32,
	backend indexer.Backend,
	gasPriceOracle gas.Backend,
	methodLimits *conf.MethodLimits,
) API {
	return &publicAPI{
		client:         client,
		archiveClient:  archiveClient,
		chainID:        chainID,
		Logger:         logger,
		backend:        backend,
		gasPriceOracle: gasPriceOracle,
		methodLimits:   methodLimits,
	}
}

// handleStorageError handles the internal storage errors.
func handleStorageError(logger *logging.Logger, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		// By web3 spec an empty response should be returned if the queried block, transaction
		// is not existing.
		logger.Debug("no results found", "err", err)
		return nil
	}
	logger.Error("internal storage error", "err", err)
	return ErrInternalError
}

func (api *publicAPI) shouldQueryArchive(n uint64) bool {
	// If there is no archive node configured, return false.
	if api.archiveClient == nil {
		return false
	}

	return n <= api.archiveClient.LatestBlock()
}

// roundParamFromBlockNum converts special BlockNumber values to the corresponding special round numbers.
func (api *publicAPI) roundParamFromBlockNum(ctx context.Context, logger *logging.Logger, blockNum ethrpc.BlockNumber) (uint64, error) {
	switch blockNum {
	case ethrpc.PendingBlockNumber:
		// Oasis does not expose a pending block. Use the latest.
		return client.RoundLatest, nil
	case ethrpc.LatestBlockNumber:
		return client.RoundLatest, nil
	case ethrpc.EarliestBlockNumber:
		var earliest uint64
		clrBlk, err := api.client.GetLastRetainedBlock(ctx)
		if err != nil {
			logger.Error("failed to get last retained block from client", "err", err)
			return 0, ErrInternalError
		}
		ilrRound, err := api.backend.QueryLastRetainedRound(ctx)
		if err != nil {
			logger.Error("failed to get last retained block from indexer", "err", err)
			return 0, ErrInternalError
		}
		if clrBlk.Header.Round < ilrRound {
			earliest = ilrRound
		} else {
			earliest = clrBlk.Header.Round
		}
		return earliest, nil
	default:
		if int64(blockNum) < 0 {
			logger.Debug("malformed block number", "block_number", blockNum)
			return 0, ErrMalformedBlockNumber
		}

		return uint64(blockNum), nil
	}
}

func (api *publicAPI) GetBlockByNumber(ctx context.Context, blockNum ethrpc.BlockNumber, fullTx bool) (map[string]interface{}, error) {
	logger := api.Logger.With("method", "eth_getBlockByNumber", "block_number", blockNum, "full_tx", fullTx)
	logger.Debug("request")

	round, err := api.roundParamFromBlockNum(ctx, logger, blockNum)
	if err != nil {
		return nil, err
	}

	blk, err := api.backend.GetBlockByRound(ctx, round)
	if err != nil {
		return nil, handleStorageError(logger, err)
	}

	return utils.ConvertToEthBlock(blk, fullTx), nil
}

func (api *publicAPI) GetBlockTransactionCountByNumber(ctx context.Context, blockNum ethrpc.BlockNumber) (hexutil.Uint, error) {
	logger := api.Logger.With("method", "eth_getBlockTransactionCountByNumber", "block_number", blockNum)
	logger.Debug("request")

	round, err := api.roundParamFromBlockNum(ctx, logger, blockNum)
	if err != nil {
		return 0, err
	}
	n, err := api.backend.GetBlockTransactionCountByRound(ctx, round)
	if err != nil {
		return 0, handleStorageError(logger, err)
	}

	return hexutil.Uint(n), nil
}

func (api *publicAPI) GetStorageAt(ctx context.Context, address common.Address, position hexutil.Big, blockNrOrHash ethrpc.BlockNumberOrHash) (hexutil.Big, error) {
	logger := api.Logger.With("method", "eth_getStorageAt", "address", address, "position", position, "block_or_hash", blockNrOrHash)
	logger.Debug("request")

	round, err := api.getBlockRound(ctx, logger, blockNrOrHash)
	if err != nil {
		return hexutil.Big{}, err
	}
	if api.shouldQueryArchive(round) {
		return api.archiveClient.GetStorageAt(ctx, address, position, round)
	}

	// EVM module takes index as H256, which needs leading zeros.
	position256 := make([]byte, 32)
	// Unmarshalling to hexutil.Big rejects overlong inputs. Verify in `TestRejectOverlong`.
	position.ToInt().FillBytes(position256)

	ethmod := evm.NewV1(api.client)
	res, err := ethmod.Storage(ctx, round, address[:], position256)
	if err != nil {
		logger.Error("failed to query storage", "err", err)
		return hexutil.Big{}, ErrInternalError
	}
	// Some apps expect no leading zeros, so output as big integer.
	var resultBI big.Int
	resultBI.SetBytes(res)
	return hexutil.Big(resultBI), nil
}

func (api *publicAPI) GetBalance(ctx context.Context, address common.Address, blockNrOrHash ethrpc.BlockNumberOrHash) (*hexutil.Big, error) {
	logger := api.Logger.With("method", "eth_getBalance", "address", address, "block_or_hash", blockNrOrHash)
	logger.Debug("request")

	round, err := api.getBlockRound(ctx, logger, blockNrOrHash)
	if err != nil {
		return nil, err
	}
	if api.shouldQueryArchive(round) {
		return api.archiveClient.GetBalance(ctx, address, round)
	}

	ethmod := evm.NewV1(api.client)
	res, err := ethmod.Balance(ctx, round, address[:])
	if err != nil {
		logger.Error("ethmod.Balance failed", "round", round, "err", err)
		return nil, ErrInternalError
	}

	return (*hexutil.Big)(res.ToBigInt()), nil
}

//nolint:revive,stylecheck
func (api *publicAPI) ChainId() (*hexutil.Big, error) {
	logger := api.Logger.With("method", "eth_chainId")
	logger.Debug("request")
	return (*hexutil.Big)(big.NewInt(int64(api.chainID))), nil
}

func (api *publicAPI) GasPrice(_ context.Context) (*hexutil.Big, error) {
	logger := api.Logger.With("method", "eth_gasPrice")
	logger.Debug("request")

	return api.gasPriceOracle.GasPrice(), nil
}

func (api *publicAPI) GetBlockTransactionCountByHash(ctx context.Context, blockHash common.Hash) (hexutil.Uint, error) {
	logger := api.Logger.With("method", "eth_getBlockTransactionCountByHash", "block_hash", blockHash.Hex())
	logger.Debug("request")

	n, err := api.backend.GetBlockTransactionCountByHash(ctx, blockHash)
	if err != nil {
		return 0, handleStorageError(logger, err)
	}

	return hexutil.Uint(n), nil
}

func (api *publicAPI) GetTransactionCount(ctx context.Context, ethAddr common.Address, blockNrOrHash ethrpc.BlockNumberOrHash) (*hexutil.Uint64, error) {
	logger := api.Logger.With("method", "eth_getBlockTransactionCount", "address", ethAddr, "block_or_hash", blockNrOrHash)
	logger.Debug("request")

	round, err := api.getBlockRound(ctx, logger, blockNrOrHash)
	if err != nil {
		return nil, err
	}
	if api.shouldQueryArchive(round) {
		return api.archiveClient.GetTransactionCount(ctx, ethAddr, round)
	}

	accountsMod := accounts.NewV1(api.client)
	accountsAddr := types.NewAddressRaw(types.AddressV0Secp256k1EthContext, ethAddr[:])

	nonce, err := accountsMod.Nonce(ctx, round, accountsAddr)
	if err != nil {
		logger.Error("accounts.Nonce failed", "err", err)
		return nil, ErrInternalError
	}

	return (*hexutil.Uint64)(&nonce), nil
}

func (api *publicAPI) GetCode(ctx context.Context, address common.Address, blockNrOrHash ethrpc.BlockNumberOrHash) (hexutil.Bytes, error) {
	logger := api.Logger.With("method", "eth_getCode", "address", address, "block_or_hash", blockNrOrHash)
	logger.Debug("request")

	round, err := api.getBlockRound(ctx, logger, blockNrOrHash)
	if err != nil {
		return nil, err
	}
	if api.shouldQueryArchive(round) {
		return api.archiveClient.GetCode(ctx, address, round)
	}

	ethmod := evm.NewV1(api.client)
	res, err := ethmod.Code(ctx, round, address[:])
	if err != nil {
		logger.Error("ethmod.Code failed", "err", err)
		return nil, err
	}

	return res, nil
}

func (api *publicAPI) Call(ctx context.Context, args utils.TransactionArgs, blockNrOrHash ethrpc.BlockNumberOrHash, _ *utils.StateOverride) (hexutil.Bytes, error) {
	logger := api.Logger.With("method", "eth_call", "block_or_hash", blockNrOrHash)
	logger.Debug("request", "args", args)

	round, err := api.getBlockRound(ctx, logger, blockNrOrHash)
	if err != nil {
		return nil, err
	}
	if api.shouldQueryArchive(round) {
		return api.archiveClient.Call(ctx, args, round)
	}

	var (
		amount   = []byte{0}
		input    = []byte{}
		sender   = common.Address{1}
		gasPrice = []byte{1}
		// This gas cap should be enough for SimulateCall an ethereum transaction
		gas     uint64 = 30_000_000
		toBytes        = []byte{}
	)

	// When simulating a contract deploy the To address may not be specified
	if args.To != nil {
		toBytes = args.To.Bytes()
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
	if args.Input != nil {
		// "data" and "input" are accepted for backwards-compatibility reasons.
		// "input" is the newer name and should be preferred by clients.
		// https://github.com/ethereum/go-ethereum/issues/15628
		input = *args.Input
	}
	if args.From != nil {
		sender = *args.From
	}

	res, err := evm.NewV1(api.client).SimulateCall(
		ctx,            // context
		round,          // round
		gasPrice,       // gasPrice
		gas,            // gasLimit
		sender.Bytes(), // caller
		toBytes,        // address
		amount,         // value
		input,          // data
	)
	if err != nil {
		return nil, api.handleCallFailure(ctx, logger, err)
	}

	logger.Debug("response", "args", args, "resp", res)

	return res, nil
}

func (api *publicAPI) SendRawTransaction(ctx context.Context, data hexutil.Bytes) (common.Hash, error) {
	logger := api.Logger.With("method", "eth_sendRawTransaction")
	logger.Debug("request", "tx", data)

	// Decode the Ethereum transaction.
	ethTx := &ethtypes.Transaction{}
	if err := ethTx.UnmarshalBinary(data); err != nil {
		logger.Debug("failed to decode raw transaction data", "err", err)
		return common.Hash{}, ErrMalformedTransaction
	}

	// Generate an Ethereum transaction that is handled by the EVM module.
	utx := types.UnverifiedTransaction{
		Body: data,
		AuthProofs: []types.AuthProof{
			{Module: "evm.ethereum.v0"},
		},
	}

	err := api.client.SubmitTxNoWait(ctx, &utx)
	if err != nil {
		logger.Debug("failed to submit transaction", "err", err)
		return ethTx.Hash(), err
	}

	return ethTx.Hash(), nil
}

func (api *publicAPI) EstimateGas(ctx context.Context, args utils.TransactionArgs, blockNum *ethrpc.BlockNumber) (hexutil.Uint64, error) {
	logger := api.Logger.With("method", "eth_estimateGas", "block_number", blockNum)
	logger.Debug("request", "args", args)

	if args.From == nil {
		// This may make sense if from not specified
		args.From = &common.Address{}
	}
	if args.Value == nil {
		args.Value = (*hexutil.Big)(big.NewInt(0))
	}
	// "data" and "input" are accepted for backwards-compatibility reasons.
	// "input" is the newer name and should be preferred by clients.
	// https://github.com/ethereum/go-ethereum/issues/15628
	if args.Data != nil && args.Input == nil {
		args.Input = args.Data
	}
	if args.Input == nil {
		args.Input = (*hexutil.Bytes)(&[]byte{})
	}

	ethTxValue := args.Value.ToInt().Bytes()
	ethTxInput := args.Input

	var tx *types.Transaction
	round := client.RoundLatest
	if blockNum != nil {
		var err error
		round, err = api.roundParamFromBlockNum(ctx, logger, *blockNum)
		if err != nil {
			return 0, err
		}
	}
	if args.To == nil {
		// evm.create
		tx = evm.NewV1(api.client).Create(ethTxValue, *ethTxInput).AppendAuthSignature(estimateGasSigSpec, 0).GetTransaction()
	} else {
		// evm.call
		tx = evm.NewV1(api.client).Call(args.To.Bytes(), ethTxValue, *ethTxInput).AppendAuthSignature(estimateGasSigSpec, 0).GetTransaction()
	}

	var ethAddress [20]byte
	copy(ethAddress[:], args.From[:])
	gas, err := core.NewV1(api.client).EstimateGasForCaller(ctx, round, types.CallerAddress{EthAddress: &ethAddress}, tx, true)
	if err != nil {
		logger.Debug("failed", "err", err)
		return 0, err
	}

	logger.Debug("result", "gas", gas)

	return hexutil.Uint64(gas), nil
}

func (api *publicAPI) GetBlockByHash(ctx context.Context, blockHash common.Hash, fullTx bool) (map[string]interface{}, error) {
	logger := api.Logger.With("method", "eth_getBlockByHash", "block_hash", blockHash, "full_tx", fullTx)
	logger.Debug("request")

	blk, err := api.backend.GetBlockByHash(ctx, blockHash)
	if err != nil {
		return nil, handleStorageError(logger, err)
	}

	return utils.ConvertToEthBlock(blk, fullTx), nil
}

func (api *publicAPI) GetTransactionByHash(ctx context.Context, hash common.Hash) (*utils.RPCTransaction, error) {
	logger := api.Logger.With("method", "eth_getTransactionByHash", "hash", hash)
	logger.Debug("request")

	dbTx, err := api.backend.QueryTransaction(ctx, hash)
	if err != nil {
		return nil, handleStorageError(logger, err)
	}

	return utils.NewRPCTransaction(dbTx), nil
}

func (api *publicAPI) GetTransactionByBlockHashAndIndex(ctx context.Context, blockHash common.Hash, index hexutil.Uint) (*utils.RPCTransaction, error) {
	logger := api.Logger.With("method", "eth_getTransactionByBlockHashAndIndex", "block_hash", blockHash, "index", index)
	logger.Debug("request")

	dbBlock, err := api.backend.GetBlockByHash(ctx, blockHash)
	if err != nil {
		return nil, handleStorageError(logger, err)
	}
	if l := uint(len(dbBlock.Transactions)); l <= uint(index) {
		logger.Debug("invalid block transaction index", "num_txs", l)
		return nil, ErrIndexOutOfRange
	}

	return utils.NewRPCTransaction(dbBlock.Transactions[index]), nil
}

func (api *publicAPI) GetTransactionByBlockNumberAndIndex(ctx context.Context, blockNum ethrpc.BlockNumber, index hexutil.Uint) (*utils.RPCTransaction, error) {
	logger := api.Logger.With("method", "eth_getTransactionByNumberAndIndex", "block_number", blockNum, "index", index)
	logger.Debug("request")

	round, err := api.roundParamFromBlockNum(ctx, logger, blockNum)
	if err != nil {
		return nil, err
	}
	blockHash, err := api.backend.QueryBlockHash(ctx, round)
	if err != nil {
		return nil, handleStorageError(logger, err)
	}

	return api.GetTransactionByBlockHashAndIndex(ctx, blockHash, index)
}

func (api *publicAPI) GetTransactionReceipt(ctx context.Context, txHash common.Hash) (map[string]interface{}, error) {
	logger := api.Logger.With("method", "eth_getTransactionReceipt", "hash", txHash)
	logger.Debug("request")

	receipt, err := api.backend.GetTransactionReceipt(ctx, txHash)
	if err != nil {
		return nil, handleStorageError(logger, err)
	}

	return receipt, nil
}

// resolveLatest ensures that the special "latest round" marker is resolved into an actual value.
func (api *publicAPI) resolveLatest(ctx context.Context, round uint64) (uint64, error) {
	switch round {
	case client.RoundLatest:
		// Resolve to actual latest round.
		return api.backend.BlockNumber(ctx)
	default:
		return round, nil
	}
}

// getStartEndRounds is a helper for fetching start and end rounds parameters.
func (api *publicAPI) getStartEndRounds(ctx context.Context, logger *logging.Logger, filter filters.FilterCriteria) (uint64, uint64, error) {
	if filter.BlockHash != nil {
		round, err := api.backend.QueryBlockRound(ctx, *filter.BlockHash)
		if err != nil {
			return 0, 0, fmt.Errorf("query block round: %w", err)
		}
		return round, round, nil
	}

	// Determine start round. If nothing passed, this is the genesis round.
	blockNum := ethrpc.EarliestBlockNumber
	if filter.FromBlock != nil {
		blockNum = ethrpc.BlockNumber(filter.FromBlock.Int64())
	}
	start, err := api.roundParamFromBlockNum(ctx, logger, blockNum)
	if err != nil {
		return 0, 0, err
	}

	// Determine end round. If nothing passed, this is the latest round.
	end := client.RoundLatest
	if filter.ToBlock != nil {
		end, err = api.roundParamFromBlockNum(ctx, logger, ethrpc.BlockNumber(filter.ToBlock.Int64()))
		if err != nil {
			return 0, 0, err
		}
	}

	// Ensure we have concrete round numbers here before proceeding as these rounds are used to
	// determine whether the range of blocks is too large.
	start, err = api.resolveLatest(ctx, start)
	if err != nil {
		return 0, 0, err
	}
	end, err = api.resolveLatest(ctx, end)
	if err != nil {
		return 0, 0, err
	}

	return start, end, nil
}

func (api *publicAPI) GetLogs(ctx context.Context, filter filters.FilterCriteria) ([]*ethtypes.Log, error) {
	logger := api.Logger.With("method", "eth_getLogs")
	logger.Debug("request", "filter", filter)

	startRoundInclusive, endRoundInclusive, err := api.getStartEndRounds(ctx, logger, filter)
	if err != nil {
		return nil, fmt.Errorf("error getting start and end rounds: %w", err)
	}

	if endRoundInclusive < startRoundInclusive {
		return nil, fmt.Errorf("%w: end round greater than start round", ErrInvalidRequest)
	}

	if limit := api.methodLimits.GetLogsMaxRounds; limit != 0 && endRoundInclusive-startRoundInclusive > limit {
		return nil, fmt.Errorf("%w: max allowed of rounds in logs query is: %d", ErrInvalidRequest, limit)
	}

	ethLogs := []*ethtypes.Log{}
	dbLogs, err := api.backend.GetLogs(ctx, startRoundInclusive, endRoundInclusive)
	if err != nil {
		logger.Error("failed to get logs", "err", err)
		return ethLogs, ErrInternalError
	}
	ethLogs = utils.DB2EthLogs(dbLogs)

	// Early return if no further filtering.
	if len(filter.Addresses) == 0 && len(filter.Topics) == 0 {
		logger.Debug("response", "rsp", ethLogs)
		return ethLogs, nil
	}

	filtered := make([]*ethtypes.Log, 0, len(ethLogs))
	for _, log := range ethLogs {
		// Filter by address.
		addressMatch := len(filter.Addresses) == 0
		for _, addr := range filter.Addresses {
			if bytes.Equal(addr[:], log.Address[:]) {
				addressMatch = true
				break
			}
		}
		if !addressMatch {
			continue
		}

		// Filter by topics.
		if !utils.TopicsMatch(log, filter.Topics) {
			continue
		}

		// Log matched all filters.
		filtered = append(filtered, log)
	}

	logger.Debug("response", "rsp", filtered, "all_logs", ethLogs)
	return filtered, nil
}

func (api *publicAPI) GetBlockHash(ctx context.Context, blockNum ethrpc.BlockNumber, _ bool) (common.Hash, error) {
	logger := api.Logger.With("method", "eth_getBlockHash", "block_num", blockNum)
	logger.Debug("request")

	round, err := api.roundParamFromBlockNum(ctx, logger, blockNum)
	if err != nil {
		return [32]byte{}, err
	}
	return api.backend.QueryBlockHash(ctx, round)
}

func (api *publicAPI) BlockNumber(ctx context.Context) (hexutil.Uint64, error) {
	logger := api.Logger.With("method", "eth_getBlockNumber")
	logger.Debug("request")

	blockNumber, err := api.backend.BlockNumber(ctx)
	if err != nil {
		logger.Error("getting latest block number failed", "err", err)
		return 0, ErrInternalError
	}

	logger.Debug("response", "blockNumber", blockNumber)

	return hexutil.Uint64(blockNumber), nil
}

func (api *publicAPI) Accounts() ([]common.Address, error) {
	logger := api.Logger.With("method", "eth_getAccounts")
	logger.Debug("request")

	addresses := make([]common.Address, 0)
	return addresses, nil
}

func (api *publicAPI) Mining() bool {
	logger := api.Logger.With("method", "eth_mining")
	logger.Debug("request")
	return false
}

func (api *publicAPI) Hashrate() hexutil.Uint64 {
	logger := api.Logger.With("method", "eth_hashrate")
	logger.Debug("request")
	return 0
}

func (api *publicAPI) Syncing(_ context.Context) (interface{}, error) {
	logger := api.Logger.With("method", "eth_syncing")
	logger.Debug("request")

	return false, nil
}

// getBlockRound returns the block round from BlockNumberOrHash.
func (api *publicAPI) getBlockRound(ctx context.Context, logger *logging.Logger, blockNrOrHash ethrpc.BlockNumberOrHash) (uint64, error) {
	switch {
	// case if block number and blockhash is specified are handling by the BlockNumberOrHash type.
	case blockNrOrHash.BlockHash == nil && blockNrOrHash.BlockNumber == nil:
		return 0, fmt.Errorf("types BlockHash and BlockNumber cannot be both nil")
	case blockNrOrHash.BlockHash != nil:
		return api.backend.QueryBlockRound(ctx, *blockNrOrHash.BlockHash)
	case blockNrOrHash.BlockNumber != nil:
		return api.roundParamFromBlockNum(ctx, logger, *blockNrOrHash.BlockNumber)
	default:
		return 0, nil
	}
}
