package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Context keys
// ---------------------------------------------------------------------------

type contextKey string

const (
	// RequestIDKey is the context key for the request ID.
	RequestIDKey contextKey = "request_id"
)

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

// withRequestID adds a unique request ID to each request context.
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = uuid.New().String()
		}
		ctx := context.WithValue(r.Context(), RequestIDKey, reqID)
		w.Header().Set("X-Request-ID", reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// WithLogging is middleware that adds structured logging for HTTP requests.
// Sensitive data is not logged (RULE-SEC-02).
func WithLogging(logger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger.InfoContext(r.Context(), "HTTP request",
				slog.String("component", "http"),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("remote_addr", r.RemoteAddr),
			)
			next.ServeHTTP(w, r)
		})
	}
}

// NewRouter creates the main HTTP router with all routes registered.
func NewRouter(invoiceHandler *InvoiceHandler, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	// Invoice routes
	mux.HandleFunc("POST /v1/invoices", invoiceHandler.HandleCreateInvoice)

	// Health endpoints (RULE-OBS-04)
	mux.HandleFunc("GET /livez", invoiceHandler.HandleLivez)
	mux.HandleFunc("GET /readyz", invoiceHandler.HandleReadyz)

	// Apply middleware chain
	var handler http.Handler = mux
	handler = WithLogging(logger)(handler)
	handler = withRequestID(handler)

	return handler
}
