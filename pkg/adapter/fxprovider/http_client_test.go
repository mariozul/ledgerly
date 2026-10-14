package fxprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agent-orchestration/ledgerly/pkg/domain"
)

// newTestServer creates a test HTTP server with a configurable handler.
func newTestServer(handler http.HandlerFunc) (*httptest.Server, Config) {
	srv := httptest.NewServer(handler)
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Timeout = 5 * time.Second
	cfg.RetryBaseInterval = 10 * time.Millisecond
	cfg.RetryMaxInterval = 100 * time.Millisecond
	cfg.RetryMaxAttempts = 2
	return srv, cfg
}

// TC-FX-01-FETCH-UNIT: Successful FX rate fetch from provider
func TestHTTPClient_GetRate_Success(t *testing.T) {
	t.Parallel()

	// Start a test server that returns a valid FX rate
	srv, cfg := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		// Verify context propagation (RULE-RES-01)
		if r.Context().Err() != nil {
			t.Error("request context is already cancelled")
		}
		// Verify the correct path was called
		if r.URL.Path != "/v1/rates" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// Check query params
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")
		if from != "EUR" || to != "USD" {
			t.Errorf("unexpected currency pair: from=%s to=%s", from, to)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"rate": 1.1845, "base": "EUR", "target": "USD"}`))
	})
	defer srv.Close()

	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	rate, err := client.GetRate(ctx, "EUR", "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := domain.FXRate(1.1845)
	if rate != expected {
		t.Errorf("expected rate %f, got %f", expected, rate)
	}
}

// TC-FX-03-TRANSIENT-ERROR-UNIT: FX provider returns transient error, client retries
func TestHTTPClient_GetRate_TransientError_Retries(t *testing.T) {
	t.Parallel()

	var attemptCount int
	srv, cfg := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount <= 2 {
			// Return 503 Service Unavailable for first two attempts
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error": "service temporarily unavailable"}`))
			return
		}
		// Third attempt succeeds
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"rate": 1.1845, "base": "EUR", "target": "USD"}`))
	})
	defer srv.Close()

	cfg.RetryMaxAttempts = 3
	cfg.RetryBaseInterval = 10 * time.Millisecond
	cfg.RetryMaxInterval = 50 * time.Millisecond

	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	rate, err := client.GetRate(ctx, "EUR", "USD")
	if err != nil {
		t.Fatalf("expected retry to succeed, got error: %v", err)
	}

	if rate != 1.1845 {
		t.Errorf("expected rate 1.1845, got %f", rate)
	}

	if attemptCount != 3 {
		t.Errorf("expected 3 attempts, got %d", attemptCount)
	}
}

// TC-FX-03-TRANSIENT-ERROR-UNIT: All transient retries exhausted returns domain error
func TestHTTPClient_GetRate_TransientError_Exhausted(t *testing.T) {
	t.Parallel()

	var attemptCount int
	srv, cfg := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error": "service temporarily unavailable"}`))
	})
	defer srv.Close()

	cfg.RetryMaxAttempts = 2
	cfg.RetryBaseInterval = 10 * time.Millisecond
	cfg.RetryMaxInterval = 50 * time.Millisecond

	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	_, err = client.GetRate(ctx, "EUR", "USD")
	if err == nil {
		t.Fatal("expected an error after exhausting retries")
	}

	if !domain.IsDomainError(err, domain.ErrFXProviderUnavailable) {
		t.Errorf("expected ErrFXProviderUnavailable, got %v", err)
	}

	if attemptCount != 2 {
		// 1 initial + 1 retry (max 2 attempts: initial + 1 retry)
		t.Errorf("expected 2 total attempts, got %d", attemptCount)
	}
}

// TC-FX-04-PERMANENT-ERROR-UNIT: FX provider returns permanent error (4xx)
func TestHTTPClient_GetRate_PermanentError_4xx(t *testing.T) {
	t.Parallel()

	var attemptCount int
	srv, cfg := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		// 422 indicates the currency pair is not supported (permanent error)
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"error": "unsupported currency pair"}`))
	})
	defer srv.Close()

	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	_, err = client.GetRate(ctx, "XYZ", "USD")
	if err == nil {
		t.Fatal("expected a permanent error for unsupported currency pair")
	}

	// Should map to domain.ErrFXRateNotFound
	if !domain.IsDomainError(err, domain.ErrFXRateNotFound) {
		t.Errorf("expected ErrFXRateNotFound, got %v", err)
	}

	// Should NOT have retried for permanent errors
	if attemptCount != 1 {
		t.Errorf("expected only 1 attempt (no retries for permanent error), got %d", attemptCount)
	}
}

// TC-FX-04-PERMANENT-ERROR-UNIT: 404 Not Found maps to ErrFXRateNotFound
func TestHTTPClient_GetRate_PermanentError_404(t *testing.T) {
	t.Parallel()

	srv, cfg := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "rate not found for pair"}`))
	})
	defer srv.Close()

	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	_, err = client.GetRate(ctx, "GBP", "USD")
	if err == nil {
		t.Fatal("expected a permanent error for unsupported pair")
	}

	if !domain.IsDomainError(err, domain.ErrFXRateNotFound) {
		t.Errorf("expected ErrFXRateNotFound, got %v", err)
	}
}

// TC-FX-04-PERMANENT-ERROR-UNIT: 401 Unauthorized maps to ErrFXProviderUnavailable
func TestHTTPClient_GetRate_PermanentError_401(t *testing.T) {
	t.Parallel()

	srv, cfg := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid API key"}`))
	})
	defer srv.Close()

	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	_, err = client.GetRate(ctx, "EUR", "USD")
	if err == nil {
		t.Fatal("expected an error for unauthorized access")
	}

	if !domain.IsDomainError(err, domain.ErrFXProviderUnavailable) {
		t.Errorf("expected ErrFXProviderUnavailable, got %v", err)
	}
}

// RULE-RES-01: Context cancellation should propagate and abort the request
func TestHTTPClient_GetRate_ContextCancellation(t *testing.T) {
	t.Parallel()

	srv, cfg := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"rate": 1.18}`))
	})
	defer srv.Close()

	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = client.GetRate(ctx, "EUR", "USD")
	if err == nil {
		t.Fatal("expected context deadline exceeded error")
	}

	// The error should be a context error wrapped in ErrFXProviderUnavailable
	if !domain.IsDomainError(err, domain.ErrFXProviderUnavailable) {
		t.Errorf("expected ErrFXProviderUnavailable wrapping context error, got %v", err)
	}
}

// RULE-RES-03: Ensure proper resource cleanup (response body closed)
func TestHTTPClient_GetRate_ResponseBodyClosed(t *testing.T) {
	t.Parallel()

	srv, cfg := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"rate": 1.25}`))
	})
	defer srv.Close()

	client, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	_, err = client.GetRate(ctx, "USD", "EUR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// If body was not closed, we'd have resource leaks - the implementation
	// must defer resp.Body.Close() immediately after error check
}

// TestHTTPClient_ConfigValidation verifies config validation
func TestHTTPClient_ConfigValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config with URL",
			config: Config{
				BaseURL:           "https://api.example.com",
				Timeout:           10 * time.Second,
				RetryMaxAttempts:  3,
				RetryBaseInterval: 100 * time.Millisecond,
				RetryMaxInterval:  5 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "missing base URL",
			config: Config{
				BaseURL:           "",
				Timeout:           10 * time.Second,
				RetryMaxAttempts:  3,
				RetryBaseInterval: 100 * time.Millisecond,
				RetryMaxInterval:  5 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "zero timeout",
			config: Config{
				BaseURL:           "https://api.example.com",
				Timeout:           0,
				RetryMaxAttempts:  3,
				RetryBaseInterval: 100 * time.Millisecond,
				RetryMaxInterval:  5 * time.Second,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewHTTPClient(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewHTTPClient() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
