package rpc

import (
	"context"

	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"

	ethRpc "github.com/ethereum/go-ethereum/rpc"

	"github.com/starfishlabs/oasis-evm-web3-gateway/rpc/eth"
	"github.com/starfishlabs/oasis-evm-web3-gateway/rpc/web3"
)

// GetRPCAPIs returns the list of all APIs
func GetRPCAPIs(
	ctx context.Context,
	client client.RuntimeClient,
) []ethRpc.API {
	var apis []ethRpc.API

	apis = append(apis,
		ethRpc.API{
			Namespace: "web3",
			Version:   "1.0",
			Service:   web3.NewPublicAPI(),
			Public:    true,
		},
		ethRpc.API{
			Namespace: "eth",
			Version:   "1.0",
			Service:   eth.NewPublicAPI(ctx, client),
			Public:    true,
		},
	)

	return apis
}
