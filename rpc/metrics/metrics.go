package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	durations = promauto.NewHistogramVec(prometheus.HistogramOpts{Name: "oasis_web3_gateway_api_seconds", Buckets: []float64{0.00001, 0.0001, .001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}, Help: "Histogram for the eth API requests duration."}, []string{"method_name"})
	requests  = promauto.NewCounterVec(prometheus.CounterOpts{Name: "oasis_web3_gateway_api_request", Help: "Counter for API requests."}, []string{"method_name"})
	failures  = promauto.NewCounterVec(prometheus.CounterOpts{Name: "oasis_web3_gateway_api_failure", Help: "Counter for API request failures."}, []string{"method_name"})
	successes = promauto.NewCounterVec(prometheus.CounterOpts{Name: "oasis_web3_gateway_api_success", Help: "Counter for API successful requests."}, []string{"method_name"})
	inflight  = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "oasis_web3_gateway_api_inflight", Help: "Number of inflight API request."}, []string{"method_name"})
)

// GetAPIMethodMetrics returns the method metrics for the specified API call.
func GetAPIMethodMetrics(method string) (prometheus.Counter, prometheus.Counter, prometheus.Counter, prometheus.Gauge, prometheus.Observer) {
	return requests.WithLabelValues(method),
		successes.WithLabelValues(method),
		failures.WithLabelValues(method),
		inflight.WithLabelValues(method),
		durations.WithLabelValues(method)
}

// InstrumentCaller instruments the caller method.
//
// The InstrumentCaller should be used the following way:
//
//	   func InstrumentMe() (err error) {
//		  r, s, f, i, d := metrics.GetAPIMethodMetrics("method")
//		  defer metrics.InstrumentCaller(r, s, f, i, d, &err)()
//	       // Do actual work.
//	   }
func InstrumentCaller(count prometheus.Counter, success prometheus.Counter, failures prometheus.Counter, inflight prometheus.Gauge, duration prometheus.Observer, err *error) func() {
	timer := prometheus.NewTimer(duration)
	inflight.Inc()
	return func() {
		// Duration.
		timer.ObserveDuration()
		// Counts.
		count.Inc()
		if err != nil && *err != nil {
			failures.Inc()
		} else {
			success.Inc()
		}
		inflight.Dec()
	}
}
