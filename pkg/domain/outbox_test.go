package domain

import (
	"testing"
	"time"
)

func TestOutboxEvent_Creation(t *testing.T) {
	t.Run("creates outbox event with valid fields", func(t *testing.T) {
		now := time.Now().UTC()
		event := &OutboxEvent{
			ID:          OutboxEventID("evt-001"),
			AggregateID: AggregateID("inv-001"),
			EventType:   EventType("invoice.issued"),
			Payload:     []byte(`{"invoice_id":"inv-001"}`),
			Status:      EventStatus("PENDING"),
			CreatedAt:   now,
		}

		if event.ID != "evt-001" {
			t.Errorf("expected ID 'evt-001', got %q", event.ID)
		}
		if event.AggregateID != "inv-001" {
			t.Errorf("expected AggregateID 'inv-001', got %q", event.AggregateID)
		}
		if event.EventType != "invoice.issued" {
			t.Errorf("expected EventType 'invoice.issued', got %q", event.EventType)
		}
		if event.Status != "PENDING" {
			t.Errorf("expected Status 'PENDING', got %q", event.Status)
		}
	})

	t.Run("outbox event without published timestamp", func(t *testing.T) {
		event := &OutboxEvent{
			ID:          OutboxEventID("evt-002"),
			AggregateID: AggregateID("inv-002"),
			EventType:   EventType("invoice.issued"),
			Payload:     []byte(`{}`),
			Status:      EventStatus("PENDING"),
			CreatedAt:   time.Now().UTC(),
		}

		if event.PublishedAt != nil {
			t.Errorf("expected PublishedAt to be nil for new events")
		}
	})

	t.Run("outbox event with published timestamp", func(t *testing.T) {
		now := time.Now().UTC()
		publishedAt := now.Add(1 * time.Second)
		event := &OutboxEvent{
			ID:          OutboxEventID("evt-003"),
			AggregateID: AggregateID("inv-003"),
			EventType:   EventType("invoice.issued"),
			Payload:     []byte(`{"invoice_id":"inv-003"}`),
			Status:      EventStatus("PUBLISHED"),
			CreatedAt:   now,
			PublishedAt: &publishedAt,
		}

		if event.Status != "PUBLISHED" {
			t.Errorf("expected Status 'PUBLISHED', got %q", event.Status)
		}
		if event.PublishedAt == nil {
			t.Errorf("expected PublishedAt to be set for published events")
		}
	})
}

func TestOutboxEvent_StatusValues(t *testing.T) {
	t.Run("status constants", func(t *testing.T) {
		statuses := []struct {
			name string
			s    EventStatus
		}{
			{"PENDING", EventStatus("PENDING")},
			{"PUBLISHED", EventStatus("PUBLISHED")},
			{"FAILED", EventStatus("FAILED")},
		}

		for _, s := range statuses {
			t.Run(s.name, func(t *testing.T) {
				if s.s != EventStatus(s.name) {
					t.Errorf("expected event status %q", s.name)
				}
			})
		}
	})
}
