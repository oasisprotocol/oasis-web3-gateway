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
)

// GetRPCAPIs returns the list of all APIs.
func GetRPCAPIs(
	ctx context.Context,
	client client.RuntimeClient,
	archiveClient *archive.Client,
	backend indexer.Backend,
	gasPriceOracle gas.Backend,
	config *conf.GatewayConfig,
	eventSystem *eventFilters.EventSystem,
) []ethRpc.API {
	var apis []ethRpc.API

	web3Service := web3.NewPublicAPI()
	ethService := eth.NewPublicAPI(client, archiveClient, logging.GetLogger("eth_rpc"), config.ChainID, backend, gasPriceOracle, config.MethodLimits)
	netService := net.NewPublicAPI(config.ChainID)
	txpoolService := txpool.NewPublicAPI()
	filtersService := filters.NewPublicAPI(client, logging.GetLogger("eth_filters"), backend, eventSystem)
	oasisService := oasis.NewPublicAPI(client, logging.GetLogger("oasis"))

	if config.Monitoring.Enabled() {
		web3Service = web3.NewMetricsWrapper(web3Service)
		netService = net.NewMetricsWrapper(netService)
		ethService = ethmetrics.NewMetricsWrapper(ethService, logging.GetLogger("eth_rpc_metrics"), backend)
		txpoolService = txpool.NewMetricsWrapper(txpoolService)
		filtersService = filters.NewMetricsWrapper(filtersService)
		oasisService = oasis.NewMetricsWrapper(oasisService)
	}

	apis = append(apis,
		ethRpc.API{
			Namespace: "web3",
			Version:   "1.0",
			Service:   web3Service,
			Public:    true,
		},
		ethRpc.API{
			Namespace: "net",
			Version:   "1.0",
			Service:   netService,
			Public:    true,
		},
		ethRpc.API{
			Namespace: "eth",
			Version:   "1.0",
			Service:   ethService,
			Public:    true,
		},
		ethRpc.API{
			Namespace: "txpool",
			Version:   "1.0",
			Service:   txpoolService,
			Public:    true,
		},
		ethRpc.API{
			Namespace: "eth",
			Version:   "1.0",
			Service:   filtersService,
			Public:    true,
		},
		ethRpc.API{
			Namespace: "oasis",
			Version:   "1.0",
			Service:   oasisService,
			Public:    config.ExposeOasisRPCs,
		},
	)

	return apis
}
