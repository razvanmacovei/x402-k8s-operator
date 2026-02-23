package routestore

import "regexp"

// CompiledRoute represents a fully compiled route from an X402Route CRD.
type CompiledRoute struct {
	Name           string
	Namespace      string
	Wallet         string
	Network        string
	FacilitatorURL string
	DefaultPrice   string
	Rules          []CompiledRule
	Backends       map[string]string // path -> backend URL
}

// CompiledRule is a single route rule with optional conditions.
type CompiledRule struct {
	Path       string
	Price      string // effective price (from rule or default)
	Free       bool
	Mode       string // "all-pay" or "conditional"
	Conditions []CompiledCondition
}

// CompiledCondition is a pre-compiled condition for conditional payment evaluation.
type CompiledCondition struct {
	Header  string
	Pattern *regexp.Regexp
	Action  string // "pay" or "free"
}
