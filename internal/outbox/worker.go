// Package outbox доставляет заведениям события заказов, накопленные в outbox
package outbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
)

// Значения по умолчанию для воркера
const (
	defaultBatchSize      = 20
	defaultRequestTimeout = 10 * time.Second
	leaseFactor           = 3
)

// Repository доступ к событиям outbox
type Repository interface {
	Claim(ctx context.Context, limit int, lease time.Duration) ([]domain.OutboxEvent, error)
	MarkDelivered(ctx context.Context, id uuid.UUID, attempts int) error
	Reschedule(ctx context.Context, id uuid.UUID, status domain.OutboxStatus,
		attempts int, nextAttemptAt time.Time, lastError string) error
}

// Config параметры воркера
type Config struct {
	// PollInterval пауза между проверками очереди
	PollInterval time.Duration
	// BatchSize сколько событий берётся за один проход
	BatchSize int
	// RequestTimeout ограничение на один запрос к заведению
	RequestTimeout time.Duration
}

// Worker фоновый отправитель webhook-ов
type Worker struct {
	repo   Repository
	client *http.Client
	logger *slog.Logger
	cfg    Config
}

// New создаёт воркер доставки
func New(repo Repository, logger *slog.Logger, cfg Config) *Worker {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultBatchSize
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = defaultRequestTimeout
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Second
	}

	return &Worker{
		repo:   repo,
		client: &http.Client{Timeout: cfg.RequestTimeout},
		logger: logger,
		cfg:    cfg,
	}
}

// Run обрабатывает очередь до отмены контекста
func (w *Worker) Run(ctx context.Context) {
	w.logger.Info("outbox worker started",
		slog.Duration("poll_interval", w.cfg.PollInterval),
		slog.Int("batch_size", w.cfg.BatchSize))

	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("outbox worker stopped")
			return
		case <-ticker.C:
			if err := w.ProcessBatch(ctx); err != nil && !errors.Is(err, context.Canceled) {
				w.logger.Error("outbox batch failed", slog.String("error", err.Error()))
			}
		}
	}
}

// ProcessBatch отправляет одну партию событий
func (w *Worker) ProcessBatch(ctx context.Context) error {
	events, err := w.repo.Claim(ctx, w.cfg.BatchSize, w.cfg.RequestTimeout*leaseFactor)
	if err != nil {
		return err
	}

	for _, event := range events {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		w.process(ctx, event)
	}
	return nil
}

// process отправляет одно событие и сохраняет результат попытки
func (w *Worker) process(ctx context.Context, event domain.OutboxEvent) {
	err := w.deliver(ctx, event)
	if err == nil {
		if err := w.repo.MarkDelivered(ctx, event.ID, event.Attempts+1); err != nil {
			w.logger.Error("mark delivered failed",
				slog.String("event_id", event.ID.String()),
				slog.String("error", err.Error()))
			return
		}
		w.logger.Info("webhook delivered",
			slog.String("event_id", event.ID.String()),
			slog.String("event_type", string(event.Type)),
			slog.String("order_id", event.OrderID.String()),
			slog.Int("attempt", event.Attempts+1))
		return
	}

	status, attempts, nextAt, cause := event.NextAttempt(time.Now(), err.Error())
	if err := w.repo.Reschedule(ctx, event.ID, status, attempts, nextAt, cause); err != nil {
		w.logger.Error("reschedule failed",
			slog.String("event_id", event.ID.String()),
			slog.String("error", err.Error()))
		return
	}

	level := slog.LevelWarn
	if status == domain.OutboxFailed {
		level = slog.LevelError
	}
	w.logger.Log(ctx, level, "webhook delivery failed",
		slog.String("event_id", event.ID.String()),
		slog.String("order_id", event.OrderID.String()),
		slog.Int("attempt", attempts),
		slog.String("status", string(status)),
		slog.String("error", cause))
}

// deliver отправляет событие заведению с подписью тела
func (w *Worker) deliver(ctx context.Context, event domain.OutboxEvent) error {
	ctx, cancel := context.WithTimeout(ctx, w.cfg.RequestTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		event.WebhookURL, bytes.NewReader(event.Payload))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(domain.SignatureHeader, domain.SignWebhook(event.WebhookSecret, event.Payload))

	response, err := w.client.Do(request)
	if err != nil {
		return fmt.Errorf("send webhook: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("webhook responded with status %d", response.StatusCode)
	}
	return nil
}
