package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/razvanmacovei/x402-k8s-operator/internal/routestore"
)

// facilitatorClient is an HTTP client with timeout for facilitator API calls.
var facilitatorClient = &http.Client{
	Timeout: 10 * time.Second,
}

// networkAssets maps network identifiers to their USDC contract addresses.
var networkAssets = map[string]string{
	"base":         "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",
	"eip155:8453":  "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",
	"base-sepolia": "0x036CbD53842c5426634e7929541eC2318f3dCF7e",
	"eip155:84532": "0x036CbD53842c5426634e7929541eC2318f3dCF7e",
}

// networkToChainID maps friendly network names to EIP-155 chain identifiers.
var networkToChainID = map[string]string{
	"base":         "eip155:8453",
	"base-sepolia": "eip155:84532",
}

type paymentRequirements struct {
	X402Version int             `json:"x402Version"`
	Accepts     []paymentAccept `json:"accepts"`
	Error       string          `json:"error,omitempty"`
}

type paymentAccept struct {
	Scheme            string `json:"scheme"`
	Network           string `json:"network"`
	MaxAmountRequired string `json:"maxAmountRequired"`
	Resource          string `json:"resource"`
	Description       string `json:"description"`
	MimeType          string `json:"mimeType,omitempty"`
	PayTo             string `json:"payTo"`
	MaxTimeoutSeconds int    `json:"maxTimeoutSeconds"`
	Asset             string `json:"asset"`
}

type verifyRequest struct {
	PaymentHeader string `json:"paymentHeader"`
	Resource      string `json:"resource"`
}

type verifyResponse struct {
	Valid bool `json:"valid"`
}

// writePaymentRequired writes a 402 Payment Required response with x402 V2 format.
func writePaymentRequired(w http.ResponseWriter, r *http.Request, route *routestore.CompiledRoute, price string) {
	network := route.Network
	chainID := network
	if mapped, ok := networkToChainID[network]; ok {
		chainID = mapped
	}

	asset := networkAssets[network]

	resp := paymentRequirements{
		X402Version: 2,
		Accepts: []paymentAccept{
			{
				Scheme:            "exact",
				Network:           chainID,
				MaxAmountRequired: price,
				Resource:          r.URL.String(),
				Description:       "Payment required to access this resource",
				PayTo:             route.Wallet,
				MaxTimeoutSeconds: 300,
				Asset:             asset,
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Payment-Required", "x402")
	w.WriteHeader(http.StatusPaymentRequired)
	json.NewEncoder(w).Encode(resp)
}

// verifyPayment verifies a payment header with the facilitator service.
func verifyPayment(paymentHeader, resource, facilitatorURL string) (bool, error) {
	reqBody := verifyRequest{
		PaymentHeader: paymentHeader,
		Resource:      resource,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return false, fmt.Errorf("marshal verify request: %w", err)
	}

	verifyURL := strings.TrimRight(facilitatorURL, "/") + "/verify"
	resp, err := facilitatorClient.Post(verifyURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("POST to facilitator: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("read facilitator response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("facilitator returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var vResp verifyResponse
	if err := json.Unmarshal(respBody, &vResp); err != nil {
		return false, fmt.Errorf("unmarshal verify response: %w", err)
	}

	return vResp.Valid, nil
}

// getPaymentHeader extracts the payment header from the request.
// Checks Payment-Signature first (V2), then falls back to X-Payment (V1 compat).
func getPaymentHeader(r *http.Request) string {
	if h := r.Header.Get("Payment-Signature"); h != "" {
		return h
	}
	return r.Header.Get("X-Payment")
}
