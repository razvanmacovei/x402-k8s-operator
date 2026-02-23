package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/razvanmacovei/x402-k8s-operator/internal/routestore"
)

// Server is the gateway HTTP server that implements manager.Runnable.
type Server struct {
	addr    string
	handler *Handler
	srv     *http.Server
}

// NewServer creates a new gateway server.
func NewServer(addr string, store *routestore.Store) *Server {
	handler := NewHandler(store)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.Handle("/", handler)

	return &Server{
		addr:    addr,
		handler: handler,
		srv: &http.Server{
			Addr:         addr,
			Handler:      mux,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
	}
}

// Start implements manager.Runnable. It starts the HTTP server and blocks until
// the context is cancelled, then gracefully shuts down.
func (s *Server) Start(ctx context.Context) error {
	slog.Info("starting x402 gateway", "addr", s.addr)

	// Shut down gracefully when context is cancelled.
	go func() {
		<-ctx.Done()
		slog.Info("shutting down gateway server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("gateway graceful shutdown failed", "error", err)
		}
	}()

	if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("gateway server failed: %w", err)
	}
	slog.Info("gateway server stopped")
	return nil
}
