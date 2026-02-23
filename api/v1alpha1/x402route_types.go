package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// X402RouteSpec defines the desired state of X402Route.
type X402RouteSpec struct {
	// IngressRef references the existing Ingress to patch with payment gating.
	IngressRef IngressReference `json:"ingressRef"`

	// Payment defines global payment defaults for this route.
	Payment PaymentDefaults `json:"payment"`

	// Routes defines per-path pricing rules.
	Routes []RouteRule `json:"routes"`
}

// IngressReference identifies an Ingress resource to patch.
type IngressReference struct {
	// Name is the name of the Ingress resource.
	Name string `json:"name"`

	// Namespace of the Ingress. Defaults to the X402Route's namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// PaymentDefaults defines the global payment configuration.
type PaymentDefaults struct {
	// Wallet is the wallet address to receive payments.
	Wallet string `json:"wallet"`

	// Network is the blockchain network (e.g. "base", "base-sepolia").
	Network string `json:"network"`

	// DefaultPrice is the default price for paid routes (e.g. "0.001").
	// Individual routes can override this.
	// +optional
	DefaultPrice string `json:"defaultPrice,omitempty"`

	// FacilitatorURL is the URL of the x402 facilitator service.
	// Defaults to https://x402.org/facilitator.
	// +optional
	FacilitatorURL string `json:"facilitatorURL,omitempty"`
}

// RouteRule defines a single route rule with pricing and optional conditions.
type RouteRule struct {
	// Path is the URL path pattern (supports * for single segment, ** for any depth).
	Path string `json:"path"`

	// Price overrides the default price for this specific path.
	// +optional
	Price string `json:"price,omitempty"`

	// Free marks this path as free (no payment required).
	// +optional
	Free bool `json:"free,omitempty"`

	// Mode is the payment mode: "all-pay" (default) or "conditional".
	// +optional
	// +kubebuilder:validation:Enum=all-pay;conditional
	// +kubebuilder:default="all-pay"
	Mode string `json:"mode,omitempty"`

	// Conditions defines when payment is required (only used when mode is "conditional").
	// +optional
	Conditions []PaymentCondition `json:"conditions,omitempty"`
}

// PaymentCondition defines a condition for conditional payment evaluation.
type PaymentCondition struct {
	// Header is the HTTP header to inspect.
	Header string `json:"header"`

	// Pattern is a regex pattern to match against the header value.
	Pattern string `json:"pattern"`

	// Action specifies what happens when the pattern matches: "pay" or "free".
	// +kubebuilder:validation:Enum=pay;free
	Action string `json:"action"`
}

// X402RouteStatus defines the observed state of X402Route.
type X402RouteStatus struct {
	// IngressPatched indicates whether the referenced Ingress has been patched.
	// +optional
	IngressPatched bool `json:"ingressPatched,omitempty"`

	// Ready indicates whether the route is fully configured and active.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// ActiveRoutes is the number of active route rules.
	// +optional
	ActiveRoutes int `json:"activeRoutes,omitempty"`

	// Conditions represent the latest available observations of the X402Route's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ingress Patched",type="boolean",JSONPath=".status.ingressPatched"
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="Active Routes",type="integer",JSONPath=".status.activeRoutes"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// X402Route is the Schema for the x402routes API.
type X402Route struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   X402RouteSpec   `json:"spec,omitempty"`
	Status X402RouteStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// X402RouteList contains a list of X402Route.
type X402RouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []X402Route `json:"items"`
}

func init() {
	SchemeBuilder.Register(&X402Route{}, &X402RouteList{})
}
