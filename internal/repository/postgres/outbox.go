package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
)

// OutboxRepo доступ к таблице outbox_events
type OutboxRepo struct {
	pool *pgxpool.Pool
}

// NewOutboxRepo создаёт репозиторий outbox
func NewOutboxRepo(pool *pgxpool.Pool) *OutboxRepo {
	return &OutboxRepo{pool: pool}
}

// Claim забирает партию событий, готовых к отправке
func (r *OutboxRepo) Claim(ctx context.Context, limit int, lease time.Duration) ([]domain.OutboxEvent, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin claim tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx,
		`SELECT e.id, e.restaurant_id, e.order_id, e.event_type, e.payload,
			e.status, e.attempts, e.next_attempt_at, e.created_at,
			r.webhook_url, r.webhook_secret
		 FROM outbox_events e
		 JOIN restaurants r ON r.id = e.restaurant_id
		 WHERE e.status = $1 AND e.next_attempt_at <= now() AND r.webhook_url IS NOT NULL
		 ORDER BY e.next_attempt_at
		 LIMIT $2
		 FOR UPDATE OF e SKIP LOCKED`,
		domain.OutboxPending, limit)
	if err != nil {
		return nil, fmt.Errorf("claim outbox events: %w", err)
	}
	defer rows.Close()

	var events []domain.OutboxEvent
	for rows.Next() {
		var event domain.OutboxEvent
		if err := rows.Scan(&event.ID, &event.RestaurantID, &event.OrderID, &event.Type,
			&event.Payload, &event.Status, &event.Attempts, &event.NextAttemptAt,
			&event.CreatedAt, &event.WebhookURL, &event.WebhookSecret); err != nil {
			return nil, fmt.Errorf("scan outbox event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outbox events: %w", err)
	}
	if len(events) == 0 {
		return nil, nil
	}

	ids := make([]uuid.UUID, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	if _, err := tx.Exec(ctx,
		"UPDATE outbox_events SET next_attempt_at = now() + $1 WHERE id = ANY($2)",
		lease, ids); err != nil {
		return nil, fmt.Errorf("lease outbox events: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit claim tx: %w", err)
	}
	return events, nil
}

// MarkDelivered отмечает успешную доставку события
func (r *OutboxRepo) MarkDelivered(ctx context.Context, id uuid.UUID, attempts int) error {
	if _, err := r.pool.Exec(ctx,
		`UPDATE outbox_events
		 SET status = $1, attempts = $2, delivered_at = now(), last_error = NULL
		 WHERE id = $3`,
		domain.OutboxDelivered, attempts, id); err != nil {
		return fmt.Errorf("mark outbox delivered: %w", err)
	}
	return nil
}

// Reschedule сохраняет результат неудачной попытки
func (r *OutboxRepo) Reschedule(ctx context.Context, id uuid.UUID, status domain.OutboxStatus,
	attempts int, nextAttemptAt time.Time, lastError string,
) error {
	if _, err := r.pool.Exec(ctx,
		`UPDATE outbox_events
		 SET status = $1, attempts = $2, next_attempt_at = $3, last_error = $4
		 WHERE id = $5`,
		status, attempts, nextAttemptAt, lastError, id); err != nil {
		return fmt.Errorf("reschedule outbox event: %w", err)
	}
	return nil
}

// enqueueEvent записывает событие в outbox внутри переданной транзакции
func enqueueEvent(ctx context.Context, tx pgx.Tx, eventType domain.WebhookEventType, order domain.Order) error {
	var hasWebhook bool
	if err := tx.QueryRow(ctx,
		"SELECT webhook_url IS NOT NULL FROM restaurants WHERE id = $1",
		order.RestaurantID).Scan(&hasWebhook); err != nil {
		return fmt.Errorf("check webhook url: %w", err)
	}
	if !hasWebhook {
		return nil
	}

	payload, err := json.Marshal(domain.NewWebhookEvent(eventType, order))
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO outbox_events (restaurant_id, order_id, event_type, payload)
		 VALUES ($1, $2, $3, $4)`,
		order.RestaurantID, order.ID, eventType, payload); err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}
