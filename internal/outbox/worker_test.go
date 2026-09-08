package outbox_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/outbox"
)

type repoStub struct {
	mu sync.Mutex

	events    []domain.OutboxEvent
	claimed   bool
	lease     time.Duration
	delivered []uuid.UUID
	attempts  int

	rescheduled  bool
	status       domain.OutboxStatus
	nextAttempts int
	lastError    string
}

func (r *repoStub) Claim(_ context.Context, _ int, lease time.Duration) ([]domain.OutboxEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.claimed {
		return nil, nil
	}
	r.claimed = true
	r.lease = lease
	return r.events, nil
}

func (r *repoStub) MarkDelivered(_ context.Context, id uuid.UUID, attempts int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.delivered = append(r.delivered, id)
	r.attempts = attempts
	return nil
}

func (r *repoStub) Reschedule(_ context.Context, _ uuid.UUID, status domain.OutboxStatus,
	attempts int, _ time.Time, lastError string,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.rescheduled = true
	r.status = status
	r.nextAttempts = attempts
	r.lastError = lastError
	return nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newEvent(url, secret string, attempts int) domain.OutboxEvent {
	return domain.OutboxEvent{
		ID:            uuid.New(),
		RestaurantID:  uuid.New(),
		OrderID:       uuid.New(),
		Type:          domain.WebhookOrderCreated,
		Payload:       []byte(`{"event_id":"test","type":"order.created"}`),
		Status:        domain.OutboxPending,
		Attempts:      attempts,
		WebhookURL:    url,
		WebhookSecret: secret,
	}
}

func TestDeliverSignsPayload(t *testing.T) {
	t.Parallel()

	var (
		gotSignature string
		gotBody      []byte
		gotType      string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSignature = r.Header.Get(domain.SignatureHeader)
		gotType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	event := newEvent(server.URL, "demo-webhook-secret", 0)
	repo := &repoStub{events: []domain.OutboxEvent{event}}
	worker := outbox.New(repo, discardLogger(), outbox.Config{})

	if err := worker.ProcessBatch(context.Background()); err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}

	if string(gotBody) != string(event.Payload) {
		t.Errorf("тело запроса = %q, want %q", gotBody, event.Payload)
	}
	if gotType != "application/json" {
		t.Errorf("Content-Type = %q", gotType)
	}
	if !domain.VerifyWebhook("demo-webhook-secret", gotBody, gotSignature) {
		t.Error("заведение должно суметь проверить подпись своим секретом")
	}
	if len(repo.delivered) != 1 || repo.delivered[0] != event.ID {
		t.Errorf("доставленных событий %d, want 1", len(repo.delivered))
	}
	if repo.attempts != 1 {
		t.Errorf("зафиксировано попыток %d, want 1", repo.attempts)
	}
	if repo.rescheduled {
		t.Error("успешная доставка не должна переносить событие")
	}
}

func TestDeliverRetriesOnServerError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	repo := &repoStub{events: []domain.OutboxEvent{newEvent(server.URL, "secret", 0)}}
	worker := outbox.New(repo, discardLogger(), outbox.Config{})

	if err := worker.ProcessBatch(context.Background()); err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}

	if len(repo.delivered) != 0 {
		t.Error("ошибка заведения не должна считаться доставкой")
	}
	if !repo.rescheduled {
		t.Fatal("после ошибки событие должно быть перенесено")
	}
	if repo.status != domain.OutboxPending {
		t.Errorf("статус = %q, want pending", repo.status)
	}
	if repo.nextAttempts != 1 {
		t.Errorf("попыток = %d, want 1", repo.nextAttempts)
	}
	if repo.lastError == "" {
		t.Error("причина отказа должна сохраняться для разбора")
	}
}

func TestDeliverGivesUpAfterMaxAttempts(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	repo := &repoStub{events: []domain.OutboxEvent{
		newEvent(server.URL, "secret", domain.MaxDeliveryAttempts-1),
	}}
	worker := outbox.New(repo, discardLogger(), outbox.Config{})

	if err := worker.ProcessBatch(context.Background()); err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}

	if repo.status != domain.OutboxFailed {
		t.Errorf("статус = %q, want failed", repo.status)
	}
	if repo.nextAttempts != domain.MaxDeliveryAttempts {
		t.Errorf("попыток = %d, want %d", repo.nextAttempts, domain.MaxDeliveryAttempts)
	}
}

func TestDeliverHandlesUnreachableRestaurant(t *testing.T) {
	t.Parallel()

	// Сервер поднят и сразу закрыт: адрес есть, соединения нет
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close()

	repo := &repoStub{events: []domain.OutboxEvent{newEvent(url, "secret", 0)}}
	worker := outbox.New(repo, discardLogger(), outbox.Config{RequestTimeout: time.Second})

	if err := worker.ProcessBatch(context.Background()); err != nil {
		t.Fatalf("ProcessBatch не должен падать из-за недоступного заведения: %v", err)
	}
	if !repo.rescheduled || repo.status != domain.OutboxPending {
		t.Error("недоступное заведение должно приводить к повтору, а не к потере события")
	}
}

func TestClaimLeaseExceedsRequestTimeout(t *testing.T) {
	t.Parallel()

	repo := &repoStub{}
	timeout := 2 * time.Second
	worker := outbox.New(repo, discardLogger(), outbox.Config{RequestTimeout: timeout})

	if err := worker.ProcessBatch(context.Background()); err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}
	if repo.lease <= timeout {
		t.Errorf("аренда события %v должна превышать таймаут запроса %v, иначе соседний воркер заберёт его повторно",
			repo.lease, timeout)
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	t.Parallel()

	repo := &repoStub{}
	worker := outbox.New(repo, discardLogger(), outbox.Config{PollInterval: 10 * time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(ctx)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("воркер должен останавливаться по отмене контекста")
	}
}
