package eth

import (
	"github.com/starfishlabs/oasis-evm-web3-gateway/rpc"
)

// PublicAPI is the eth_ prefixed set of APIs in the Web3 JSON-RPC spec.
type PublicAPI struct {
	backend rpc.Backend
}

// NewPublicAPI creates an instance of the public ETH Web3 API.
func NewPublicAPI(backend rpc.Backend) *PublicAPI {
	return &PublicAPI{
		backend: backend,
	}
}
