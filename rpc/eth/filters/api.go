package filters

import (
	"context"

	ethrpc "github.com/ethereum/go-ethereum/rpc"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	ethfilters "github.com/ethereum/go-ethereum/eth/filters"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"

	"github.com/starfishlabs/oasis-evm-web3-gateway/indexer"
)


// PublicAPI is the eth_ prefixed set of APIs in the Web3 JSON-RPC spec.
type PublicFilterAPI struct {
	ctx     context.Context
	client  client.RuntimeClient
	backend indexer.Backend
	Logger  *logging.Logger
}

// NewPublicAPI creates an instance of the public ETH Web3 API with filter func.
func NewPublicAPI(
	ctx context.Context,
	client client.RuntimeClient,
	logger *logging.Logger,
	backend indexer.Backend,
) *PublicFilterAPI {
	return &PublicFilterAPI{
		ctx:     ctx,
		client:  client,
		Logger:  logger,
		backend: backend,
	}
}

func (api *PublicFilterAPI) NewHeads(ctx context.Context) (*ethrpc.Subscription, error) {
	notifier, supported := ethrpc.NotifierFromContext(ctx)
	if !supported {
		return &ethrpc.Subscription{}, ethrpc.ErrNotificationsUnsupported
	}

	rpcSub := notifier.CreateSubscription()
	headers := make(chan []*ethtypes.Header)

	go func() {

		for {
			select {
			case h := <-headers:
				notifier.Notify(rpcSub.ID, h)
			case <-rpcSub.Err():
				// client send an unsubscribe request
				return
			case <-notifier.Closed():
				// connection dropped
				return
			}
		}
	}()

	return rpcSub, nil
}

func (api *PublicFilterAPI) Logs(ctx context.Context, crit ethfilters.FilterCriteria) (*ethrpc.Subscription, error) {
	notifier, supported := ethrpc.NotifierFromContext(ctx)
	if !supported {
		return &ethrpc.Subscription{}, ethrpc.ErrNotificationsUnsupported
	}

	rpcSub := notifier.CreateSubscription()
	matchedLogs := make(chan []*ethtypes.Log)

	go func() {

		for {
			select {
			case logs := <-matchedLogs:
				for _, log := range logs {
					notifier.Notify(rpcSub.ID, &log)
				}
			case <-rpcSub.Err():
				// client send an unsubscribe request
				return
			case <-notifier.Closed():
				// connection dropped
				return
			}
		}
	}()

	return rpcSub, nil
}
