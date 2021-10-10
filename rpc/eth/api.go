package eth

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	ethrpc "github.com/ethereum/go-ethereum/rpc"

	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
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
	logger := api.Logger.With("web3gateway", "json-rpc")

	logger.Debug("eth_getBlockByNumber", "number", blockNum)
	resBlock, err := api.client.GetBlock(api.ctx, blockNum)
	if err != nil {
		logger.Error("GetBlock failed", "number", blockNum)
	}

	bhash, _ := resBlock.Header.IORoot.MarshalBinary()
	ethRPCTxs := []interface{}{}
	var resTxs []*types.UnverifiedTransaction
	resTxs, err = api.client.GetTransactions(api.ctx, resBlock.Header.Round)
	if err != nil {
		logger.Error("GetTransactions failed", "number", blockNum)
	}

	for i, oasisTx := range resTxs {
		ethTx := &ethtypes.Transaction{}
		err := rlp.DecodeBytes(oasisTx.Body, ethTx)
		if err != nil {
			logger.Error("Failed to decode UnverifiedTransaction", "height", blockNum, "index", i, "error", err.Error())
			continue
		}

		rpcTx, err := utils.ConstructRPCTransaction(
			ethTx,
			common.BytesToHash(bhash),
			uint64(resBlock.Header.Round),
			uint64(i),
		)
		if err != nil {
			logger.Error("Failed to ConstructRPCTransaction", "hash", ethTx.Hash().Hex(), "error", err.Error())
			continue
		}

		ethRPCTxs = append(ethRPCTxs, rpcTx)
	}

	res, err := utils.ConvertToEthBlock(resBlock, ethRPCTxs)
	if err != nil {
		logger.Debug("Failed to ConvertToEthBlock", "height", blockNum, "error", err.Error())
		return nil, err
	}

	return res, nil
}

func (e *PublicAPI) GetBalance(address common.Address, blockNrOrHash ethrpc.BlockNumberOrHash) (*hexutil.Big, error) {
	ethmod := evm.NewV1(e.client)
	res, err := ethmod.Balance(e.ctx, address[:])

	if err != nil {
		e.Logger.Error("Get balance failed")
	}
	return (*hexutil.Big)(new(big.Int).SetBytes(res)), nil
}
