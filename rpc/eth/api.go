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
	"github.com/oasisprotocol/oasis-core/go/common/cbor"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	roothash "github.com/oasisprotocol/oasis-core/go/roothash/api"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
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
	ErrMalformedTransaction       = errors.New("malformed transaction")

	// estimateGasSigSpec is a dummy signature spec used by the estimate gas method, as
	// otherwise transactions without signature would be underestimated.
	estimateGasSigSpec = estimateGasDummySigSpec()
)

const revertErrorPrefix = "reverted: "

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

func (api *PublicAPI) getRPCBlockData(oasisBlock *block.Block) (uint64, ethtypes.Transactions, uint64, []*ethtypes.Log, error) {
	bhash, _ := oasisBlock.Header.IORoot.MarshalBinary()
	blockNum := oasisBlock.Header.Round
	ethTxs := ethtypes.Transactions{}
	var gasUsed uint64
	var logs []*ethtypes.Log
	txResults, err := api.client.GetTransactionsWithResults(api.ctx, blockNum)
	if err != nil {
		api.Logger.Error("Failed to get transaction results", "number", blockNum, "error", err.Error())
		return 0, nil, 0, nil, err
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

		var oasisLogs []*Log
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
	return blockNum, ethTxs, gasUsed, logs, nil
}

func (api *PublicAPI) getRPCBlock(oasisBlock *block.Block) (map[string]interface{}, error) {
	blockNum, ethTxs, gasUsed, logs, err := api.getRPCBlockData(oasisBlock)
	if err != nil {
		return nil, err
	}

	res, err := utils.ConvertToEthBlock(oasisBlock, ethTxs, logs, gasUsed)
	if err != nil {
		api.Logger.Debug("Failed to ConvertToEthBlock", "height", blockNum, "error", err.Error())
		return nil, err
	}
	return res, nil
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
	resBlock, err := api.client.GetBlock(api.ctx, round)
	if err != nil {
		api.Logger.Error("GetBlock failed", "number", blockNum, "error", err.Error())
		// Block doesn't exist, by web3 spec an empty response should be returned, not an error.
		return nil, ErrInternalQuery
	}

	return api.getRPCBlock(resBlock)
}

// GetBlockTransactionCountByNumber returns the number of transactions in the block.
func (api *PublicAPI) GetBlockTransactionCountByNumber(blockNum ethrpc.BlockNumber) *hexutil.Uint {
	round, err := api.roundParamFromBlockNum(blockNum)
	if err != nil {
		err = fmt.Errorf("convert block number to round: %w", err)
		api.Logger.Error("Get Transactions failed", "number", blockNum, "error", err.Error())
		var dunno hexutil.Uint
		return &dunno
	}
	resTxs, err := api.client.GetTransactions(api.ctx, round)
	if err != nil {
		api.Logger.Error("Get Transactions failed", "number", blockNum, "error", err.Error())
	}

	// TODO: only filter the eth transactions ?
	n := hexutil.Uint(len(resTxs))
	return &n
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

	// TODO: only filter the eth transactions ?
	n := hexutil.Uint(len(resTxs))
	return &n
}

// GetTransactionCount returns the number of transactions the given address has sent for the given block number.
func (api *PublicAPI) GetTransactionCount(ethaddr common.Address, blockNum ethrpc.BlockNumber) (*hexutil.Uint64, error) {
	api.Logger.Debug("eth_getTransactionCount", "address", ethaddr.Hex(), "blockNumber", blockNum)
	accountsMod := accounts.NewV1(api.client)
	accountsAddr := types.NewAddressRaw(types.AddressV0Secp256k1EthContext, ethaddr[:])

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
	round, err := api.backend.QueryBlockRound(blockHash)
	if err != nil {
		api.Logger.Error("Matched block error, block hash: ", blockHash)
		// Block doesn't exist, by web3 spec an empty response should be returned, not an error.
		return nil, nil
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

	round, err := api.backend.QueryBlockRound(blockHash)
	if err != nil {
		api.Logger.Error("Matched block error, block hash: ", blockHash)
		// Block doesn't exist, by web3 spec an empty response should be returned, not an error.
		return nil, nil
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
	txRef, err := api.backend.QueryTransactionRef(txHash.String())
	if err != nil {
		api.Logger.Error("failed query transaction round and index", "hash", txHash.Hex(), "error", err.Error())
		// Transaction doesn't exist, don't return an error, but empty response.
		return nil, nil
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
		"from":              nil,
		"to":                nil,
	}
	if ethTx.FromAddr != "" {
		receipt["from"] = ethTx.FromAddr
	}
	if ethTx.ToAddr != "" {
		receipt["to"] = ethTx.ToAddr
	}
	if logs == nil {
		receipt["logs"] = [][]*ethtypes.Log{}
	}
	if ethTx.ToAddr == "" && txResults[txRef.Index].Result.IsSuccess() {
		var out []byte
		if err := cbor.Unmarshal(txResults[txRef.Index].Result.Ok, &out); err != nil {
			return nil, err
		}
		receipt["contractAddress"] = common.BytesToAddress(out)
	}
	api.Logger.Debug("eth_getTransactionReceipt done", "receipt", receipt)
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
	var ethLogs []*ethtypes.Log
	for round := startRoundInclusive; ; /* see explicit break */ round++ {
		block, err := api.client.GetBlock(api.ctx, round)
		if err != nil {
			if errors.Is(err, roothash.ErrNotFound) && round != client.RoundLatest && endRoundInclusive == client.RoundLatest {
				// We've walked up to the latest round
				break
			}
			return nil, fmt.Errorf("get block %d: %w", round, err)
		}
		_, _, _, blockLogs, err := api.getRPCBlockData(block)
		if err != nil {
			return nil, fmt.Errorf("convert block %d to rpc block: %w", block.Header.Round, err)
		}

		// TODO: filter addresses and topics

		ethLogs = append(ethLogs, blockLogs...)

		if round == endRoundInclusive {
			break
		}
	}

	api.Logger.Debug("eth_getLogs response", "resp", ethLogs)

	return ethLogs, nil
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

func (api *PublicAPI) GetBlockHash(blockNum ethrpc.BlockNumber, _ bool) (common.Hash, error) {
	round, err := api.roundParamFromBlockNum(blockNum)
	if err != nil {
		return [32]byte{}, fmt.Errorf("convert block number to round: %w", err)
	}
	return api.backend.QueryBlockHash(round)
}

func (api *PublicAPI) BlockNumber() (hexutil.Uint64, error) {
	api.Logger.Debug("eth_getBlockNumber start")
	blk, err := api.client.GetBlock(api.ctx, client.RoundLatest)
	if err != nil {
		api.Logger.Error("eth_getBlockNumber get the latest number", "error", err.Error())
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
