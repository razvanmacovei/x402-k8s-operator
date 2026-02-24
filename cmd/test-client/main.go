package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	endpoint := "http://localhost:8402/api/hello"
	if len(os.Args) > 1 {
		endpoint = os.Args[1]
	} else if env := os.Getenv("X402_ENDPOINT"); env != "" {
		endpoint = env
	}

	fmt.Println("=== x402 Test Client ===")
	fmt.Printf("Endpoint: %s\n\n", endpoint)

	// Step 1: Send request without payment — expect 402.
	fmt.Println("--- Step 1: Request without payment ---")
	resp, err := http.Get(endpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	fmt.Printf("Status: %d %s\n", resp.StatusCode, resp.Status)
	fmt.Printf("Content-Type: %s\n", resp.Header.Get("Content-Type"))

	// Decode PAYMENT-REQUIRED Base64 header.
	if payReqHeader := resp.Header.Get("PAYMENT-REQUIRED"); payReqHeader != "" {
		fmt.Printf("PAYMENT-REQUIRED header (Base64): %s...\n", truncate(payReqHeader, 60))
		if decoded, err := base64.StdEncoding.DecodeString(payReqHeader); err == nil {
			var pretty json.RawMessage
			if json.Unmarshal(decoded, &pretty) == nil {
				indented, _ := json.MarshalIndent(pretty, "  ", "  ")
				fmt.Printf("PAYMENT-REQUIRED (decoded):\n  %s\n", string(indented))
			}
		} else {
			fmt.Printf("PAYMENT-REQUIRED header decode error: %v\n", err)
		}
	}

	fmt.Printf("Body:\n%s\n\n", string(body))

	if resp.StatusCode != http.StatusPaymentRequired {
		fmt.Println("Expected 402 Payment Required, got something else.")
		fmt.Println("The endpoint may not be a paid route, or the gateway is not running.")
		os.Exit(0)
	}

	// Step 2: Send request with a mock payment header (Payment-Signature).
	fmt.Println("--- Step 2: Request with mock payment (Payment-Signature header) ---")

	fakePayload := `{"scheme":"exact","network":"eip155:84532","payload":{"signature":"0xdeadbeef","authorization":{"from":"0x0000000000000000000000000000000000000001","to":"0x1f6004907Adc7d313768b85917e069e011150390","value":"1000","validAfter":"0","validBefore":"999999999999","nonce":"0x01"}}}`
	paymentHeader := base64.StdEncoding.EncodeToString([]byte(fakePayload))

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating request: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Payment-Signature", paymentHeader)

	fmt.Printf("Payment-Signature: %s...\n", truncate(paymentHeader, 60))

	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	fmt.Printf("Status: %d %s\n", resp2.StatusCode, resp2.Status)
	fmt.Printf("Content-Type: %s\n", resp2.Header.Get("Content-Type"))

	// Decode PAYMENT-RESPONSE Base64 header.
	if payRespHeader := resp2.Header.Get("PAYMENT-RESPONSE"); payRespHeader != "" {
		fmt.Printf("PAYMENT-RESPONSE header (Base64): %s...\n", truncate(payRespHeader, 60))
		if decoded, err := base64.StdEncoding.DecodeString(payRespHeader); err == nil {
			var pretty json.RawMessage
			if json.Unmarshal(decoded, &pretty) == nil {
				indented, _ := json.MarshalIndent(pretty, "  ", "  ")
				fmt.Printf("PAYMENT-RESPONSE (decoded):\n  %s\n", string(indented))
			}
		} else {
			fmt.Printf("PAYMENT-RESPONSE header decode error: %v\n", err)
		}
	}

	fmt.Printf("Body:\n%s\n\n", string(body2))

	if resp2.StatusCode == http.StatusOK {
		fmt.Println("Payment accepted! The gateway forwarded the request to the backend.")
	} else {
		fmt.Printf("Unexpected status %d. Check the facilitator and gateway logs.\n", resp2.StatusCode)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
