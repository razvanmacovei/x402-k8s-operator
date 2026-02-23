package gateway

import (
	"net/http"

	"github.com/razvanmacovei/x402-k8s-operator/internal/routestore"
)

// evaluateConditions checks request headers against compiled conditions.
// Returns true if payment is required for this request.
//
// For "conditional" mode:
//   - If any condition matches with action "pay", payment is required.
//   - If any condition matches with action "free", payment is not required.
//   - If no conditions match, payment is required (safe default).
func evaluateConditions(r *http.Request, conditions []routestore.CompiledCondition) bool {
	for _, cond := range conditions {
		headerVal := r.Header.Get(cond.Header)
		if headerVal == "" {
			continue
		}
		if cond.Pattern.MatchString(headerVal) {
			return cond.Action == "pay"
		}
	}
	// No condition matched — require payment as safe default.
	return true
}
