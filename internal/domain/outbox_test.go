package domain_test

import (
	"testing"
	"time"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
)

func TestBackoffGrows(t *testing.T) {
	t.Parallel()

	previous := time.Duration(0)
	for attempt := 1; attempt <= 5; attempt++ {
		current := domain.Backoff(attempt)
		if current <= previous {
			t.Errorf("задержка попытки %d = %v, должна быть больше предыдущей %v", attempt, current, previous)
		}
		previous = current
	}

	// За пределами расписания задержка не растёт бесконечно
	last := domain.Backoff(5)
	for _, attempt := range []int{6, 8, 100} {
		if got := domain.Backoff(attempt); got != last {
			t.Errorf("Backoff(%d) = %v, want %v", attempt, got, last)
		}
	}

	// Некорректный номер попытки трактуется как первая
	if domain.Backoff(0) != domain.Backoff(1) {
		t.Error("нулевая попытка должна давать ту же задержку, что и первая")
	}
}

func TestNextAttemptReschedules(t *testing.T) {
	t.Parallel()

	now := time.Now()
	event := domain.OutboxEvent{Attempts: 2}

	status, attempts, nextAt, cause := event.NextAttempt(now, "connection refused")

	if status != domain.OutboxPending {
		t.Errorf("статус = %q, want pending", status)
	}
	if attempts != 3 {
		t.Errorf("попыток = %d, want 3", attempts)
	}
	if want := now.Add(domain.Backoff(3)); !nextAt.Equal(want) {
		t.Errorf("следующая попытка в %v, want %v", nextAt, want)
	}
	if cause != "connection refused" {
		t.Errorf("причина = %q", cause)
	}
}

func TestNextAttemptExhausts(t *testing.T) {
	t.Parallel()

	now := time.Now()
	event := domain.OutboxEvent{Attempts: domain.MaxDeliveryAttempts - 1}

	status, attempts, _, _ := event.NextAttempt(now, "timeout")

	if status != domain.OutboxFailed {
		t.Errorf("статус после исчерпания попыток = %q, want failed", status)
	}
	if attempts != domain.MaxDeliveryAttempts {
		t.Errorf("попыток = %d, want %d", attempts, domain.MaxDeliveryAttempts)
	}
}

func TestExhausted(t *testing.T) {
	t.Parallel()

	if (domain.OutboxEvent{Attempts: domain.MaxDeliveryAttempts - 1}).Exhausted() {
		t.Error("до предела попытки не исчерпаны")
	}
	if !(domain.OutboxEvent{Attempts: domain.MaxDeliveryAttempts}).Exhausted() {
		t.Error("на пределе попытки исчерпаны")
	}
}
