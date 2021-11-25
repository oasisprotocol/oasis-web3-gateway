package eth

import (
	"context"
	"crypto/sha512"
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
	"github.com/starfishlabs/oasis-evm-web3-gateway/indexer"
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
	"github.com/starfishlabs/oasis-evm-web3-gateway/rpc/utils"
)

func estimateGasDummySigSpec() types.SignatureAddressSpec {
	pk := sha512.Sum512_256([]byte("estimateGas: dummy sigspec"))
	signer := secp256k1.NewSigner(pk[:])
	return types.NewSignatureAddressSpecSecp256k1Eth(signer.Public().(secp256k1.PublicKey))
}

var (
	ErrInternalQuery              = errors.New("internal query error")
	ErrBlockNotFound              = errors.New("block not found")
	ErrTransactionNotFound        = errors.New("transaction not found")
	ErrTransactionReceiptNotFound = errors.New("transaction receipt not found")
	ErrIndexOutOfRange            = errors.New("index out of range")
	ErrMalformedTransaction       = errors.New("malformed transaction")

	// estimateGasSigSpec is a dummy signature spec used by the estimate gas method, as
	// otherwise transactions without signature would be underestimated.
	estimateGasSigSpec = estimateGasDummySigSpec()
)

const revertErrorPrefix = "reverted: "

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
		var earliest uint64
		clrBlk, err := api.client.GetLastRetainedBlock(api.ctx)
		if err != nil {
			api.Logger.Error("failed to get last retained block from client", "error", err)
		}
		ilrRound, err := api.backend.QueryLastRetainedRound()
		if err != nil {
			api.Logger.Error("failed to get last retained block from indexer", "error", err)
		}
		if clrBlk.Header.Round < ilrRound {
			earliest = ilrRound
		} else {
			earliest = clrBlk.Header.Round
		}
		return earliest, nil
	default:
		return uint64(blockNum), nil
	}
}

// GetBlockByNumber returns the block identified by number.
func (api *PublicAPI) GetBlockByNumber(blockNum ethrpc.BlockNumber, fullTx bool) (map[string]interface{}, error) {
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

	return utils.ConvertToEthBlock(blk, fullTx), nil
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
		api.Logger.Error("Query MinGasPrice failed", "error", err)
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

type revertError struct {
	error
	reason string `json:"reason"`
}

// ErrorData returns the ABI encoded error reason.
func (e *revertError) ErrorData() interface{} {
	return e.reason
}

// NewRevertError returns an revert error with ABI encoded revert reason.
func (api *PublicAPI) NewRevertError(revertErr error) *revertError {
	// ABI encoded function.
	abiReason := []byte{0x08, 0xc3, 0x79, 0xa0} // Keccak256("Error(string)")

	// ABI encode the revert reason string.
	revertReason := strings.TrimPrefix(revertErr.Error(), revertErrorPrefix)
	typ, _ := abi.NewType("string", "", nil)
	unpacked, err := (abi.Arguments{{Type: typ}}).Pack(revertReason)
	if err != nil {
		api.Logger.Error("failed to encode revert error", "revert_reason", revertReason, "err", err)
		return &revertError{
			error: revertErr,
		}
	}
	abiReason = append(abiReason, unpacked...)

	return &revertError{
		error:  revertErr,
		reason: hexutil.Encode(abiReason),
	}
}

// Call executes the given transaction on the state for the given block number.
// this function doesn't make any changes in the evm state of blockchain
func (api *PublicAPI) Call(args utils.TransactionArgs, blockNum ethrpc.BlockNumber, _ *utils.StateOverride) (hexutil.Bytes, error) {
	api.Logger.Info("eth_call", "args", args)
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
		input,
	)
	if err != nil {
		if strings.HasPrefix(err.Error(), revertErrorPrefix) {
			revertErr := api.NewRevertError(err)
			api.Logger.Error("failed to execute SimulateCall, reverted", "error", err, "reason", revertErr.reason)
			return nil, revertErr
		}
		api.Logger.Error("failed to execute SimulateCall", "error", err)
		return nil, err
	}

	api.Logger.Info("eth_call response", "args", args, "resp", res)

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
		round, err = api.roundParamFromBlockNum(*blockNum)
		if err != nil {
			err = fmt.Errorf("convert block number to round: %w", err)
			api.Logger.Error("Failed to EstimateGas", "error", err.Error())
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
		api.Logger.Error("Matched block error", "block hash", blockHash)
		// Block doesn't exist, by web3 spec an empty response should be returned, not an error.
		return nil, ErrInternalQuery
	}

	return utils.ConvertToEthBlock(blk, fullTx), nil
}

// getRPCTransaction extracts data from model dbTx and returns.
func (api *PublicAPI) getRPCTransaction(dbTx *model.Transaction) (*utils.RPCTransaction, error) {
	resTx, err := utils.NewRPCTransaction(dbTx)
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
		api.Logger.Error("Matched block error, block hash", blockHash)
		// Block doesn't exist, by web3 spec an empty response should be returned, not an error.
		return nil, nil
	}
	l := len(dbBlock.Transactions)
	if uint32(l) <= index {
		api.Logger.Error("out of index", "tx len", l, "index", index)
		return nil, ErrIndexOutOfRange
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
	receipt, err := api.backend.GetTransactionReceipt(txHash)
	if err != nil {
		// Transaction doesn't exist, don't return an error, but empty response.
		api.Logger.Error("failed to get transaction receipt", "err", err)
		return nil, nil
	}

	return receipt, nil
}

// GetLogs returns the ethereum logs.
func (api *PublicAPI) GetLogs(filter filters.FilterCriteria) ([]*ethtypes.Log, error) {
	api.Logger.Debug("eth_getLogs", "filter", filter)

	ethLogs := []*ethtypes.Log{}
	startRoundInclusive := client.RoundLatest
	endRoundInclusive := client.RoundLatest
	if filter.BlockHash != nil {
		round, err := api.backend.QueryBlockRound(*filter.BlockHash)
		if err != nil {
			return ethLogs, fmt.Errorf("query block round: %w", err)
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

	//TODO: filter addresses and topics

	dbLogs, err := api.backend.GetLogs(*filter.BlockHash, startRoundInclusive, endRoundInclusive)
	if err != nil {
		return ethLogs, nil
	}
	ethLogs = utils.Db2EthLogs(dbLogs)

	api.Logger.Debug("eth_getLogs response", "resp", ethLogs)

	return ethLogs, nil
}

// GetBlockHash returns the block hash by the given number.
func (api *PublicAPI) GetBlockHash(blockNum ethrpc.BlockNumber, _ bool) (common.Hash, error) {
	round, err := api.roundParamFromBlockNum(blockNum)
	if err != nil {
		return [32]byte{}, fmt.Errorf("convert block number to round: %w", err)
	}
	return api.backend.QueryBlockHash(round)
}

// BlockNumber returns the latest block number.
func (api *PublicAPI) BlockNumber() (hexutil.Uint64, error) {
	api.Logger.Debug("eth_getBlockNumber start")
	blockNumber, err := api.backend.BlockNumber()
	if err != nil {
		api.Logger.Error("failed to get the latest block number", "err", err)
		return 0, err
	}
	api.Logger.Debug("eth_getBlockNumber get the the latest block number", "blockNumber", blockNumber)

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
