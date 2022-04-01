package net

import (
	"fmt"
)

// API is the net_ prefixed set of APIs in the Web3 JSON-RPC spec.
type API interface {
	// Version returns the current ethereum protocol version.
	Version() string
}

type publicAPI struct {
	chainID uint32
}

// NewPublicAPI creates an instance of the Web3 API.
func NewPublicAPI(chainID uint32) API {
	return &publicAPI{chainID}
}

func (api *publicAPI) Version() string {
	return fmt.Sprintf("%d", api.chainID)
}
