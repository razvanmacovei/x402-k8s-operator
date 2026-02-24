package controller

import (
	"net"
	"testing"
)

func TestValidateFacilitatorURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		// Valid URLs
		{name: "https external", url: "https://x402.org/facilitator", wantErr: false},
		{name: "https external with port", url: "https://facilitator.example.com:8443/verify", wantErr: false},
		{name: "http bare service name", url: "http://mock-facilitator:8080", wantErr: false},
		{name: "http bare service name no port", url: "http://my-facilitator", wantErr: false},
		{name: "http svc.cluster.local", url: "http://facilitator.payments.svc.cluster.local:8080", wantErr: false},
		{name: "http short svc", url: "http://facilitator.payments.svc", wantErr: false},
		{name: "https bare service name", url: "https://mock-facilitator:8080", wantErr: false},

		// Blocked: private IPs
		{name: "loopback 127.0.0.1", url: "https://127.0.0.1/metadata", wantErr: true},
		{name: "loopback 127.0.0.2", url: "https://127.0.0.2", wantErr: true},
		{name: "10.x private", url: "https://10.0.0.1:8080/verify", wantErr: true},
		{name: "172.16.x private", url: "https://172.16.0.1", wantErr: true},
		{name: "192.168.x private", url: "https://192.168.1.1", wantErr: true},
		{name: "link-local metadata", url: "https://169.254.169.254/latest/meta-data/", wantErr: true},
		{name: "IPv6 loopback", url: "https://[::1]:8080", wantErr: true},
		{name: "IPv6 fd00 ULA", url: "https://[fd00::1]", wantErr: true},
		{name: "IPv6 fe80 link-local", url: "https://[fe80::1]", wantErr: true},

		// Blocked: dangerous hostnames
		{name: "localhost", url: "http://localhost:8080", wantErr: true},
		{name: "localhost https", url: "https://localhost", wantErr: true},
		{name: "dot internal", url: "https://metadata.internal", wantErr: true},
		{name: "dot internal subdomain", url: "https://foo.bar.internal", wantErr: true},

		// Blocked: HTTP to external domains
		{name: "http external domain", url: "http://example.com/verify", wantErr: true},
		{name: "http external with subdomain", url: "http://facilitator.example.com:8080", wantErr: true},

		// Blocked: invalid schemes
		{name: "ftp scheme", url: "ftp://example.com/file", wantErr: true},
		{name: "file scheme", url: "file:///etc/passwd", wantErr: true},
		{name: "gopher scheme", url: "gopher://evil.com", wantErr: true},

		// Blocked: HTTP to literal IPs
		{name: "http public IP", url: "http://8.8.8.8/verify", wantErr: true},

		// Blocked: missing hostname
		{name: "empty hostname", url: "https:///path", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFacilitatorURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFacilitatorURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.0.1", true},
		{"169.254.169.254", true},
		{"::1", true},
		{"fd00::1", true},
		{"fe80::1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"2001:db8::1", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP %q", tt.ip)
			}
			got := isPrivateIP(ip)
			if got != tt.private {
				t.Errorf("isPrivateIP(%s) = %v, want %v", tt.ip, got, tt.private)
			}
		})
	}
}

func TestIsInClusterHostname(t *testing.T) {
	tests := []struct {
		hostname  string
		inCluster bool
	}{
		{"mock-facilitator", true},
		{"my-svc", true},
		{"facilitator.payments.svc.cluster.local", true},
		{"facilitator.payments.svc", true},
		{"example.com", false},
		{"facilitator.example.com", false},
		{"svc.example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			got := isInClusterHostname(tt.hostname)
			if got != tt.inCluster {
				t.Errorf("isInClusterHostname(%q) = %v, want %v", tt.hostname, got, tt.inCluster)
			}
		})
	}
}
