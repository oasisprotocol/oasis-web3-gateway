package net

import (
	"fmt"
)

// PublicAPI is the net_ prefixed set of APIs in the Web3 JSON-RPC spec.
type PublicAPI struct {
	chainID uint32
}

// NewPublicAPI creates an instance of the Web3 API.
func NewPublicAPI(chainID uint32) *PublicAPI {
	return &PublicAPI{chainID}
}

// Version returns the current ethereum protocol version.
func (api *PublicAPI) Version() string {
	return fmt.Sprintf("%d", api.chainID)
}
