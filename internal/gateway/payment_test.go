package gateway

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/razvanmacovei/x402-k8s-operator/internal/routestore"
)

func TestHumanToAtomicUnits(t *testing.T) {
	tests := []struct {
		name     string
		price    string
		decimals int
		want     string
		wantErr  bool
	}{
		{name: "0.001 with 6 decimals", price: "0.001", decimals: 6, want: "1000"},
		{name: "0.01 with 6 decimals", price: "0.01", decimals: 6, want: "10000"},
		{name: "1 with 6 decimals", price: "1", decimals: 6, want: "1000000"},
		{name: "0 with 6 decimals", price: "0", decimals: 6, want: "0"},
		{name: "0.000001 with 6 decimals", price: "0.000001", decimals: 6, want: "1"},
		{name: "100 with 6 decimals", price: "100", decimals: 6, want: "100000000"},
		{name: "0.5 with 6 decimals", price: "0.5", decimals: 6, want: "500000"},
		{name: "1.23 with 2 decimals", price: "1.23", decimals: 2, want: "123"},
		{name: "empty price", price: "", decimals: 6, wantErr: true},
		{name: "invalid format", price: "abc", decimals: 6, wantErr: true},
		{name: "too many decimals", price: "0.0000001", decimals: 6, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := humanToAtomicUnits(tt.price, tt.decimals)
			if (err != nil) != tt.wantErr {
				t.Errorf("humanToAtomicUnits(%q, %d) error = %v, wantErr %v", tt.price, tt.decimals, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("humanToAtomicUnits(%q, %d) = %q, want %q", tt.price, tt.decimals, got, tt.want)
			}
		})
	}
}

func TestBuildPaymentRequirements(t *testing.T) {
	route := &routestore.CompiledRoute{
		Wallet:  "0xTestWallet",
		Network: "base-sepolia",
	}

	r := httptest.NewRequest("GET", "/api/test", nil)
	reqs, err := buildPaymentRequirements(r, route, "0.001")
	if err != nil {
		t.Fatalf("buildPaymentRequirements returned error: %v", err)
	}

	if reqs.X402Version != 2 {
		t.Errorf("X402Version = %d, want 2", reqs.X402Version)
	}

	if reqs.Resource == nil {
		t.Fatal("Resource is nil")
	}
	if reqs.Resource.URL != "/api/test" {
		t.Errorf("Resource.URL = %q, want %q", reqs.Resource.URL, "/api/test")
	}
	if reqs.Resource.Description == "" {
		t.Error("Resource.Description is empty")
	}

	if len(reqs.Accepts) != 1 {
		t.Fatalf("len(Accepts) = %d, want 1", len(reqs.Accepts))
	}

	accept := reqs.Accepts[0]
	if accept.Network != "eip155:84532" {
		t.Errorf("Network = %q, want %q", accept.Network, "eip155:84532")
	}
	if accept.Amount != "1000" {
		t.Errorf("Amount = %q, want %q", accept.Amount, "1000")
	}
	if accept.PayTo != "0xTestWallet" {
		t.Errorf("PayTo = %q, want %q", accept.PayTo, "0xTestWallet")
	}
	if accept.Asset != "0x036CbD53842c5426634e7929541eC2318f3dCF7e" {
		t.Errorf("Asset = %q, want base-sepolia USDC address", accept.Asset)
	}
	if accept.Extra == nil {
		t.Fatal("Extra is nil")
	}
	if accept.Extra.Name != "USDC" {
		t.Errorf("Extra.Name = %q, want %q", accept.Extra.Name, "USDC")
	}
	if accept.Extra.Version != "2" {
		t.Errorf("Extra.Version = %q, want %q", accept.Extra.Version, "2")
	}
}

func TestWritePaymentRequired(t *testing.T) {
	route := &routestore.CompiledRoute{
		Wallet:  "0xTestWallet",
		Network: "base-sepolia",
	}

	r := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	writePaymentRequired(w, r, route, "0.01")

	resp := w.Result()

	// Check status code.
	if resp.StatusCode != http.StatusPaymentRequired {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusPaymentRequired)
	}

	// Check Content-Type.
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	// Check PAYMENT-REQUIRED header exists and is valid Base64.
	payReqHeader := resp.Header.Get("PAYMENT-REQUIRED")
	if payReqHeader == "" {
		t.Fatal("PAYMENT-REQUIRED header is missing")
	}

	decoded, err := base64.StdEncoding.DecodeString(payReqHeader)
	if err != nil {
		t.Fatalf("PAYMENT-REQUIRED header is not valid Base64: %v", err)
	}

	var reqs paymentRequirements
	if err := json.Unmarshal(decoded, &reqs); err != nil {
		t.Fatalf("PAYMENT-REQUIRED header is not valid JSON: %v", err)
	}

	if reqs.X402Version != 2 {
		t.Errorf("decoded X402Version = %d, want 2", reqs.X402Version)
	}
	if reqs.Resource == nil {
		t.Fatal("decoded Resource is nil")
	}
	if len(reqs.Accepts) != 1 {
		t.Fatalf("decoded len(Accepts) = %d, want 1", len(reqs.Accepts))
	}
	if reqs.Accepts[0].Amount != "10000" {
		t.Errorf("decoded Amount = %q, want %q (0.01 * 10^6)", reqs.Accepts[0].Amount, "10000")
	}

	// Verify body matches header.
	var bodyReqs paymentRequirements
	if err := json.NewDecoder(resp.Body).Decode(&bodyReqs); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if bodyReqs.X402Version != reqs.X402Version {
		t.Error("body and header X402Version mismatch")
	}
}
