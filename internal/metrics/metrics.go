package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "x402_requests_total",
			Help: "Total number of requests processed by the x402 gateway",
		},
		[]string{"path", "namespace", "route_name", "payment_status"},
	)

	PaymentAmountTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "x402_payment_amount_total",
			Help: "Total payment amounts processed",
		},
		[]string{"path", "wallet", "network"},
	)

	PaymentVerificationDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "x402_payment_verification_duration_seconds",
			Help:    "Duration of payment verification calls to the facilitator",
			Buckets: prometheus.DefBuckets,
		},
	)

	ProxyRequestDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "x402_proxy_request_duration_seconds",
			Help:    "Duration of proxied requests to backends",
			Buckets: prometheus.DefBuckets,
		},
	)

	ActiveRoutes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "x402_active_routes",
			Help: "Number of active X402Route resources",
		},
	)

	RouteStoreUpdatesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "x402_route_store_updates_total",
			Help: "Total number of route store updates",
		},
	)
)

func init() {
	metrics.Registry.MustRegister(
		RequestsTotal,
		PaymentAmountTotal,
		PaymentVerificationDuration,
		ProxyRequestDuration,
		ActiveRoutes,
		RouteStoreUpdatesTotal,
	)
}
