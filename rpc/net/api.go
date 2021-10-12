package net

import (
	"fmt"
)

// PublicAPI is the net_ prefixed set of APIs in the Web3 JSON-RPC spec.
type PublicAPI struct{}

// NewPublicAPI creates an instance of the Web3 API.
func NewPublicAPI() *PublicAPI {
	return &PublicAPI{}
}

// Version returns the current ethereum protocol version.
func (api *PublicAPI) Version() string {
	return fmt.Sprintf("%d", 1)
}
