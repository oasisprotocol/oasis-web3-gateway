// Package rpc provides the Web3 JSON-RPC APIs for the Oasis Web3 Gateway.
package rpc

import (
	"context"

	ethRpc "github.com/ethereum/go-ethereum/rpc"

	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"

	"github.com/oasisprotocol/oasis-web3-gateway/archive"
	"github.com/oasisprotocol/oasis-web3-gateway/conf"
	eventFilters "github.com/oasisprotocol/oasis-web3-gateway/filters"
	"github.com/oasisprotocol/oasis-web3-gateway/gas"
	"github.com/oasisprotocol/oasis-web3-gateway/indexer"
	"github.com/oasisprotocol/oasis-web3-gateway/rpc/eth"
	"github.com/oasisprotocol/oasis-web3-gateway/rpc/eth/filters"
	ethmetrics "github.com/oasisprotocol/oasis-web3-gateway/rpc/eth/metrics"
	"github.com/oasisprotocol/oasis-web3-gateway/rpc/net"
	"github.com/oasisprotocol/oasis-web3-gateway/rpc/oasis"
	"github.com/oasisprotocol/oasis-web3-gateway/rpc/txpool"
	"github.com/oasisprotocol/oasis-web3-gateway/rpc/web3"
	"github.com/oasisprotocol/oasis-web3-gateway/server"
)

// GetRPCAPIs returns the list of enabled RPC APIs and accompanying health checks.
func GetRPCAPIs(
	ctx context.Context,
	client client.RuntimeClient,
	archiveClient *archive.Client,
	backend indexer.Backend,
	gasPriceOracle gas.Backend,
	config *conf.GatewayConfig,
	eventSystem *eventFilters.EventSystem,
) ([]ethRpc.API, []server.HealthCheck) {
	var apis []ethRpc.API
	var healthChecks []server.HealthCheck

	// Web3 JSON-RPC Spec APIs - always enabled.
	web3Service := web3.NewPublicAPI()
	ethService := eth.NewPublicAPI(client, archiveClient, logging.GetLogger("eth_rpc"), config.ChainID, backend, gasPriceOracle, config.MethodLimits)
	netService := net.NewPublicAPI(config.ChainID)
	txpoolService := txpool.NewPublicAPI()
	filtersService := filters.NewPublicAPI(client, logging.GetLogger("eth_filters"), backend, eventSystem)
	if config.Monitoring.Enabled() {
		web3Service = web3.NewMetricsWrapper(web3Service)
		netService = net.NewMetricsWrapper(netService)
		ethService = ethmetrics.NewMetricsWrapper(ethService, logging.GetLogger("eth_rpc_metrics"), backend)
		txpoolService = txpool.NewMetricsWrapper(txpoolService)
		filtersService = filters.NewMetricsWrapper(filtersService)
	}
	apis = append(apis,
		ethRpc.API{
			Namespace: "web3",
			Service:   web3Service,
		},
		ethRpc.API{
			Namespace: "net",
			Service:   netService,
		},
		ethRpc.API{
			Namespace: "eth",
			Service:   ethService,
		},
		ethRpc.API{
			Namespace: "txpool",
			Service:   txpoolService,
		},
		ethRpc.API{
			Namespace: "eth",
			Service:   filtersService,
		},
	)

	// Configure oasis_ APIs if enabled.
	if config.ExposeOasisRPCs {
		oasisService, oasisHealth := oasis.NewPublicAPI(ctx, client, logging.GetLogger("oasis"))
		if config.Monitoring.Enabled() {
			oasisService = oasis.NewMetricsWrapper(oasisService)
		}

		apis = append(apis, ethRpc.API{
			Namespace: "oasis",
			Service:   oasisService,
		})
		healthChecks = append(healthChecks, oasisHealth)
	}

	return apis, healthChecks
}
