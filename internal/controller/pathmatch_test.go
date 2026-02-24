package controller

import "testing"

func TestPathMatchesPaidRoutes(t *testing.T) {
	r := &X402RouteReconciler{}

	tests := []struct {
		name        string
		ingressPath string
		paidPaths   []string
		want        bool
	}{
		// Exact matches
		{name: "exact match /api", ingressPath: "/api", paidPaths: []string{"/api"}, want: true},
		{name: "exact match /", ingressPath: "/", paidPaths: []string{"/"}, want: true},

		// Wildcard paid paths matching same ingress path
		{name: "ingress /api matches paid /api/*", ingressPath: "/api", paidPaths: []string{"/api/*"}, want: true},
		{name: "ingress /api matches paid /api/**", ingressPath: "/api", paidPaths: []string{"/api/**"}, want: true},

		// Catch-all ingress "/" should match any paid path
		{name: "catch-all / matches paid /api/*", ingressPath: "/", paidPaths: []string{"/api/*"}, want: true},
		{name: "catch-all / matches paid /api/v1/*", ingressPath: "/", paidPaths: []string{"/api/v1/*"}, want: true},
		{name: "catch-all / matches paid /data", ingressPath: "/", paidPaths: []string{"/data"}, want: true},

		// Ingress prefix is parent of paid path
		{name: "ingress /api matches paid /api/v1/*", ingressPath: "/api", paidPaths: []string{"/api/v1/*"}, want: true},

		// No match
		{name: "ingress /web does not match paid /api/*", ingressPath: "/web", paidPaths: []string{"/api/*"}, want: false},
		{name: "ingress /api-v2 does not match paid /api/*", ingressPath: "/api-v2", paidPaths: []string{"/api/*"}, want: false},

		// NGINX regex suffix
		{name: "ingress /api(.*) matches paid /api/*", ingressPath: "/api(.*)", paidPaths: []string{"/api/*"}, want: true},

		// Multiple paid paths
		{name: "matches one of multiple paid paths", ingressPath: "/", paidPaths: []string{"/api/*", "/data/*"}, want: true},
		{name: "no match against multiple paid paths", ingressPath: "/web", paidPaths: []string{"/api/*", "/data/*"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.pathMatchesPaidRoutes(tt.ingressPath, tt.paidPaths)
			if got != tt.want {
				t.Errorf("pathMatchesPaidRoutes(%q, %v) = %v, want %v", tt.ingressPath, tt.paidPaths, got, tt.want)
			}
		})
	}
}
