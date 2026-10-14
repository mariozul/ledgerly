package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Server wraps an HTTP server with graceful shutdown capabilities.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

// NewServer creates a new Server with the given address, handler, and logger.
func NewServer(addr string, handler http.Handler, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}

	httpServer := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &Server{
		httpServer: httpServer,
		logger:     logger,
	}
}

// Start begins listening and serving HTTP requests.
// This is a blocking call; run it in a goroutine.
func (s *Server) Start() error {
	s.logger.InfoContext(context.Background(), "HTTP server starting",
		slog.String("component", "server"),
		slog.String("addr", s.httpServer.Addr),
	)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server with a timeout.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.InfoContext(ctx, "HTTP server shutting down",
		slog.String("component", "server"),
	)
	return s.httpServer.Shutdown(ctx)
}

// IsErrServerClosed checks if the error is http.ErrServerClosed.
func IsErrServerClosed(err error) bool {
	return errors.Is(err, http.ErrServerClosed)
}
