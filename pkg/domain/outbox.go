package domain

import (
	"context"
	"time"
)

// Type aliases for outbox domain primitives
type (
	OutboxEventID string
	AggregateID   string
	EventType     string
	EventStatus   string
)

// OutboxEvent represents a domain event stored in the transactional outbox.
// Pure domain entity - no framework dependencies (RULE-ARCH-01, RULE-EVT-02).
type OutboxEvent struct {
	ID          OutboxEventID
	AggregateID AggregateID // E.g., InvoiceID
	EventType   EventType   // E.g., 'invoice.issued'
	Payload     []byte      // JSON serialized event data
	Status      EventStatus // E.g., 'PENDING', 'PUBLISHED', 'FAILED'
	CreatedAt   time.Time
	PublishedAt *time.Time
}

// OutboxRepository defines the persistence contract for outbox events.
// Abstract interface - implementations reside in infrastructure layer (RULE-ARCH-02).
type OutboxRepository interface {
	SaveEvent(ctx context.Context, event *OutboxEvent) error
}
