package oasis

import (
	"context"
	"errors"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
	"github.com/oasisprotocol/oasis-web3-gateway/server"
)

var ErrInternalError = errors.New("internal error")

// API is the oasis_ prefixed set of APIs.
type API interface {
	// CallDataPublicKey returns the calldata public key for the runtime with the provided ID.
	CallDataPublicKey(ctx context.Context) (*CallDataPublicKey, error)
}

// CallDataPublicKey is the public key alongside the key manager's signature.
//
// This is a flattened `core.CallDataPublicKeyResponse` with hex-encoded bytes for easy consumption by Web3 clients.
type CallDataPublicKey struct {
	// PublicKey is the requested public key.
	PublicKey hexutil.Bytes `json:"key"`
	// Checksum is the checksum of the key manager state.
	Checksum hexutil.Bytes `json:"checksum"`
	// Signature is the Sign(sk, (key || checksum)) from the key manager.
	Signature hexutil.Bytes `json:"signature"`
	// Epoch is the epoch of the ephemeral runtime key.
	Epoch uint64 `json:"epoch,omitempty"`
}

type publicAPI struct {
	client client.RuntimeClient
	logger *logging.Logger
}

// NewPublicAPI creates an instance of the Web3 API and accompanying health check.
func NewPublicAPI(
	ctx context.Context,
	client client.RuntimeClient,
	logger *logging.Logger,
) (API, server.HealthCheck) {
	health := &healthChecker{ctx: ctx, client: client, logger: logger}
	go health.run()

	return &publicAPI{client: client, logger: logger}, health
}

func (api *publicAPI) CallDataPublicKey(ctx context.Context) (*CallDataPublicKey, error) {
	logger := api.logger.With("method", "oasis_callDataPublicKey")
	res, err := core.NewV1(api.client).CallDataPublicKey(ctx)
	if err != nil {
		logger.Error("failed to fetch public key", "err", err)
		return nil, ErrInternalError
	}
	return &CallDataPublicKey{
		PublicKey: res.PublicKey.PublicKey[:],
		Checksum:  res.PublicKey.Checksum,
		Signature: res.PublicKey.Signature[:],
		Epoch:     res.Epoch,
	}, nil
}
