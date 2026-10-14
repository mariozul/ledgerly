package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agent-orchestration/ledgerly/pkg/domain"
)

// PgOutboxRepository implements domain.OutboxRepository using PostgreSQL.
// Clean Architecture: Infrastructure adapter (RULE-ARCH-02, RULE-EVT-02).
type PgOutboxRepository struct {
	pool *pgxpool.Pool
}

// NewPgOutboxRepository creates a new PgOutboxRepository.
func NewPgOutboxRepository(connStr string) *PgOutboxRepository {
	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		return &PgOutboxRepository{pool: nil}
	}
	return &PgOutboxRepository{pool: pool}
}

// NewPgOutboxRepositoryWithPool creates a PgOutboxRepository with an existing pool.
func NewPgOutboxRepositoryWithPool(pool *pgxpool.Pool) *PgOutboxRepository {
	return &PgOutboxRepository{pool: pool}
}

// SaveEvent persists an outbox event to the database.
// Implements RULE-EVT-02: transactional outbox for distributed events.
// The caller is responsible for managing the database transaction;
// this method is typically called within the same transaction as the
// business state mutation.
func (r *PgOutboxRepository) SaveEvent(ctx context.Context, event *domain.OutboxEvent) error {
	if r.pool == nil {
		return fmt.Errorf("database pool not initialized")
	}

	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO outbox (id, aggregate_id, event_type, payload, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		event.ID, event.AggregateID, event.EventType,
		event.Payload, event.Status, now,
	)
	if err != nil {
		return fmt.Errorf("failed to save outbox event: %w", err)
	}

	return nil
}
