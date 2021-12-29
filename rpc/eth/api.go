package eth

import (
	"bytes"
	"context"
	"crypto/sha512"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/filters"
	"github.com/ethereum/go-ethereum/rlp"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/crypto/signature/secp256k1"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/accounts"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/evm"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"

	"github.com/oasisprotocol/oasis-evm-web3-gateway/conf"
	"github.com/oasisprotocol/oasis-evm-web3-gateway/indexer"
	"github.com/oasisprotocol/oasis-evm-web3-gateway/rpc/utils"
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

const (
	revertErrorPrefix = "reverted: "
)

// PublicAPI is the eth_ prefixed set of APIs in the Web3 JSON-RPC spec.
type PublicAPI struct {
	ctx          context.Context
	client       client.RuntimeClient
	backend      indexer.Backend
	chainID      uint32
	Logger       *logging.Logger
	methodLimits *conf.MethodLimits
}

// NewPublicAPI creates an instance of the public ETH Web3 API.
func NewPublicAPI(
	ctx context.Context,
	client client.RuntimeClient,
	logger *logging.Logger,
	chainID uint32,
	backend indexer.Backend,
	methodLimits *conf.MethodLimits,
) *PublicAPI {
	return &PublicAPI{
		ctx:          ctx,
		client:       client,
		chainID:      chainID,
		Logger:       logger,
		backend:      backend,
		methodLimits: methodLimits,
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

// roundParamFromBlockNum converts special BlockNumber values to the corresponding special round numbers.
func (api *PublicAPI) roundParamFromBlockNum(logger *logging.Logger, blockNum ethrpc.BlockNumber) (uint64, error) {
	switch blockNum {
	case ethrpc.PendingBlockNumber:
		// Oasis does not expose a pending block. Use the latest.
		return client.RoundLatest, nil
	case ethrpc.LatestBlockNumber:
		return client.RoundLatest, nil
	case ethrpc.EarliestBlockNumber:
		var earliest uint64
		clrBlk, err := api.client.GetLastRetainedBlock(api.ctx)
		if err != nil {
			logger.Error("failed to get last retained block from client", "err", err)
			return 0, ErrInternalError
		}
		ilrRound, err := api.backend.QueryLastRetainedRound()
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

// GetBlockByNumber returns the block identified by number.
func (api *PublicAPI) GetBlockByNumber(blockNum ethrpc.BlockNumber, fullTx bool) (map[string]interface{}, error) {
	logger := api.Logger.With("method", "eth_getBlockByNumber", "block_number", blockNum, "full_tx", fullTx)
	logger.Debug("request")

	round, err := api.roundParamFromBlockNum(logger, blockNum)
	if err != nil {
		return nil, err
	}

	blk, err := api.backend.GetBlockByRound(round)
	if err != nil {
		return nil, handleStorageError(logger, err)
	}

	return utils.ConvertToEthBlock(blk, fullTx), nil
}

// GetBlockTransactionCountByNumber returns the number of transactions in the block.
func (api *PublicAPI) GetBlockTransactionCountByNumber(blockNum ethrpc.BlockNumber) (hexutil.Uint, error) {
	logger := api.Logger.With("method", "eth_getBlockTransactionCountByNumber", "block_number", blockNum)
	logger.Debug("request")

	round, err := api.roundParamFromBlockNum(logger, blockNum)
	if err != nil {
		return 0, err
	}
	n, err := api.backend.GetBlockTransactionCountByRound(round)
	if err != nil {
		return 0, handleStorageError(logger, err)
	}

	return hexutil.Uint(n), nil
}

func (api *PublicAPI) GetStorageAt(address common.Address, position hexutil.Big, blockNum ethrpc.BlockNumber) (hexutil.Big, error) {
	logger := api.Logger.With("method", "eth_getStorageAt", "address", address, "position", position, "block_number", blockNum)
	logger.Debug("request")

	round, err := api.roundParamFromBlockNum(logger, blockNum)
	if err != nil {
		return hexutil.Big{}, err
	}
	// EVM module takes index as H256, which needs leading zeros.
	position256 := make([]byte, 32)
	// Unmarshalling to hexutil.Big rejects overlong inputs. Verify in `TestRejectOverlong`.
	position.ToInt().FillBytes(position256)

	ethmod := evm.NewV1(api.client)
	res, err := ethmod.Storage(api.ctx, round, address[:], position256)
	if err != nil {
		logger.Error("failed to query storage", "err", err)
		return hexutil.Big{}, ErrInternalError
	}
	// Some apps expect no leading zeros, so output as big integer.
	var resultBI big.Int
	resultBI.SetBytes(res)
	return hexutil.Big(resultBI), nil
}

// GetBalance returns the provided account's balance up to the provided block number.
func (api *PublicAPI) GetBalance(address common.Address, blockNum ethrpc.BlockNumber) (*hexutil.Big, error) {
	logger := api.Logger.With("method", "eth_getBalance", "address", address, "block_number", blockNum)
	logger.Debug("request")

	ethmod := evm.NewV1(api.client)
	round, err := api.roundParamFromBlockNum(logger, blockNum)
	if err != nil {
		return nil, err
	}
	res, err := ethmod.Balance(api.ctx, round, address[:])
	if err != nil {
		logger.Error("ethmod.Balance failed", "round", round, "err", err)
		return nil, ErrInternalError
	}

	return (*hexutil.Big)(res.ToBigInt()), nil
}

// nolint:revive,stylecheck
// ChainId return the EIP-155 chain id for the current network.
func (api *PublicAPI) ChainId() (*hexutil.Big, error) {
	logger := api.Logger.With("method", "eth_chainId")
	logger.Debug("request")
	return (*hexutil.Big)(big.NewInt(int64(api.chainID))), nil
}

// GasPrice returns a suggestion for a gas price for legacy transactions.
func (api *PublicAPI) GasPrice() (*hexutil.Big, error) {
	logger := api.Logger.With("method", "eth_gasPrice")
	logger.Debug("request")

	coremod := core.NewV1(api.client)
	mgp, err := coremod.MinGasPrice(api.ctx)
	if err != nil {
		logger.Error("core.MinGasPrice failed", "err", err)
		return nil, ErrInternalError
	}
	nativeMGP := mgp[types.NativeDenomination]
	return (*hexutil.Big)(nativeMGP.ToBigInt()), nil
}

// GetBlockTransactionCountByHash returns the number of transactions in the block identified by hash.
func (api *PublicAPI) GetBlockTransactionCountByHash(blockHash common.Hash) (hexutil.Uint, error) {
	logger := api.Logger.With("method", "eth_getBlockTransactionCountByHash", "block_hash", blockHash.Hex())
	logger.Debug("request")

	n, err := api.backend.GetBlockTransactionCountByHash(blockHash)
	if err != nil {
		return 0, handleStorageError(logger, err)
	}

	return hexutil.Uint(n), nil
}

// GetTransactionCount returns the number of transactions the given address has sent for the given block number.
func (api *PublicAPI) GetTransactionCount(ethAddr common.Address, blockNum ethrpc.BlockNumber) (*hexutil.Uint64, error) {
	logger := api.Logger.With("method", "eth_getBlockTransactionCount", "address", ethAddr, "block_number", blockNum)
	logger.Debug("request")

	accountsMod := accounts.NewV1(api.client)
	accountsAddr := types.NewAddressRaw(types.AddressV0Secp256k1EthContext, ethAddr[:])

	round, err := api.roundParamFromBlockNum(logger, blockNum)
	if err != nil {
		return nil, err
	}
	nonce, err := accountsMod.Nonce(api.ctx, round, accountsAddr)
	if err != nil {
		logger.Error("accounts.Nonce failed", "err", err)
		return nil, ErrInternalError
	}

	return (*hexutil.Uint64)(&nonce), nil
}

// GetCode returns the contract code at the given address and block number.
func (api *PublicAPI) GetCode(address common.Address, blockNum ethrpc.BlockNumber) (hexutil.Bytes, error) {
	logger := api.Logger.With("method", "eth_getCode", "address", address, "block_number", blockNum)
	logger.Debug("request")

	ethmod := evm.NewV1(api.client)
	round, err := api.roundParamFromBlockNum(logger, blockNum)
	if err != nil {
		return nil, err
	}
	res, err := ethmod.Code(api.ctx, round, address[:])
	if err != nil {
		logger.Error("ethmod.Code failed", "err", err)
		return nil, err
	}

	return res, nil
}

type RevertError struct {
	error
	Reason string `json:"reason"`
}

// ErrorData returns the ABI encoded error reason.
func (e *RevertError) ErrorData() interface{} {
	return e.Reason
}

// NewRevertError returns an revert error with ABI encoded revert reason.
func (api *PublicAPI) NewRevertError(revertErr error) *RevertError {
	// ABI encoded function.
	abiReason := []byte{0x08, 0xc3, 0x79, 0xa0} // Keccak256("Error(string)")

	// ABI encode the revert Reason string.
	revertReason := strings.TrimPrefix(revertErr.Error(), revertErrorPrefix)
	typ, _ := abi.NewType("string", "", nil)
	unpacked, err := (abi.Arguments{{Type: typ}}).Pack(revertReason)
	if err != nil {
		api.Logger.Error("failed to encode revert error", "revert_reason", revertReason, "err", err)
		return &RevertError{
			error: revertErr,
		}
	}
	abiReason = append(abiReason, unpacked...)

	return &RevertError{
		error:  revertErr,
		Reason: hexutil.Encode(abiReason),
	}
}

// Call executes the given transaction on the state for the given block number.
// This function doesn't make any changes in the evm state of blockchain.
func (api *PublicAPI) Call(args utils.TransactionArgs, blockNrOrHash ethrpc.BlockNumberOrHash, _ *utils.StateOverride) (hexutil.Bytes, error) {
	logger := api.Logger.With("method", "eth_call", "block number or hash", blockNrOrHash)
	logger.Debug("request", "args", args)
	var (
		amount   = []byte{0}
		input    = []byte{}
		sender   = common.Address{1}
		gasPrice = []byte{1}
		// This gas cap should be enough for SimulateCall an ethereum transaction
		gas uint64 = 30_000_000
	)

	round, err := api.getBlockRound(logger, blockNrOrHash)
	if err != nil {
		return nil, err
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
		input,
	)
	if err != nil {
		if strings.HasPrefix(err.Error(), revertErrorPrefix) {
			revertErr := api.NewRevertError(err)
			logger.Debug("failed to execute SimulateCall, reverted", "err", err, "reason", revertErr.Reason)
			return nil, revertErr
		}
		logger.Debug("failed to execute SimulateCall", "err", err)
		return nil, err
	}

	logger.Debug("response", "args", args, "resp", res)

	return res, nil
}

// SendRawTransaction send a raw Ethereum transaction.
func (api *PublicAPI) SendRawTransaction(data hexutil.Bytes) (common.Hash, error) {
	logger := api.Logger.With("method", "eth_sendRawTransaction")
	logger.Debug("request", "length", len(data))

	// Decode the Ethereum transaction.
	ethTx := &ethtypes.Transaction{}
	if err := rlp.DecodeBytes(data, ethTx); err != nil {
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

	err := api.client.SubmitTxNoWait(api.ctx, &utx)
	if err != nil {
		logger.Debug("failed to submit transaction", "err", err)
		return ethTx.Hash(), err
	}

	return ethTx.Hash(), nil
}

// EstimateGas returns an estimate of gas usage for the given transaction .
func (api *PublicAPI) EstimateGas(args utils.TransactionArgs, blockNum *ethrpc.BlockNumber) (hexutil.Uint64, error) {
	logger := api.Logger.With("method", "eth_estimateGas", "block_number", blockNum)
	logger.Debug("request", "args", args)

	if args.From == nil {
		// This may make sense if from not specified
		args.From = &common.Address{}
	}
	if args.Value == nil {
		args.Value = (*hexutil.Big)(big.NewInt(0))
	}
	if args.Data == nil {
		args.Data = (*hexutil.Bytes)(&[]byte{})
	}

	ethTxValue := args.Value.ToInt().Bytes()
	ethTxData := args.Data

	var tx *types.Transaction
	round := client.RoundLatest
	if blockNum != nil {
		var err error
		round, err = api.roundParamFromBlockNum(logger, *blockNum)
		if err != nil {
			return 0, err
		}
	}
	if args.To == nil {
		// evm.create
		tx = evm.NewV1(api.client).Create(ethTxValue, *ethTxData).AppendAuthSignature(estimateGasSigSpec, 0).GetTransaction()
	} else {
		// evm.call
		tx = evm.NewV1(api.client).Call(args.To.Bytes(), ethTxValue, *ethTxData).AppendAuthSignature(estimateGasSigSpec, 0).GetTransaction()
	}

	var ethAddress [20]byte
	copy(ethAddress[:], args.From[:])
	gas, err := core.NewV1(api.client).EstimateGasForCaller(api.ctx, round, types.CallerAddress{EthAddress: &ethAddress}, tx)
	if err != nil {
		logger.Debug("failed", "err", err)
		return 0, err
	}

	logger.Debug("result", "gas", gas)

	return hexutil.Uint64(gas), nil
}

// GetBlockByHash returns the block identified by hash.
func (api *PublicAPI) GetBlockByHash(blockHash common.Hash, fullTx bool) (map[string]interface{}, error) {
	logger := api.Logger.With("method", "eth_getBlockByHash", "block_hash", blockHash, "full_tx", fullTx)
	logger.Debug("request")

	blk, err := api.backend.GetBlockByHash(blockHash)
	if err != nil {
		return nil, handleStorageError(logger, err)
	}

	return utils.ConvertToEthBlock(blk, fullTx), nil
}

// GetTransactionByHash returns the transaction identified by hash.
func (api *PublicAPI) GetTransactionByHash(hash common.Hash) (*utils.RPCTransaction, error) {
	logger := api.Logger.With("method", "eth_getTransactionByHash", "hash", hash)
	logger.Debug("request")

	dbTx, err := api.backend.QueryTransaction(hash)
	if err != nil {
		return nil, handleStorageError(logger, err)
	}

	return utils.NewRPCTransaction(dbTx), nil
}

// GetTransactionByBlockHashAndIndex returns the transaction for the given block hash and index.
func (api *PublicAPI) GetTransactionByBlockHashAndIndex(blockHash common.Hash, index uint32) (*utils.RPCTransaction, error) {
	logger := api.Logger.With("method", "eth_getTransactionByBlockHashAndIndex", "block_hash", blockHash, "index", index)
	logger.Debug("request")

	dbBlock, err := api.backend.GetBlockByHash(blockHash)
	if err != nil {
		return nil, handleStorageError(logger, err)
	}
	l := len(dbBlock.Transactions)
	if uint32(l) <= index {
		logger.Debug("invalid block transaction index", "num_txs", l)
		return nil, ErrIndexOutOfRange
	}

	return utils.NewRPCTransaction(dbBlock.Transactions[index]), nil
}

// GetTransactionByBlockNumberAndIndex returns the transaction identified by number and index.
func (api *PublicAPI) GetTransactionByBlockNumberAndIndex(blockNum ethrpc.BlockNumber, index uint32) (*utils.RPCTransaction, error) {
	logger := api.Logger.With("method", "eth_getTransactionByNumberAndIndex", "block_number", blockNum, "index", index)
	logger.Debug("request")

	round, err := api.roundParamFromBlockNum(logger, blockNum)
	if err != nil {
		return nil, err
	}
	blockHash, err := api.backend.QueryBlockHash(round)
	if err != nil {
		return nil, handleStorageError(logger, err)
	}

	return api.GetTransactionByBlockHashAndIndex(blockHash, index)
}

// GetTransactionReceipt returns the transaction receipt by hash.
func (api *PublicAPI) GetTransactionReceipt(txHash common.Hash) (map[string]interface{}, error) {
	logger := api.Logger.With("method", "eth_getTransactionReceipt", "hash", txHash)
	logger.Debug("request")

	receipt, err := api.backend.GetTransactionReceipt(txHash)
	if err != nil {
		return nil, handleStorageError(logger, err)
	}

	return receipt, nil
}

// getStartEndRounds is a helper for fetching start and end rounds parameters.
func (api *PublicAPI) getStartEndRounds(logger *logging.Logger, filter filters.FilterCriteria) (uint64, uint64, error) {
	if filter.BlockHash != nil {
		round, err := api.backend.QueryBlockRound(*filter.BlockHash)
		if err != nil {
			return 0, 0, fmt.Errorf("query block round: %w", err)
		}
		return round, round, nil
	}

	start := client.RoundLatest
	end := client.RoundLatest
	if filter.FromBlock != nil {
		round, err := api.roundParamFromBlockNum(logger, ethrpc.BlockNumber(filter.FromBlock.Int64()))
		if err != nil {
			return 0, 0, err
		}
		start = round
	}
	if filter.ToBlock != nil {
		round, err := api.roundParamFromBlockNum(logger, ethrpc.BlockNumber(filter.ToBlock.Int64()))
		if err != nil {
			return 0, 0, err
		}
		end = round
	}

	return start, end, nil
}

// GetLogs returns the ethereum logs.
func (api *PublicAPI) GetLogs(filter filters.FilterCriteria) ([]*ethtypes.Log, error) {
	logger := api.Logger.With("method", "eth_getLogs")
	logger.Debug("request", "filter", filter)

	startRoundInclusive, endRoundInclusive, err := api.getStartEndRounds(logger, filter)
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
	dbLogs, err := api.backend.GetLogs(startRoundInclusive, endRoundInclusive)
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
		addressMatch := len(filtered) == 0
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

// GetBlockHash returns the block hash by the given number.
func (api *PublicAPI) GetBlockHash(blockNum ethrpc.BlockNumber, _ bool) (common.Hash, error) {
	logger := api.Logger.With("method", "eth_getBlockHash", "block_num", blockNum)
	logger.Debug("request")

	round, err := api.roundParamFromBlockNum(logger, blockNum)
	if err != nil {
		return [32]byte{}, err
	}
	return api.backend.QueryBlockHash(round)
}

// BlockNumber returns the latest block number.
func (api *PublicAPI) BlockNumber() (hexutil.Uint64, error) {
	logger := api.Logger.With("method", "eth_getBlockNumber")
	logger.Debug("request")

	blockNumber, err := api.backend.BlockNumber()
	if err != nil {
		logger.Error("getting latest block number failed", "err", err)
		return 0, ErrInternalError
	}

	logger.Debug("response", "blockNumber", blockNumber)

	return hexutil.Uint64(blockNumber), nil
}

// Accounts returns the list of accounts available to this node.
func (api *PublicAPI) Accounts() ([]common.Address, error) {
	logger := api.Logger.With("method", "eth_getAccounts")
	logger.Debug("request")

	addresses := make([]common.Address, 0)
	return addresses, nil
}

// Mining returns whether or not this node is currently mining. Always false.
func (api *PublicAPI) Mining() bool {
	logger := api.Logger.With("method", "eth_mining")
	logger.Debug("request")
	return false
}

// Hashrate returns the current node's hashrate. Always 0.
func (api *PublicAPI) Hashrate() hexutil.Uint64 {
	logger := api.Logger.With("method", "eth_hashrate")
	logger.Debug("request")
	return 0
}

// getBlockRound returns the block round from BlockNumberOrHash
func (api *PublicAPI) getBlockRound(logger *logging.Logger, blockNrOrHash ethrpc.BlockNumberOrHash) (uint64, error) {
	switch {
	case blockNrOrHash.BlockHash == nil && blockNrOrHash.BlockNumber == nil:
		return 0, fmt.Errorf("types BlockHash and BlockNumber cannot be both nil")
	case blockNrOrHash.BlockHash != nil:
		round, err := api.backend.QueryBlockRound(*blockNrOrHash.BlockHash)
		return round, err
	case blockNrOrHash.BlockNumber != nil:
		round, err := api.roundParamFromBlockNum(logger, *blockNrOrHash.BlockNumber)
		return round, err
	default:
		return 0, nil
	}
}