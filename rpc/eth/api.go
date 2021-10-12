package eth

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	ethrpc "github.com/ethereum/go-ethereum/rpc"

	"github.com/oasisprotocol/oasis-core/go/common/crypto/address"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/accounts"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/evm"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"

	"github.com/starfishlabs/oasis-evm-web3-gateway/rpc/utils"
)

// PublicAPI is the eth_ prefixed set of APIs in the Web3 JSON-RPC spec.
type PublicAPI struct {
	ctx    context.Context
	client client.RuntimeClient

	Logger *logging.Logger
}

// NewPublicAPI creates an instance of the public ETH Web3 API.
func NewPublicAPI(
	ctx context.Context,
	client client.RuntimeClient,
	logger *logging.Logger,
) *PublicAPI {
	return &PublicAPI{
		ctx:    ctx,
		client: client,
		Logger: logger,
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
	var resTxs []*types.UnverifiedTransaction
	resTxs, err = api.client.GetTransactions(api.ctx, resBlock.Header.Round)
	if err != nil {
		api.Logger.Error("GetTransactions failed", "number", blockNum)
	}

	for i, oasisTx := range resTxs {
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
			api.Logger.Error("Failed to decode UnverifiedTransaction", "height", blockNum, "index", i, "error", err.Error())
			continue
		}

		rpcTx, err := utils.ConstructRPCTransaction(
			ethTx,
			common.BytesToHash(bhash),
			uint64(resBlock.Header.Round),
			uint64(i),
		)
		if err != nil {
			api.Logger.Error("Failed to ConstructRPCTransaction", "hash", ethTx.Hash().Hex(), "error", err.Error())
			continue
		}

		ethRPCTxs = append(ethRPCTxs, rpcTx)
	}

	res, err := utils.ConvertToEthBlock(resBlock, ethRPCTxs)
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

	return (*hexutil.Big)(new(big.Int).SetBytes(res)), nil
}

// GetTransactionCount returns the number of transactions the given address has sent for the given block number.
func (api *PublicAPI) GetTransactionCount(ethaddr common.Address, blockNrOrHash ethrpc.BlockNumberOrHash) (*hexutil.Uint64, error) {
	accountsMod := accounts.NewV1(api.client)
	accountsAddr := address.NewAddress(types.AddressV0ModuleContext, ethaddr[:])
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
func (api *PublicAPI) Call(args utils.TransactionArgs, blockNrOrHash ethrpc.BlockNumberOrHash) (hexutil.Bytes, error) {
	api.Logger.Info("EVM call", "from", args.From, "to", args.To, "input", args.Input, "value", args.Value)

	return (hexutil.Bytes)([]byte{}), nil
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
func (api *PublicAPI) EstimateGas(args utils.TransactionArgs, blockNrOptional *ethrpc.BlockNumber) (hexutil.Uint64, error) {
	api.Logger.Debug("eth_estimateGas")

	return 0, nil
}
