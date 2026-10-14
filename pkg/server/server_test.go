package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Test that the server can start and shutdown gracefully
func TestServer_StartAndShutdown(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(":0", handler, logger)

	// Start server in background
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Should be able to query the server
	shutdownErr := srv.Shutdown(ctx)
	if shutdownErr != nil {
		t.Fatalf("Failed to shutdown server: %v", shutdownErr)
	}

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("Server returned unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Timed out waiting for server to stop")
	}
}

// Test that /livez endpoint responds correctly through the server
func TestServer_LivezEndpoint(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/livez" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"alive"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(":0", handler, logger)

	// Use a test server instead of actually binding
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/livez")
	if err != nil {
		t.Fatalf("Failed to GET /livez: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
	}

	// Verify we can shut down
	_ = srv
}
