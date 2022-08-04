package oasis

import (
	"context"
	"errors"

	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
)

var ErrInternalError = errors.New("internal error")

// API is the net_ prefixed set of APIs in the Web3 JSON-RPC spec.
type API interface {
	// CallDataPublicKey returns the calldata public key for the runtime with the provided ID.
	CallDataPublicKey(ctx context.Context) (*core.CallDataPublicKeyResponse, error)
}

type publicAPI struct {
	client client.RuntimeClient
	Logger *logging.Logger
}

// NewPublicAPI creates an instance of the Web3 API.
func NewPublicAPI(
	client client.RuntimeClient,
	logger *logging.Logger,
) API {
	return &publicAPI{client: client, Logger: logger}
}

func (api *publicAPI) CallDataPublicKey(ctx context.Context) (*core.CallDataPublicKeyResponse, error) {
	logger := api.Logger.With("method", "oasis_callDataPublicKey")
	res, err := core.NewV1(api.client).CallDataPublicKey(ctx)
	if err != nil {
		logger.Error("failed to fetch public key", "err", err)
		return nil, ErrInternalError
	}
	return res, nil
}
