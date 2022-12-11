package web3

import (
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/oasisprotocol/oasis-web3-gateway/rpc/metrics"
)

type metricsWrapper struct {
	api API
}

// ClientVersion implements API.
func (m *metricsWrapper) ClientVersion() string {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("web3_clientVersion")
	defer metrics.InstrumentCaller(r, s, f, i, d, nil)()

	return m.api.ClientVersion()
}

// Sha3 implements API.
func (m *metricsWrapper) Sha3(input hexutil.Bytes) hexutil.Bytes {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("web3_sha3")
	defer metrics.InstrumentCaller(r, s, f, i, d, nil)()

	return m.api.Sha3(input)
}

// NewMetricsWrapper returns an instrumanted API service.
func NewMetricsWrapper(api API) API {
	return &metricsWrapper{
		api,
	}
}
