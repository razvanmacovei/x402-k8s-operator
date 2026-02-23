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

type verifyResponse struct {
	Valid              bool   `json:"valid"`
	InvalidationReason *string `json:"invalidationReason"`
}

type settleResponse struct {
	Success bool   `json:"success"`
	TxHash  string `json:"txHash"`
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

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(verifyResponse{
			Valid:              true,
			InvalidationReason: nil,
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

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(settleResponse{
			Success: true,
			TxHash:  "0xmocktx123",
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
