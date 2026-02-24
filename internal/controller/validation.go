package controller

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// privateRanges defines CIDR blocks for private/reserved IP addresses.
var privateRanges = func() []*net.IPNet {
	cidrs := []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"::1/128",
		"fd00::/8",
		"fe80::/10",
	}
	var nets []*net.IPNet
	for _, cidr := range cidrs {
		_, ipNet, _ := net.ParseCIDR(cidr)
		nets = append(nets, ipNet)
	}
	return nets
}()

// validateFacilitatorURL validates that the facilitator URL is safe and not
// pointing at internal/private network resources (SSRF prevention).
func validateFacilitatorURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("malformed URL: %w", err)
	}

	// Require http or https scheme.
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme %q not allowed, must be http or https", u.Scheme)
	}

	hostname := u.Hostname()
	if hostname == "" {
		return fmt.Errorf("missing hostname")
	}

	// Block known dangerous hostnames.
	lower := strings.ToLower(hostname)
	if lower == "localhost" {
		return fmt.Errorf("hostname %q is not allowed", hostname)
	}
	if strings.HasSuffix(lower, ".internal") {
		return fmt.Errorf("hostname %q is not allowed (*.internal)", hostname)
	}

	// Check if hostname is a literal IP address.
	ip := net.ParseIP(hostname)
	if ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("IP address %s is in a private/reserved range", hostname)
		}
		// Literal IPs require HTTPS.
		if u.Scheme != "https" {
			return fmt.Errorf("HTTP not allowed for IP address %s, use HTTPS", hostname)
		}
		return nil
	}

	// Hostname is a DNS name.
	if isInClusterHostname(lower) {
		// In-cluster hostnames: HTTP is allowed.
		return nil
	}

	// External DNS hostname: require HTTPS.
	if u.Scheme != "https" {
		return fmt.Errorf("HTTP not allowed for external hostname %q, use HTTPS", hostname)
	}

	return nil
}

// isPrivateIP returns true if the IP falls within a private or reserved range.
func isPrivateIP(ip net.IP) bool {
	for _, cidr := range privateRanges {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// isInClusterHostname returns true if the hostname looks like an in-cluster
// Kubernetes service name (bare name without dots, or *.svc.cluster.local, or *.svc).
func isInClusterHostname(hostname string) bool {
	// Bare hostname with no dots (e.g. "my-facilitator").
	if !strings.Contains(hostname, ".") {
		return true
	}
	// Full cluster-local DNS (e.g. "my-svc.my-ns.svc.cluster.local").
	if strings.HasSuffix(hostname, ".svc.cluster.local") {
		return true
	}
	// Short cluster-local DNS (e.g. "my-svc.my-ns.svc").
	if strings.HasSuffix(hostname, ".svc") {
		return true
	}
	return false
}
