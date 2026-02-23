package gateway

import "strings"

// matchPath checks if a request path matches a pattern.
// Supports:
//   - Exact match: "/api/v1/users" matches "/api/v1/users"
//   - Single segment wildcard (*): "/api/v1/*" matches "/api/v1/users" but not "/api/v1/users/123"
//   - Multi-segment wildcard (**): "/api/v1/**" matches "/api/v1/users" and "/api/v1/users/123/posts"
func matchPath(pattern, path string) bool {
	if pattern == path {
		return true
	}

	// Handle ** (any depth) at the end.
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		prefix = strings.TrimRight(prefix, "/")
		cleanPath := strings.TrimRight(path, "/")
		if prefix == "" {
			return true
		}
		return cleanPath == prefix || strings.HasPrefix(cleanPath, prefix+"/")
	}

	// Handle trailing /* (also any depth for backward compat).
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		prefix = strings.TrimRight(prefix, "/")
		cleanPath := strings.TrimRight(path, "/")
		if prefix == "" {
			return true
		}
		return cleanPath == prefix || strings.HasPrefix(cleanPath, prefix+"/")
	}

	// Segment-by-segment matching with single * wildcards.
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")

	if len(patternParts) != len(pathParts) {
		return false
	}

	for i, pp := range patternParts {
		if pp == "*" {
			continue
		}
		if pp != pathParts[i] {
			return false
		}
	}
	return true
}
