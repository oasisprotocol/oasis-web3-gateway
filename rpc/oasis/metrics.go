package oasis

import (
	"context"

	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"

	"github.com/oasisprotocol/emerald-web3-gateway/rpc/metrics"
)

type metricsWrapper struct {
	api API
}

// PublicKey implements API.
func (m *metricsWrapper) CallDataPublicKey(ctx context.Context) (*core.CallDataPublicKeyResponse, error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("oasis_callDataPublicKey")
	defer metrics.InstrumentCaller(r, s, f, i, d, nil)()
	return m.api.CallDataPublicKey(ctx)
}

// NewMetricsWrapper returns an instrumanted API service.
func NewMetricsWrapper(api API) API {
	return &metricsWrapper{
		api,
	}
}
