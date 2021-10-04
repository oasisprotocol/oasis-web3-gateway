package eth

import (
	"context"

	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"

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
) *PublicAPI {
	return &PublicAPI{
		ctx:    ctx,
		client: client,
	}
}

// GetBlockByNumber returns the block identified by number.
func (api *PublicAPI) GetBlockByNumber(blockNum uint64) (map[string]interface{}, error) {
	logger := api.Logger.With("web3gateway", "json-rpc")

	logger.Debug("eth_getBlockByNumber", "number", blockNum)
	resBlock, err := api.client.GetBlock(api.ctx, blockNum)

	if resBlock == nil {
		return nil, nil
	}

	res, err := api.ConvertToEthBlock(resBlock)
	if err != nil {
		logger.Debug("ConvertToEthBlock failed", "height", blockNum, "error", err.Error())
		return nil, err
	}

	return res, nil
}

// ConvertToEthBlock returns a JSON-RPC compatible Ethereum block from a given Oasis block and its block result.
func (api *PublicAPI) ConvertToEthBlock(
	block *block.Block,
) (map[string]interface{}, error) {
	// TODO tx releated
	res := utils.ConstructBlock(block.Header)
	return res, nil
}
