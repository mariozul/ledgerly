package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/agent-orchestration/ledgerly/pkg/domain"
	"github.com/agent-orchestration/ledgerly/pkg/service"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockInvoiceService struct {
	createInvoiceFn func(ctx context.Context, req service.CreateInvoiceRequest) (*domain.Invoice, error)
}

func (m *mockInvoiceService) CreateInvoice(ctx context.Context, req service.CreateInvoiceRequest) (*domain.Invoice, error) {
	if m.createInvoiceFn != nil {
		return m.createInvoiceFn(ctx, req)
	}
	return nil, nil
}

// mockDurationRecorder implements DurationRecorder for testing.
type mockDurationRecorder struct {
	lastLabels map[string]string
	lastValue  float64
}

func (m *mockDurationRecorder) Record(labels map[string]string, value float64) error {
	m.lastLabels = labels
	m.lastValue = value
	return nil
}

// mockCounterRecorder implements CounterRecorder for testing.
type mockCounterRecorder struct {
	lastLabels map[string]string
	lastValue  float64
}

func (m *mockCounterRecorder) Record(labels map[string]string, value float64) error {
	m.lastLabels = labels
	m.lastValue = value
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestInvoiceHandler(t *testing.T, svc InvoiceServiceInterface, logger *slog.Logger) *InvoiceHandler {
	t.Helper()
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	metrics := &HandlerMetrics{}
	return NewInvoiceHandler(svc, logger, metrics)
}

// newTestRouter creates a test HTTP handler with the invoice routes registered.
func newTestRouter(t *testing.T, svc InvoiceServiceInterface, logger *slog.Logger) http.Handler {
	t.Helper()
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	h := NewInvoiceHandler(svc, logger, &HandlerMetrics{})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/invoices", h.HandleCreateInvoice)
	mux.HandleFunc("GET /livez", h.HandleLivez)
	mux.HandleFunc("GET /readyz", h.HandleReadyz)
	return withRequestID(mux)
}

// ---------------------------------------------------------------------------
// TC-INV-02-API-INT: Successful POST /v1/invoices through HTTP
// ---------------------------------------------------------------------------

func TestInvoiceHandler_CreateInvoice_Success(t *testing.T) {
	svc := &mockInvoiceService{
		createInvoiceFn: func(ctx context.Context, req service.CreateInvoiceRequest) (*domain.Invoice, error) {
			now := time.Now().UTC()
			total := decimal.NewFromFloat(100.00).Add(decimal.NewFromFloat(200.00))
			return &domain.Invoice{
				ID:             domain.InvoiceID("inv-123"),
				TenantID:       req.TenantID,
				Currency:       req.Currency,
				BaseCurrency:   req.BaseCurrency,
				FXRate:         1.25,
				TotalAmount:    total,
				IdempotencyKey: req.IdempotencyKey,
				CreatedAt:      now,
				CreatedBy:      "user-1",
				LineItems: []domain.InvoiceLine{
					{ID: "line-1", InvoiceID: "inv-123", Description: "Item 1", Amount: decimal.NewFromFloat(100.00), CreatedAt: now, CreatedBy: "user-1"},
					{ID: "line-2", InvoiceID: "inv-123", Description: "Item 2", Amount: decimal.NewFromFloat(200.00), CreatedAt: now, CreatedBy: "user-1"},
				},
			}, nil
		},
	}

	handler := newTestRouter(t, svc, nil)

	body := map[string]interface{}{
		"currency":        "EUR",
		"base_currency":   "USD",
		"idempotency_key": uuid.New().String(),
		"line_items": []map[string]interface{}{
			{"description": "Item 1", "amount": "100.00"},
			{"description": "Item 2", "amount": "200.00"},
		},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/v1/invoices", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-1")
	req.Header.Set("X-User-ID", "user-1")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 201 Created, got %d. Body: %s", resp.StatusCode, string(respBody))
	}

	var respData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if respData["id"] != "inv-123" {
		t.Errorf("Expected invoice ID 'inv-123', got %v", respData["id"])
	}
	if respData["tenant_id"] != "tenant-1" {
		t.Errorf("Expected tenant_id 'tenant-1', got %v", respData["tenant_id"])
	}
}

// TC-INV-02-API-INT: Error cases - invalid input returns 400
func TestInvoiceHandler_CreateInvoice_InvalidInput(t *testing.T) {
	svc := &mockInvoiceService{
		createInvoiceFn: func(ctx context.Context, req service.CreateInvoiceRequest) (*domain.Invoice, error) {
			return nil, domain.NewDomainError(domain.ErrInvalidInput, "invalid input")
		},
	}

	handler := newTestRouter(t, svc, nil)

	tests := []struct {
		name       string
		body       map[string]interface{}
		expectCode int
	}{
		{
			name: "missing currency",
			body: map[string]interface{}{
				"base_currency":   "USD",
				"idempotency_key": uuid.New().String(),
				"line_items":      []map[string]interface{}{{"description": "Item", "amount": "100.00"}},
			},
			expectCode: http.StatusBadRequest,
		},
		{
			name: "missing line_items",
			body: map[string]interface{}{
				"currency":        "EUR",
				"base_currency":   "USD",
				"idempotency_key": uuid.New().String(),
				"line_items":      []map[string]interface{}{},
			},
			expectCode: http.StatusBadRequest,
		},
		{
			name: "missing idempotency_key",
			body: map[string]interface{}{
				"currency":      "EUR",
				"base_currency": "USD",
				"line_items":    []map[string]interface{}{{"description": "Item", "amount": "100.00"}},
			},
			expectCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest("POST", "/v1/invoices", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Tenant-ID", "tenant-1")
			req.Header.Set("X-User-ID", "user-1")

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectCode {
				respBody, _ := io.ReadAll(resp.Body)
				t.Errorf("Expected %d, got %d. Body: %s", tt.expectCode, resp.StatusCode, string(respBody))
			}
		})
	}
}

// TC-INV-02-API-INT: Domain error mappings
func TestInvoiceHandler_CreateInvoice_DomainErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		domainErr  error
		expectCode int
	}{
		{
			name:       "invalid input -> 400",
			domainErr:  domain.NewDomainError(domain.ErrInvalidInput, "invalid"),
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "already exists -> 409",
			domainErr:  domain.NewDomainError(domain.ErrAlreadyExists, "exists"),
			expectCode: http.StatusConflict,
		},
		{
			name:       "FX provider unavailable -> 503",
			domainErr:  domain.NewDomainError(domain.ErrFXProviderUnavailable, "unavailable"),
			expectCode: http.StatusServiceUnavailable,
		},
		{
			name:       "FX rate not found -> 422",
			domainErr:  domain.NewDomainError(domain.ErrFXRateNotFound, "not found"),
			expectCode: http.StatusUnprocessableEntity,
		},
		{
			name:       "unauthorized -> 401",
			domainErr:  domain.NewDomainError(domain.ErrUnauthorized, "unauthorized"),
			expectCode: http.StatusUnauthorized,
		},
		{
			name:       "forbidden -> 403",
			domainErr:  domain.NewDomainError(domain.ErrForbidden, "forbidden"),
			expectCode: http.StatusForbidden,
		},
		{
			name:       "internal -> 500",
			domainErr:  domain.NewDomainError(domain.ErrInternal, "internal"),
			expectCode: http.StatusInternalServerError,
		},
		{
			name:       "generic error -> 500",
			domainErr:  domain.NewDomainError(domain.ErrInternal, "unknown"),
			expectCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockInvoiceService{
				createInvoiceFn: func(ctx context.Context, req service.CreateInvoiceRequest) (*domain.Invoice, error) {
					return nil, tt.domainErr
				},
			}

			handler := newTestRouter(t, svc, nil)

			body := map[string]interface{}{
				"currency":        "EUR",
				"base_currency":   "USD",
				"idempotency_key": uuid.New().String(),
				"line_items":      []map[string]interface{}{{"description": "Item", "amount": "100.00"}},
			}
			bodyBytes, _ := json.Marshal(body)

			req := httptest.NewRequest("POST", "/v1/invoices", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Tenant-ID", "tenant-1")
			req.Header.Set("X-User-ID", "user-1")

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectCode {
				respBody, _ := io.ReadAll(resp.Body)
				t.Errorf("Expected %d, got %d. Body: %s", tt.expectCode, resp.StatusCode, string(respBody))
			}
		})
	}
}

// TC-OBS-01-HEALTH-INT: /livez and /readyz endpoints return 200 OK
func TestInvoiceHandler_HealthEndpoints(t *testing.T) {
	svc := &mockInvoiceService{}
	handler := newTestRouter(t, svc, nil)

	t.Run("/livez returns 200", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/livez", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected 200, got %d. Body: %s", resp.StatusCode, string(respBody))
		}
	})

	t.Run("/readyz returns 200", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/readyz", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected 200, got %d. Body: %s", resp.StatusCode, string(respBody))
		}
	})
}

// TC-OBS-02-LOGS-UNIT: Sensitive data is masked in structured logs (RULE-SEC-02)
func TestInvoiceHandler_LogsDoNotContainSensitiveData(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	svc := &mockInvoiceService{
		createInvoiceFn: func(ctx context.Context, req service.CreateInvoiceRequest) (*domain.Invoice, error) {
			return nil, domain.NewDomainError(domain.ErrInvalidInput, "invalid request")
		},
	}

	handler := newTestRouter(t, svc, logger)

	body := map[string]interface{}{
		"currency":        "EUR",
		"base_currency":   "USD",
		"idempotency_key": uuid.New().String(),
		"line_items": []map[string]interface{}{
			{"description": "Consulting", "amount": "10000.00"},
		},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/v1/invoices", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-1")
	req.Header.Set("X-User-ID", "user-1")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	_ = w.Result()

	logOutput := logBuf.String()
	// These patterns should NOT appear in logs (RULE-SEC-02)
	sensitivePatterns := []string{"10000.00", "Consulting", "idempotency_key"}
	for _, pattern := range sensitivePatterns {
		if strings.Contains(logOutput, pattern) {
			t.Errorf("RULE-SEC-02 Violation: Log contains sensitive data %q. Log output: %s", pattern, logOutput)
		}
	}
}

// TC-SEC-01-TENANT-ISOLATION-INT: Tenant ID is securely extracted from context, not from payload
func TestInvoiceHandler_TenantIDFromContext(t *testing.T) {
	var capturedTenantID domain.TenantID

	svc := &mockInvoiceService{
		createInvoiceFn: func(ctx context.Context, req service.CreateInvoiceRequest) (*domain.Invoice, error) {
			capturedTenantID = req.TenantID
			now := time.Now().UTC()
			return &domain.Invoice{
				ID:             domain.InvoiceID("inv-456"),
				TenantID:       req.TenantID,
				Currency:       req.Currency,
				BaseCurrency:   req.BaseCurrency,
				FXRate:         1.0,
				TotalAmount:    decimal.NewFromFloat(100.00),
				IdempotencyKey: req.IdempotencyKey,
				CreatedAt:      now,
				CreatedBy:      "user-1",
			}, nil
		},
	}

	handler := newTestRouter(t, svc, nil)

	// The request body contains NO tenant_id - tenant_id should come from header/context
	body := map[string]interface{}{
		"currency":        "USD",
		"base_currency":   "EUR",
		"idempotency_key": uuid.New().String(),
		"line_items":      []map[string]interface{}{{"description": "Item", "amount": "100.00"}},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/v1/invoices", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-secure-1")
	req.Header.Set("X-User-ID", "user-1")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 201, got %d. Body: %s", resp.StatusCode, string(respBody))
	}

	if capturedTenantID != "tenant-secure-1" {
		t.Errorf("RULE-SEC-01 Violation: Expected tenant_id 'tenant-secure-1' from context/headers, got %q", capturedTenantID)
	}
}

// Test serializer: Validate that monetary values are not returned as raw internal types
func TestInvoiceHandler_ResponseFormat(t *testing.T) {
	svc := &mockInvoiceService{
		createInvoiceFn: func(ctx context.Context, req service.CreateInvoiceRequest) (*domain.Invoice, error) {
			now := time.Now().UTC()
			return &domain.Invoice{
				ID:             domain.InvoiceID("inv-789"),
				TenantID:       req.TenantID,
				Currency:       req.Currency,
				BaseCurrency:   req.BaseCurrency,
				FXRate:         1.5,
				TotalAmount:    decimal.NewFromFloat(150.00),
				IdempotencyKey: req.IdempotencyKey,
				CreatedAt:      now,
				CreatedBy:      "user-1",
				LineItems: []domain.InvoiceLine{
					{ID: "line-1", InvoiceID: "inv-789", Description: "Service", Amount: decimal.NewFromFloat(150.00), CreatedAt: now, CreatedBy: "user-1"},
				},
			}, nil
		},
	}

	handler := newTestRouter(t, svc, nil)

	body := map[string]interface{}{
		"currency":        "GBP",
		"base_currency":   "USD",
		"idempotency_key": uuid.New().String(),
		"line_items":      []map[string]interface{}{{"description": "Service", "amount": "150.00"}},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/v1/invoices", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-1")
	req.Header.Set("X-User-ID", "user-1")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	var respData InvoiceResponse
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if respData.ID != "inv-789" {
		t.Errorf("Expected ID inv-789, got %s", respData.ID)
	}
	if respData.TenantID != "tenant-1" {
		t.Errorf("Expected tenant_id tenant-1, got %s", respData.TenantID)
	}
	if respData.FXRate != 1.5 {
		t.Errorf("Expected FXRate 1.5, got %f", respData.FXRate)
	}
	if respData.Currency != "GBP" {
		t.Errorf("Expected currency GBP, got %s", respData.Currency)
	}
	if len(respData.LineItems) != 1 {
		t.Errorf("Expected 1 line item, got %d", len(respData.LineItems))
	}
}

// Test missing tenant header returns 401
func TestInvoiceHandler_MissingTenantHeader(t *testing.T) {
	handler := newTestRouter(t, &mockInvoiceService{}, nil)

	body := map[string]interface{}{
		"currency":        "EUR",
		"base_currency":   "USD",
		"idempotency_key": uuid.New().String(),
		"line_items":      []map[string]interface{}{{"description": "Item", "amount": "100.00"}},
	}
	bodyBytes, _ := json.Marshal(body)

	// No X-Tenant-ID header
	req := httptest.NewRequest("POST", "/v1/invoices", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		respBody, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected 401, got %d. Body: %s", resp.StatusCode, string(respBody))
	}
}
