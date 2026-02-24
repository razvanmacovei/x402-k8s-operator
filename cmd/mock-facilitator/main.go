package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// facilitatorRequest is the request format from the gateway.
type facilitatorRequest struct {
	PaymentPayload      json.RawMessage `json:"paymentPayload"`
	PaymentRequirements json.RawMessage `json:"paymentRequirements"`
}

type verifyResponse struct {
	IsValid       bool   `json:"isValid"`
	InvalidReason string `json:"invalidReason,omitempty"`
	Payer         string `json:"payer,omitempty"`
}

type settleResponse struct {
	Success     bool   `json:"success"`
	ErrorReason string `json:"errorReason,omitempty"`
	Payer       string `json:"payer,omitempty"`
	Transaction string `json:"transaction,omitempty"`
	Network     string `json:"network,omitempty"`
}

func main() {
	port := os.Getenv("X402_PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()

	mux.HandleFunc("POST /verify", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()

		slog.Info("verify request",
			"method", r.Method,
			"path", r.URL.Path,
			"body", string(body),
			"time", time.Now().Format(time.RFC3339),
		)

		var req facilitatorRequest
		if err := json.Unmarshal(body, &req); err != nil {
			slog.Warn("invalid request body", "error", err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(verifyResponse{
			IsValid: true,
			Payer:   "0x0000000000000000000000000000000000000001",
		})
	})

	mux.HandleFunc("POST /settle", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()

		slog.Info("settle request",
			"method", r.Method,
			"path", r.URL.Path,
			"body", string(body),
			"time", time.Now().Format(time.RFC3339),
		)

		var req facilitatorRequest
		if err := json.Unmarshal(body, &req); err != nil {
			slog.Warn("invalid request body", "error", err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(settleResponse{
			Success:     true,
			Payer:       "0x0000000000000000000000000000000000000001",
			Transaction: "0xmocktx123abc456def789",
			Network:     "eip155:84532",
		})
	})

	addr := fmt.Sprintf(":%s", port)
	slog.Info("starting mock facilitator", "addr", addr)
	slog.Info("endpoints", "verify", "POST /verify", "settle", "POST /settle")

	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
