package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/agent-orchestration/ledgerly/pkg/domain"
)

// ---------------------------------------------------------------------------
// DTOs
// ---------------------------------------------------------------------------

// LineItemInput represents a single line item in the create invoice request.
type LineItemInput struct {
	Description string          `json:"description"`
	Amount      decimal.Decimal `json:"amount"`
}

// CreateInvoiceRequest is the input for creating a new invoice.
type CreateInvoiceRequest struct {
	TenantID       domain.TenantID       `json:"tenant_id"`
	Currency       domain.Currency       `json:"currency"`
	BaseCurrency   domain.Currency       `json:"base_currency"`
	IdempotencyKey domain.IdempotencyKey `json:"idempotency_key"`
	LineItems      []LineItemInput       `json:"line_items"`
	CreatedBy      string                `json:"created_by"`
}

// invoiceIssuedEventPayload is the JSON payload for invoice.issued event.
type invoiceIssuedEventPayload struct {
	InvoiceID string            `json:"invoice_id"`
	TenantID  domain.TenantID   `json:"tenant_id"`
	Currency  domain.Currency   `json:"currency"`
	Total     decimal.Decimal   `json:"total_amount"`
}

// ---------------------------------------------------------------------------
// InvoiceService
// ---------------------------------------------------------------------------

// InvoiceService orchestrates the full invoice issuance use case.
// It depends on abstract interfaces (Clean Architecture, RULE-ARCH-02).
type InvoiceService struct {
	invoiceRepo domain.InvoiceRepository
	ledgerRepo  domain.LedgerRepository
	outboxRepo  domain.OutboxRepository
	fxProvider  domain.FXProvider
}

// NewInvoiceService creates a new InvoiceService with its required dependencies.
func NewInvoiceService(
	invoiceRepo domain.InvoiceRepository,
	ledgerRepo domain.LedgerRepository,
	outboxRepo domain.OutboxRepository,
	fxProvider domain.FXProvider,
) *InvoiceService {
	return &InvoiceService{
		invoiceRepo: invoiceRepo,
		ledgerRepo:  ledgerRepo,
		outboxRepo:  outboxRepo,
		fxProvider:  fxProvider,
	}
}

// CreateInvoice handles the full lifecycle of invoice creation:
// 1. Validates input (FR-INV-01)
// 2. Checks idempotency (FR-INV-02, RULE-DATA-02)
// 3. Fetches FX rate (FR-FX-01)
// 4. Constructs invoice and ledger entities
// 5. Persists atomically (FR-LED-01, RULE-DATA-01)
// 6. Saves event to outbox (FR-EVT-01, RULE-EVT-02)
//
// Context is propagated to all downstream calls (RULE-RES-01).
// Domain errors are returned for all failure cases (RULE-ARCH-03).
func (s *InvoiceService) CreateInvoice(ctx context.Context, req CreateInvoiceRequest) (*domain.Invoice, error) {
	// Step 1: Validate input
	if err := validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Step 2: Check idempotency - RULE-DATA-02
	existing, err := s.invoiceRepo.GetInvoiceByIdempotencyKey(ctx, req.TenantID, req.IdempotencyKey)
	if err == nil && existing != nil {
		// Idempotent: return the existing invoice (FR-INV-02, AC-01)
		return existing, nil
	}
	if err != nil && !domain.IsDomainError(err, domain.ErrInvoiceNotFound) {
		// An unexpected error (not "not found") should propagate
		return nil, fmt.Errorf("failed to check idempotency: %w", err)
	}

	// Step 3: Fetch FX rate (FR-FX-01)
	fxRate, err := s.fxProvider.GetRate(ctx, req.Currency, req.BaseCurrency)
	if err != nil {
		// Propagate typed domain errors (FR-FX-02, RULE-ARCH-03)
		return nil, fmt.Errorf("failed to fetch FX rate: %w", err)
	}

	// Step 4: Build domain entities
	now := time.Now().UTC()
	invoiceID := domain.InvoiceID(uuid.New().String())

	// Calculate total
	total := decimal.Zero
	for _, item := range req.LineItems {
		total = total.Add(item.Amount)
	}

	// Build invoice lines
	lines := make([]domain.InvoiceLine, 0, len(req.LineItems))
	for _, item := range req.LineItems {
		lines = append(lines, domain.InvoiceLine{
			ID:          uuid.New().String(),
			InvoiceID:   invoiceID,
			Description: item.Description,
			Amount:      item.Amount,
			CreatedAt:   now,
			CreatedBy:   req.CreatedBy,
		})
	}

	inv := &domain.Invoice{
		ID:             invoiceID,
		TenantID:       req.TenantID,
		Currency:       req.Currency,
		BaseCurrency:   req.BaseCurrency,
		FXRate:         fxRate, // Immutable snapshot (FR-FX-01, AC-03)
		TotalAmount:    total,
		LineItems:      lines,
		IdempotencyKey: req.IdempotencyKey,
		CreatedAt:      now,
		CreatedBy:      req.CreatedBy,
	}

	// Build double-entry ledger entries (FR-LED-01)
	// Debit AccountsReceivable, Credit Revenue
	ledgerEntries := []domain.LedgerEntry{
		{
			ID:        domain.LedgerEntryID(uuid.New().String()),
			InvoiceID: invoiceID,
			TenantID:  req.TenantID,
			Account:   "AccountsReceivable",
			Debit:     total,
			Credit:    decimal.Zero,
			CreatedAt: now,
		},
		{
			ID:        domain.LedgerEntryID(uuid.New().String()),
			InvoiceID: invoiceID,
			TenantID:  req.TenantID,
			Account:   "Revenue",
			Debit:     decimal.Zero,
			Credit:    total,
			CreatedAt: now,
		},
	}

	// Step 5: Atomic persistence (FR-LED-01, RULE-DATA-01)
	// The repository is responsible for transactional atomicity
	if err := s.invoiceRepo.CreateInvoice(ctx, inv, ledgerEntries); err != nil {
		return nil, fmt.Errorf("failed to persist invoice and ledger: %w", err)
	}

	// Step 6: Save event to transactional outbox (FR-EVT-01, RULE-EVT-02)
	// This happens after the transaction has committed
	payload, err := json.Marshal(invoiceIssuedEventPayload{
		InvoiceID: string(invoiceID),
		TenantID:  req.TenantID,
		Currency:  req.Currency,
		Total:     total,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal outbox event payload: %w", err)
	}

	outboxEvent := &domain.OutboxEvent{
		ID:          domain.OutboxEventID(uuid.New().String()),
		AggregateID: domain.AggregateID(invoiceID),
		EventType:   "invoice.issued",
		Payload:     payload,
		Status:      "PENDING",
		CreatedAt:   now,
	}

	if err := s.outboxRepo.SaveEvent(ctx, outboxEvent); err != nil {
		return nil, fmt.Errorf("failed to save outbox event: %w", err)
	}

	return inv, nil
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

func validateCreateRequest(req CreateInvoiceRequest) error {
	if req.TenantID == "" {
		return domain.NewDomainError(domain.ErrInvalidInput, "tenant_id is required")
	}
	if req.Currency == "" {
		return domain.NewDomainError(domain.ErrInvalidInput, "currency is required")
	}
	if req.BaseCurrency == "" {
		return domain.NewDomainError(domain.ErrInvalidInput, "base_currency is required")
	}
	if req.IdempotencyKey == "" {
		return domain.NewDomainError(domain.ErrInvalidInput, "idempotency_key is required")
	}
	if len(req.LineItems) == 0 {
		return domain.NewDomainError(domain.ErrInvalidInput, "at least one line_item is required")
	}
	for i, item := range req.LineItems {
		if item.Description == "" {
			return domain.NewDomainError(domain.ErrInvalidInput, fmt.Sprintf("line_items[%d].description is required", i))
		}
		if item.Amount.IsNegative() || item.Amount.IsZero() {
			return domain.NewDomainError(domain.ErrInvalidInput, fmt.Sprintf("line_items[%d].amount must be positive", i))
		}
	}
	return nil
}
