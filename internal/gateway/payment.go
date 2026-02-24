package gateway

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
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

	// Avalanche
	"avalanche":       "0xB97EF9Ef8734C71904D8002F8b6Bc66Dd9c48a6E",
	"eip155:43114":    "0xB97EF9Ef8734C71904D8002F8b6Bc66Dd9c48a6E",
	"avalanche-fuji":  "0x5425890298aed601595a70AB815c96711a31Bc65",
	"eip155:43113":    "0x5425890298aed601595a70AB815c96711a31Bc65",

	// Solana
	"solana":                                        "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
	"solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp":      "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
	"solana-devnet":                                  "4zMMC9srt5Ri5X14GAgXhaHii3GnPAEERYPJgZJDncDU",
	"solana:EtWTRABZaYq6iMfeYKouRu166VU2xqa1":       "4zMMC9srt5Ri5X14GAgXhaHii3GnPAEERYPJgZJDncDU",
}

// networkToChainID maps friendly network names to chain identifiers.
var networkToChainID = map[string]string{
	"base":           "eip155:8453",
	"base-sepolia":   "eip155:84532",
	"avalanche":      "eip155:43114",
	"avalanche-fuji": "eip155:43113",
	"solana":         "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp",
	"solana-devnet":  "solana:EtWTRABZaYq6iMfeYKouRu166VU2xqa1",
}

// assetInfo holds metadata for a network's payment asset.
type assetInfo struct {
	Name     string
	Version  string
	Decimals int
}

// networkAssetInfo maps chain identifiers to asset metadata.
var networkAssetInfo = map[string]assetInfo{
	"eip155:8453":                              {Name: "USDC", Version: "2", Decimals: 6},
	"eip155:84532":                             {Name: "USDC", Version: "2", Decimals: 6},
	"eip155:43114":                             {Name: "USDC", Version: "2", Decimals: 6},
	"eip155:43113":                             {Name: "USDC", Version: "2", Decimals: 6},
	"solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp": {Name: "USDC", Version: "2", Decimals: 6},
	"solana:EtWTRABZaYq6iMfeYKouRu166VU2xqa1": {Name: "USDC", Version: "2", Decimals: 6},
}

// --- Structs ---

// paymentResource describes the resource being paid for.
type paymentResource struct {
	URL         string `json:"url"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType,omitempty"`
}

// paymentExtra carries asset metadata in the payment schema.
type paymentExtra struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// paymentAccept is a single accepted payment method.
type paymentAccept struct {
	Scheme            string `json:"scheme"`
	Network           string `json:"network"`
	Amount            string `json:"amount"`
	PayTo             string `json:"payTo"`
	MaxTimeoutSeconds int    `json:"maxTimeoutSeconds"`
	Asset             string `json:"asset"`
	Extra             *paymentExtra `json:"extra,omitempty"`
}

// paymentRequirements is the full 402 response body and PAYMENT-REQUIRED header.
type paymentRequirements struct {
	X402Version int               `json:"x402Version"`
	Resource    *paymentResource  `json:"resource"`
	Accepts     []paymentAccept   `json:"accepts"`
	Error       string            `json:"error,omitempty"`
}

// facilitatorRequest is the request body sent to /verify and /settle.
type facilitatorRequest struct {
	PaymentPayload      json.RawMessage  `json:"paymentPayload"`
	PaymentRequirements *paymentAccept   `json:"paymentRequirements"`
}

// verifyResponse is the response from /verify.
type verifyResponse struct {
	IsValid       bool   `json:"isValid"`
	InvalidReason string `json:"invalidReason,omitempty"`
	Payer         string `json:"payer,omitempty"`
}

// settleResponse is the response from /settle.
type settleResponse struct {
	Success     bool   `json:"success"`
	ErrorReason string `json:"errorReason,omitempty"`
	Payer       string `json:"payer,omitempty"`
	Transaction string `json:"transaction,omitempty"`
	Network     string `json:"network,omitempty"`
}

// --- Helper functions ---

// humanToAtomicUnits converts a human-readable price string (e.g. "0.001") to atomic
// units for a token with the given number of decimals (e.g. 6 → "1000").
func humanToAtomicUnits(price string, decimals int) (string, error) {
	if price == "" {
		return "", fmt.Errorf("empty price")
	}

	// Use big.Rat for precise decimal arithmetic.
	rat := new(big.Rat)
	if _, ok := rat.SetString(price); !ok {
		return "", fmt.Errorf("invalid price format: %q", price)
	}

	// Multiply by 10^decimals.
	multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	rat.Mul(rat, new(big.Rat).SetInt(multiplier))

	// The result must be a whole number.
	if !rat.IsInt() {
		return "", fmt.Errorf("price %q has more decimal places than token supports (%d)", price, decimals)
	}

	return rat.Num().String(), nil
}

// buildPaymentRequirements constructs the full paymentRequirements from a route and price.
func buildPaymentRequirements(r *http.Request, route *routestore.CompiledRoute, price string) (*paymentRequirements, error) {
	network := route.Network
	chainID := network
	if mapped, ok := networkToChainID[network]; ok {
		chainID = mapped
	}

	asset := networkAssets[network]

	info, ok := networkAssetInfo[chainID]
	if !ok {
		// Fallback: default to 6 decimals USDC.
		info = assetInfo{Name: "USDC", Version: "2", Decimals: 6}
	}

	atomicAmount, err := humanToAtomicUnits(price, info.Decimals)
	if err != nil {
		return nil, fmt.Errorf("convert price to atomic units: %w", err)
	}

	return &paymentRequirements{
		X402Version: 2,
		Resource: &paymentResource{
			URL:         r.URL.String(),
			Description: "Payment required to access this resource",
		},
		Accepts: []paymentAccept{
			{
				Scheme:            "exact",
				Network:           chainID,
				Amount:            atomicAmount,
				PayTo:             route.Wallet,
				MaxTimeoutSeconds: 300,
				Asset:             asset,
				Extra: &paymentExtra{
					Name:    info.Name,
					Version: info.Version,
				},
			},
		},
	}, nil
}

// --- Main functions ---

// writePaymentRequired writes a 402 Payment Required response with x402 format.
// Sets both the JSON body and the Base64-encoded PAYMENT-REQUIRED header.
func writePaymentRequired(w http.ResponseWriter, r *http.Request, route *routestore.CompiledRoute, price string) {
	reqs, err := buildPaymentRequirements(r, route, price)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to build payment requirements: %v", err), http.StatusInternalServerError)
		return
	}

	respJSON, err := json.Marshal(reqs)
	if err != nil {
		http.Error(w, "failed to marshal payment requirements", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("PAYMENT-REQUIRED", base64.StdEncoding.EncodeToString(respJSON))
	w.WriteHeader(http.StatusPaymentRequired)
	w.Write(respJSON)
}

// verifyAndSettlePayment decodes the Payment-Signature header, calls the facilitator's
// /verify endpoint, and on success calls /settle. Returns the settle response.
func verifyAndSettlePayment(paymentHeader string, paymentReqs *paymentRequirements, facilitatorURL string) (*settleResponse, error) {
	// Decode the Base64 Payment-Signature header to get the payment payload JSON.
	payloadBytes, err := base64.StdEncoding.DecodeString(paymentHeader)
	if err != nil {
		return nil, fmt.Errorf("base64 decode Payment-Signature: %w", err)
	}

	// Validate that payloadBytes is valid JSON.
	if !json.Valid(payloadBytes) {
		return nil, fmt.Errorf("Payment-Signature is not valid JSON after base64 decode")
	}

	if len(paymentReqs.Accepts) == 0 {
		return nil, fmt.Errorf("no payment accepts in requirements")
	}

	facReq := facilitatorRequest{
		PaymentPayload:      json.RawMessage(payloadBytes),
		PaymentRequirements: &paymentReqs.Accepts[0],
	}

	reqBody, err := json.Marshal(facReq)
	if err != nil {
		return nil, fmt.Errorf("marshal facilitator request: %w", err)
	}

	baseURL := strings.TrimRight(facilitatorURL, "/")

	// --- /verify ---
	verifyResp, err := facilitatorClient.Post(baseURL+"/verify", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("POST to facilitator /verify: %w", err)
	}
	defer verifyResp.Body.Close()

	verifyBody, err := io.ReadAll(verifyResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read /verify response: %w", err)
	}

	if verifyResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("facilitator /verify returned status %d: %s", verifyResp.StatusCode, string(verifyBody))
	}

	var vResp verifyResponse
	if err := json.Unmarshal(verifyBody, &vResp); err != nil {
		return nil, fmt.Errorf("unmarshal /verify response: %w", err)
	}

	if !vResp.IsValid {
		reason := vResp.InvalidReason
		if reason == "" {
			reason = "payment not valid"
		}
		return nil, fmt.Errorf("payment invalid: %s", reason)
	}

	// --- /settle ---
	settleResp, err := facilitatorClient.Post(baseURL+"/settle", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("POST to facilitator /settle: %w", err)
	}
	defer settleResp.Body.Close()

	settleBody, err := io.ReadAll(settleResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read /settle response: %w", err)
	}

	if settleResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("facilitator /settle returned status %d: %s", settleResp.StatusCode, string(settleBody))
	}

	var sResp settleResponse
	if err := json.Unmarshal(settleBody, &sResp); err != nil {
		return nil, fmt.Errorf("unmarshal /settle response: %w", err)
	}

	if !sResp.Success {
		reason := sResp.ErrorReason
		if reason == "" {
			reason = "settlement failed"
		}
		return nil, fmt.Errorf("settlement failed: %s", reason)
	}

	return &sResp, nil
}

// getPaymentHeader extracts the payment header from the request.
// Checks Payment-Signature first, then falls back to X-Payment for compat.
func getPaymentHeader(r *http.Request) string {
	if h := r.Header.Get("Payment-Signature"); h != "" {
		return h
	}
	return r.Header.Get("X-Payment")
}
