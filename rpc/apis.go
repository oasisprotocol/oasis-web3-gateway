package rpc

import (
	"context"

	ethRpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"

	"github.com/starfishlabs/oasis-evm-web3-gateway/conf"
	eventFilters "github.com/starfishlabs/oasis-evm-web3-gateway/filters"
	"github.com/starfishlabs/oasis-evm-web3-gateway/indexer"
	"github.com/starfishlabs/oasis-evm-web3-gateway/rpc/eth"
	"github.com/starfishlabs/oasis-evm-web3-gateway/rpc/eth/filters"
	"github.com/starfishlabs/oasis-evm-web3-gateway/rpc/net"
	"github.com/starfishlabs/oasis-evm-web3-gateway/rpc/txpool"
	"github.com/starfishlabs/oasis-evm-web3-gateway/rpc/web3"
)

// GetRPCAPIs returns the list of all APIs.
func GetRPCAPIs(
	ctx context.Context,
	client client.RuntimeClient,
	backend indexer.Backend,
	config *conf.GatewayConfig,
	eventSystem *eventFilters.EventSystem,
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
			Namespace: "net",
			Version:   "1.0",
			Service:   net.NewPublicAPI(config.ChainID),
			Public:    true,
		},
		ethRpc.API{
			Namespace: "eth",
			Version:   "1.0",
			Service:   eth.NewPublicAPI(ctx, client, logging.GetLogger("eth_rpc"), config.ChainID, backend, config.MethodLimits),
			Public:    true,
		},
		ethRpc.API{
			Namespace: "txpool",
			Version:   "1.0",
			Service:   txpool.NewPublicAPI(),
			Public:    true,
		},
		ethRpc.API{
			Namespace: "eth",
			Version:   "1.0",
			Service:   filters.NewPublicAPI(ctx, client, logging.GetLogger("eth_filters"), backend, eventSystem),
			Public:    true,
		},
	)

	return apis
}
