package gateway

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/razvanmacovei/x402-k8s-operator/internal/metrics"
	"github.com/razvanmacovei/x402-k8s-operator/internal/routestore"
)

// Handler handles incoming HTTP requests, performing route matching,
// payment verification, and proxying to backends.
type Handler struct {
	store *routestore.Store
}

// NewHandler creates a new gateway handler.
func NewHandler(store *routestore.Store) *Handler {
	return &Handler{store: store}
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	path := r.URL.Path
	routes := h.store.Snapshot()

	for _, route := range routes {
		rule, matched := h.findMatchingRule(path, route)
		if !matched {
			continue
		}

		// Free path — forward directly.
		if rule.Free {
			slog.Info("free path, forwarding", "path", path, "route", route.Name)
			metrics.RequestsTotal.WithLabelValues(path, route.Namespace, route.Name, "free").Inc()
			proxyToBackend(w, r, route, path)
			metrics.ProxyRequestDuration.Observe(time.Since(start).Seconds())
			return
		}

		// Determine if payment is required for conditional mode.
		if rule.Mode == "conditional" && len(rule.Conditions) > 0 {
			if !evaluateConditions(r, rule.Conditions) {
				slog.Info("conditional: no payment needed", "path", path, "route", route.Name)
				metrics.RequestsTotal.WithLabelValues(path, route.Namespace, route.Name, "conditional_free").Inc()
				proxyToBackend(w, r, route, path)
				metrics.ProxyRequestDuration.Observe(time.Since(start).Seconds())
				return
			}
		}

		// Payment required — check for payment header.
		paymentHeader := getPaymentHeader(r)
		if paymentHeader == "" {
			slog.Info("paid path, no payment header", "path", path, "route", route.Name)
			metrics.RequestsTotal.WithLabelValues(path, route.Namespace, route.Name, "payment_required").Inc()
			writePaymentRequired(w, r, route, rule.Price)
			return
		}

		// Verify payment with facilitator.
		verifyStart := time.Now()
		valid, err := verifyPayment(paymentHeader, r.URL.String(), route.FacilitatorURL)
		metrics.PaymentVerificationDuration.Observe(time.Since(verifyStart).Seconds())

		if err != nil {
			slog.Error("payment verification failed", "path", path, "route", route.Name, "error", err)
			metrics.RequestsTotal.WithLabelValues(path, route.Namespace, route.Name, "verification_error").Inc()
			writePaymentRequired(w, r, route, rule.Price)
			return
		}

		if !valid {
			slog.Info("payment invalid", "path", path, "route", route.Name)
			metrics.RequestsTotal.WithLabelValues(path, route.Namespace, route.Name, "payment_invalid").Inc()
			writePaymentRequired(w, r, route, rule.Price)
			return
		}

		slog.Info("payment verified, forwarding", "path", path, "route", route.Name)
		metrics.RequestsTotal.WithLabelValues(path, route.Namespace, route.Name, "payment_accepted").Inc()
		w.Header().Set("Payment-Response", "accepted")
		proxyToBackend(w, r, route, path)
		metrics.ProxyRequestDuration.Observe(time.Since(start).Seconds())
		return
	}

	// No route matched.
	slog.Info("no matching route", "path", path)
	http.Error(w, "no x402 route configured for this path", http.StatusNotFound)
}

// findMatchingRule finds the first rule in a route that matches the given path.
func (h *Handler) findMatchingRule(path string, route *routestore.CompiledRoute) (*routestore.CompiledRule, bool) {
	for i := range route.Rules {
		if matchPath(route.Rules[i].Path, path) {
			return &route.Rules[i], true
		}
	}
	return nil, false
}
