package gateway

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/razvanmacovei/x402-k8s-operator/internal/routestore"
)

// proxyToBackend forwards the request to the appropriate backend.
func proxyToBackend(w http.ResponseWriter, r *http.Request, route *routestore.CompiledRoute, path string) {
	backendURL := findBackend(route.Backends, path)
	if backendURL == "" {
		slog.Error("no backend found for path", "path", path, "route", route.Name)
		http.Error(w, "no backend configured", http.StatusBadGateway)
		return
	}

	target, err := url.Parse(backendURL)
	if err != nil {
		slog.Error("failed to parse backend URL", "url", backendURL, "error", err)
		http.Error(w, "bad backend URL", http.StatusBadGateway)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ServeHTTP(w, r)
}

// findBackend finds the best matching backend URL for a path.
func findBackend(backends map[string]string, path string) string {
	// Exact match first.
	if u, ok := backends[path]; ok {
		return u
	}

	// Pattern match.
	for pattern, u := range backends {
		if matchPath(pattern, path) {
			return u
		}
	}

	// Fallback to any backend (single-backend common case).
	for _, u := range backends {
		return u
	}
	return ""
}
