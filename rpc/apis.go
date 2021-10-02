package rpc

import (
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/starfishlabs/oasis-evm-web3-gateway/rpc/web3"
)

// GetRPCAPIs returns the list of all APIs
func GetRPCAPIs() []rpc.API {
	// backend := NewBackend()
	var apis []rpc.API

	apis = append(apis,
		rpc.API{
			Namespace: "web3",
			Version:   "1.0",
			Service:   web3.NewPublicAPI(),
			Public:    true,
		},
		rpc.API{
			Namespace: "eth",
			Version:   "1.0",
			// Service:   eth.NewPublicAPI(backend),
			Public: true,
		},
	)

	return apis
}
