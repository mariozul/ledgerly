package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/agent-orchestration/ledgerly/pkg/domain"
	"github.com/agent-orchestration/ledgerly/pkg/service"
)

// ---------------------------------------------------------------------------
// Interfaces
// ---------------------------------------------------------------------------

// InvoiceServiceInterface defines the contract for the invoice service
// that the handler depends on. Clean Architecture: outer layer calls inner layer (RULE-ARCH-02).
type InvoiceServiceInterface interface {
	CreateInvoice(ctx context.Context, req service.CreateInvoiceRequest) (*domain.Invoice, error)
}

// ---------------------------------------------------------------------------
// InvoiceHandler
// ---------------------------------------------------------------------------

// InvoiceHandler handles HTTP requests for the invoice API.
type InvoiceHandler struct {
	svc     InvoiceServiceInterface
	logger  *slog.Logger
	metrics *HandlerMetrics
}

// HandlerMetrics holds metric collectors for HTTP operations.
type HandlerMetrics struct {
	HTTPDuration         DurationRecorder
	InvoiceCreationCount CounterRecorder
}

// DurationRecorder interface for recording duration metrics.
type DurationRecorder interface {
	Record(labels map[string]string, value float64) error
}

// CounterRecorder interface for recording counter metrics.
type CounterRecorder interface {
	Record(labels map[string]string, value float64) error
}

// NewInvoiceHandler creates a new InvoiceHandler.
func NewInvoiceHandler(svc InvoiceServiceInterface, logger *slog.Logger, metrics *HandlerMetrics) *InvoiceHandler {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	if metrics == nil {
		metrics = &HandlerMetrics{}
	}
	return &InvoiceHandler{
		svc:     svc,
		logger:  logger,
		metrics: metrics,
	}
}

// ---------------------------------------------------------------------------
// Handler: POST /v1/invoices
// ---------------------------------------------------------------------------

// HandleCreateInvoice handles POST /v1/invoices.
// It extracts tenant_id securely from the request context/headers (RULE-SEC-01, FR-INV-01).
func (h *InvoiceHandler) HandleCreateInvoice(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	statusCode := http.StatusInternalServerError

	defer func() {
		duration := time.Since(start).Seconds()
		if h.metrics.HTTPDuration != nil {
			_ = h.metrics.HTTPDuration.Record(map[string]string{
				"path":        "/v1/invoices",
				"method":      "POST",
				"status_code": http.StatusText(statusCode),
			}, duration)
		}
	}()

	// Extract tenant ID from request context/headers - RULE-SEC-01
	// NEVER from the request body (FR-INV-01)
	tenantID := extractTenantID(r)
	if tenantID == "" {
		statusCode = http.StatusUnauthorized
		h.logger.ErrorContext(r.Context(), "missing tenant authentication",
			slog.String("component", "invoice_handler"),
		)
		writeError(w, statusCode, "UNAUTHORIZED", "missing or invalid tenant authentication")
		return
	}

	// Parse request body
	var req CreateInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		statusCode = http.StatusBadRequest
		h.logger.WarnContext(r.Context(), "failed to decode request body",
			slog.String("component", "invoice_handler"),
		)
		writeError(w, statusCode, "INVALID_INPUT", "invalid request body")
		return
	}

	// Convert handler DTOs to service request
	svcLineItems := make([]service.LineItemInput, len(req.LineItems))
	for i, item := range req.LineItems {
		svcLineItems[i] = service.LineItemInput{
			Description: item.Description,
			Amount:      item.Amount,
		}
	}

	svcReq := service.CreateInvoiceRequest{
		TenantID:       domain.TenantID(tenantID),
		Currency:       domain.Currency(req.Currency),
		BaseCurrency:   domain.Currency(req.BaseCurrency),
		IdempotencyKey: domain.IdempotencyKey(req.IdempotencyKey),
		LineItems:      svcLineItems,
		CreatedBy:      extractUserID(r),
	}

	// Call service
	invoice, err := h.svc.CreateInvoice(r.Context(), svcReq)
	if err != nil {
		statusCode = mapDomainErrorToHTTP(err)
		// Do NOT log the full error details - it may contain sensitive info (RULE-SEC-02)
		h.logger.WarnContext(r.Context(), "invoice creation failed",
			slog.String("component", "invoice_handler"),
			slog.String("error_code", errorCodeFromDomain(err)),
		)
		writeError(w, statusCode, errorCodeFromDomain(err), err.Error())
		return
	}

	// Build response
	statusCode = http.StatusCreated
	resp := invoiceToResponse(invoice)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.ErrorContext(r.Context(), "failed to encode response",
			slog.String("component", "invoice_handler"),
		)
	}

	// Record success metric with bounded cardinality labels (RULE-OBS-03)
	if h.metrics.InvoiceCreationCount != nil {
		_ = h.metrics.InvoiceCreationCount.Record(map[string]string{
			"status":   "success",
			"currency": string(invoice.Currency),
		}, 1)
	}
}

// ---------------------------------------------------------------------------
// Health endpoints (RULE-OBS-04)
// ---------------------------------------------------------------------------

// HandleLivez handles GET /livez - returns 200 if process is alive.
func (h *InvoiceHandler) HandleLivez(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"alive"}`))
}

// HandleReadyz handles GET /readyz - returns 200 if service is ready to serve traffic.
// In production, this would check database connectivity and dependent services.
func (h *InvoiceHandler) HandleReadyz(w http.ResponseWriter, r *http.Request) {
	// TODO: In production, check database connectivity here
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// extractTenantID extracts the tenant ID from request headers (RULE-SEC-01).
// The tenant ID MUST come from authenticated context, NOT from the request body.
func extractTenantID(r *http.Request) string {
	// In production, this would extract from JWT claims or gateway-verified headers.
	// For now, we use the X-Tenant-ID header which should be set by the API gateway.
	return r.Header.Get("X-Tenant-ID")
}

// extractUserID extracts the user ID from request headers.
func extractUserID(r *http.Request) string {
	return r.Header.Get("X-User-ID")
}

// mapDomainErrorToHTTP maps domain errors to HTTP status codes (Error Catalog from ReqSpec).
func mapDomainErrorToHTTP(err error) int {
	if err == nil {
		return http.StatusInternalServerError
	}

	var de *domain.DomainError
	if errors.As(err, &de) {
		switch {
		case errors.Is(de.Err, domain.ErrInvalidInput):
			return http.StatusBadRequest
		case errors.Is(de.Err, domain.ErrInvoiceNotFound):
			return http.StatusNotFound
		case errors.Is(de.Err, domain.ErrAlreadyExists):
			return http.StatusConflict
		case errors.Is(de.Err, domain.ErrFXProviderUnavailable):
			return http.StatusServiceUnavailable
		case errors.Is(de.Err, domain.ErrFXRateNotFound):
			return http.StatusUnprocessableEntity
		case errors.Is(de.Err, domain.ErrUnauthorized):
			return http.StatusUnauthorized
		case errors.Is(de.Err, domain.ErrForbidden):
			return http.StatusForbidden
		default:
			return http.StatusInternalServerError
		}
	}

	return http.StatusInternalServerError
}

// errorCodeFromDomain extracts the error code string from a domain error.
func errorCodeFromDomain(err error) string {
	if err == nil {
		return "INTERNAL_SERVER_ERROR"
	}

	var de *domain.DomainError
	if errors.As(err, &de) {
		switch {
		case errors.Is(de.Err, domain.ErrInvalidInput):
			return "INVALID_INPUT"
		case errors.Is(de.Err, domain.ErrInvoiceNotFound):
			return "INVOICE_NOT_FOUND"
		case errors.Is(de.Err, domain.ErrAlreadyExists):
			return "INVOICE_ALREADY_EXISTS"
		case errors.Is(de.Err, domain.ErrFXProviderUnavailable):
			return "FX_PROVIDER_UNAVAILABLE"
		case errors.Is(de.Err, domain.ErrFXRateNotFound):
			return "FX_RATE_NOT_FOUND"
		case errors.Is(de.Err, domain.ErrUnauthorized):
			return "UNAUTHORIZED"
		case errors.Is(de.Err, domain.ErrForbidden):
			return "FORBIDDEN"
		default:
			return "INTERNAL_SERVER_ERROR"
		}
	}

	return "INTERNAL_SERVER_ERROR"
}

// invoiceToResponse converts a domain invoice to an API response.
func invoiceToResponse(inv *domain.Invoice) InvoiceResponse {
	lines := make([]InvoiceLineResponse, len(inv.LineItems))
	for i, line := range inv.LineItems {
		lines[i] = InvoiceLineResponse{
			ID:          line.ID,
			Description: line.Description,
			Amount:      line.Amount,
		}
	}

	return InvoiceResponse{
		ID:             string(inv.ID),
		TenantID:       string(inv.TenantID),
		Currency:       string(inv.Currency),
		BaseCurrency:   string(inv.BaseCurrency),
		FXRate:         float64(inv.FXRate),
		TotalAmount:    inv.TotalAmount,
		LineItems:      lines,
		IdempotencyKey: string(inv.IdempotencyKey),
		CreatedAt:      inv.CreatedAt,
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Code:    code,
		Message: message,
	})
}
