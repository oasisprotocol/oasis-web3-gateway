package txpool

import "github.com/oasisprotocol/oasis-web3-gateway/rpc/metrics"

type metricsWrapper struct {
	api API
}

// Content implements API.
func (m *metricsWrapper) Content() (res map[string][]interface{}, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("txpool_content")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	res, err = m.api.Content()
	return
}

// NewMetricsWrapper returns an instrumanted API service.
func NewMetricsWrapper(api API) API {
	return &metricsWrapper{
		api,
	}
}
