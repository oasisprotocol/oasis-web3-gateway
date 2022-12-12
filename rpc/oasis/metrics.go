package oasis

import (
	"context"

	"github.com/oasisprotocol/oasis-web3-gateway/rpc/metrics"
)

type metricsWrapper struct {
	api API
}

// PublicKey implements API.
func (m *metricsWrapper) CallDataPublicKey(ctx context.Context) (*CallDataPublicKey, error) {
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
