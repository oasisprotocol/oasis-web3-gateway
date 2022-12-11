package net

import (
	"github.com/oasisprotocol/oasis-web3-gateway/rpc/metrics"
)

type metricsWrapper struct {
	api API
}

// ClientVersion implements API.
func (m *metricsWrapper) Version() string {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("net_version")
	defer metrics.InstrumentCaller(r, s, f, i, d, nil)()

	return m.api.Version()
}

// NewMetricsWrapper returns an instrumanted API service.
func NewMetricsWrapper(api API) API {
	return &metricsWrapper{
		api,
	}
}
