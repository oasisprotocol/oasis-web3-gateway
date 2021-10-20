package net

import (
	"fmt"

	"github.com/starfishlabs/oasis-evm-web3-gateway/server"
)

// PublicAPI is the net_ prefixed set of APIs in the Web3 JSON-RPC spec.
type PublicAPI struct{config server.Config}

// NewPublicAPI creates an instance of the Web3 API.
func NewPublicAPI(config server.Config) *PublicAPI {
	return &PublicAPI{config: config}
}

// Version returns the current ethereum protocol version.
func (api *PublicAPI) Version() string {
	return fmt.Sprintf("%d", api.config.ChainId)
}
