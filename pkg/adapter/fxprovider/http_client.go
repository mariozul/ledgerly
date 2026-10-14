package fxprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"time"

	"github.com/agent-orchestration/ledgerly/pkg/domain"
)

// HTTPClient implements domain.FXProvider using an HTTP client to call an
// external FX rate API. It includes retry logic with exponential backoff
// for transient errors (FR-FX-02).
type HTTPClient struct {
	config Config
	client *http.Client
}

// fxRateResponse represents the JSON response from the external FX API.
type fxRateResponse struct {
	Rate   float64 `json:"rate"`
	Base   string  `json:"base"`
	Target string  `json:"target"`
}

// NewHTTPClient creates a new HTTPClient with the given configuration.
// Returns an error if the config is invalid.
func NewHTTPClient(config Config) (*HTTPClient, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid fxprovider config: %w", err)
	}

	client := &http.Client{
		Timeout: config.Timeout,
		// Do not follow redirects for the external API
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return &HTTPClient{
		config: config,
		client: client,
	}, nil
}

// GetRate fetches the exchange rate from the source currency to the target currency.
// It implements domain.FXProvider (RULE-ARCH-02).
//
// Returns:
//   - FXRate on success
//   - ErrFXProviderUnavailable for transient errors (FR-FX-02)
//   - ErrFXRateNotFound if the currency pair is not supported (FR-FX-02)
//
// Context propagation is enforced (RULE-RES-01) and response bodies are
// properly closed (RULE-RES-03).
func (c *HTTPClient) GetRate(ctx context.Context, from, to domain.Currency) (domain.FXRate, error) {
	// Build the request URL
	u, err := url.Parse(c.config.BaseURL + "/v1/rates")
	if err != nil {
		return 0, domain.NewDomainError(domain.ErrFXProviderUnavailable,
			fmt.Sprintf("invalid base URL: %v", err))
	}

	q := u.Query()
	q.Set("from", string(from))
	q.Set("to", string(to))
	u.RawQuery = q.Encode()

	// Create the HTTP request with context (RULE-RES-01)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, domain.NewDomainError(domain.ErrFXProviderUnavailable,
			fmt.Sprintf("failed to create request: %v", err))
	}

	// Set headers
	req.Header.Set("Accept", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	// Execute with retry logic
	var lastErr error
	attempts := 1 // at least 1 attempt
	if c.config.RetryMaxAttempts > 0 {
		attempts = c.config.RetryMaxAttempts
	}

	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			// Calculate backoff: base * 2^(attempt-1), capped at max interval
			backoff := float64(c.config.RetryBaseInterval) * math.Pow(2, float64(attempt-1))
			if backoff > float64(c.config.RetryMaxInterval) {
				backoff = float64(c.config.RetryMaxInterval)
			}
			backoffDuration := time.Duration(backoff)

			// Sleep with context awareness
			select {
			case <-ctx.Done():
				return 0, domain.NewDomainError(domain.ErrFXProviderUnavailable,
					fmt.Sprintf("request cancelled during retry backoff: %v", ctx.Err()))
			case <-time.After(backoffDuration):
			}
		}

		rate, err := c.doRequest(req)
		if err == nil {
			return rate, nil
		}

		lastErr = err

		// Check if the error is permanent - if so, don't retry
		if !isTransientError(err) {
			break
		}

		// Check if context was cancelled
		if ctx.Err() != nil {
			return 0, domain.NewDomainError(domain.ErrFXProviderUnavailable,
				fmt.Sprintf("request cancelled: %v", ctx.Err()))
		}
	}

	return 0, lastErr
}

// doRequest executes a single HTTP request and returns the FX rate.
// It handles response body cleanup (RULE-RES-03).
func (c *HTTPClient) doRequest(req *http.Request) (domain.FXRate, error) {
	resp, err := c.client.Do(req)
	if err != nil {
		// Network-level errors are transient
		return 0, domain.NewDomainError(domain.ErrFXProviderUnavailable,
			fmt.Sprintf("HTTP request failed: %v", err))
	}

	// Ensure response body is closed (RULE-RES-03)
	defer resp.Body.Close()

	// Read the response body for error messages
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, domain.NewDomainError(domain.ErrFXProviderUnavailable,
			fmt.Sprintf("failed to read response body: %v", err))
	}

	// Handle HTTP status codes
	switch {
	case resp.StatusCode == http.StatusOK:
		// Parse the JSON response
		var rateResp fxRateResponse
		if err := json.Unmarshal(body, &rateResp); err != nil {
			return 0, domain.NewDomainError(domain.ErrFXProviderUnavailable,
				fmt.Sprintf("failed to parse response: %v", err))
		}
		return domain.FXRate(rateResp.Rate), nil

	case resp.StatusCode == http.StatusNotFound,
		resp.StatusCode == http.StatusUnprocessableEntity,
		resp.StatusCode == http.StatusBadRequest:
		// Permanent errors: currency pair not supported or invalid request
		return 0, domain.NewDomainError(domain.ErrFXRateNotFound,
			fmt.Sprintf("FX rate not found for pair: %s", string(body)))

	case resp.StatusCode == http.StatusUnauthorized,
		resp.StatusCode == http.StatusForbidden:
		// Authentication/authorization failures - treat as provider unavailable
		return 0, domain.NewDomainError(domain.ErrFXProviderUnavailable,
			fmt.Sprintf("authentication failed: %s", string(body)))

	default:
		// 5xx responses and other unexpected status codes are transient
		return 0, domain.NewDomainError(domain.ErrFXProviderUnavailable,
			fmt.Sprintf("unexpected HTTP status %d: %s", resp.StatusCode, string(body)))
	}
}

// isTransientError determines if an error is potentially transient and should be retried.
func isTransientError(err error) bool {
	// If the error wraps ErrFXRateNotFound, it's permanent
	if domain.IsDomainError(err, domain.ErrFXRateNotFound) {
		return false
	}
	// All other errors (including ErrFXProviderUnavailable) are potentially transient
	return true
}
