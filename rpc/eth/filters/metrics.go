package filters

import (
	"context"

	ethfilters "github.com/ethereum/go-ethereum/eth/filters"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/oasisprotocol/oasis-web3-gateway/rpc/metrics"
)

var (
	durations = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "oasis_oasis_web3_gateway_subscription_seconds",
			// Buckets ranging from 1 second to 24 hours.
			Buckets: []float64{1, 10, 30, 60, 600, 1800, 3600, 7200, 21600, 86400},
			Help:    "Histogram for the eth subscription API subscriptions duration.",
		},
		[]string{"method_name"},
	)
	inflightSubs = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "oasis_oasis_web3_gateway_subscription_inflight",
			Help: "Number of concurrent eth inflight subscriptions.",
		},
		[]string{"method_name"},
	)
)

type metricsWrapper struct {
	api API
}

// Logs implements API.
func (m *metricsWrapper) Logs(ctx context.Context, crit ethfilters.FilterCriteria) (rpcSub *ethrpc.Subscription, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_logs")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	inflightSubs := inflightSubs.WithLabelValues("eth_logs")
	duration := durations.WithLabelValues("eth_logs")

	// Measure subscirpiton duration and concurrent subscriptions.
	timer := prometheus.NewTimer(duration)
	inflightSubs.Inc()

	rpcSub, err = m.api.Logs(ctx, crit)
	go func() {
		// Wait for subscription to unsubscribe.
		<-rpcSub.Err()
		timer.ObserveDuration()
		// Decrement in-flight.
		inflightSubs.Dec()
	}()

	return rpcSub, err
}

// NewHeads implements API.
func (m *metricsWrapper) NewHeads(ctx context.Context) (rpcSub *ethrpc.Subscription, err error) {
	r, s, f, i, d := metrics.GetAPIMethodMetrics("eth_newHeads")
	defer metrics.InstrumentCaller(r, s, f, i, d, &err)()

	inflightSubs := inflightSubs.WithLabelValues("eth_newHeads")
	duration := durations.WithLabelValues("eth_newHeads")

	// Measure subscirpiton duration and concurrent subscriptions.
	timer := prometheus.NewTimer(duration)
	inflightSubs.Inc()

	rpcSub, err = m.api.NewHeads(ctx)
	go func() {
		// Wait for subscription to unsubscribe.
		<-rpcSub.Err()
		timer.ObserveDuration()
		// Decrement in-flight.
		inflightSubs.Dec()
	}()

	return rpcSub, err
}

// NewMetricsWrapper returns an instrumanted API service.
func NewMetricsWrapper(api API) API {
	return &metricsWrapper{
		api,
	}
}
