package domain

import (
	"time"

	"github.com/google/uuid"
)

// OutboxStatus состояние доставки события
type OutboxStatus string

// Состояния события в outbox
const (
	// OutboxPending событие ждёт отправки или следующей попытки
	OutboxPending OutboxStatus = "pending"
	// OutboxDelivered заведение подтвердило приём
	OutboxDelivered OutboxStatus = "delivered"
	// OutboxFailed попытки исчерпаны, событие остаётся для разбора
	OutboxFailed OutboxStatus = "failed"
)

// MaxDeliveryAttempts число попыток доставки, после которого событие помечается failed
const MaxDeliveryAttempts = 8

// backoffSchedule задержки перед повторными попытками
var backoffSchedule = []time.Duration{
	1 * time.Second,
	5 * time.Second,
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
}

// OutboxEvent событие для доставки заведению
type OutboxEvent struct {
	ID            uuid.UUID
	RestaurantID  uuid.UUID
	OrderID       uuid.UUID
	Type          WebhookEventType
	Payload       []byte
	Status        OutboxStatus
	Attempts      int
	NextAttemptAt time.Time
	LastError     string
	DeliveredAt   *time.Time
	CreatedAt     time.Time
	WebhookURL    string
	WebhookSecret string
}

// Backoff возвращает задержку перед попыткой с номером attempt
func Backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > len(backoffSchedule) {
		return backoffSchedule[len(backoffSchedule)-1]
	}
	return backoffSchedule[attempt-1]
}

// Exhausted сообщает, что попытки исчерпаны и повторять доставку не нужно
func (e OutboxEvent) Exhausted() bool {
	return e.Attempts >= MaxDeliveryAttempts
}

// NextAttempt возвращает состояние события после неудачной попытки
func (e OutboxEvent) NextAttempt(now time.Time, cause string) (status OutboxStatus, attempts int, nextAt time.Time, lastError string) {
	attempts = e.Attempts + 1
	if attempts >= MaxDeliveryAttempts {
		return OutboxFailed, attempts, now, cause
	}
	return OutboxPending, attempts, now.Add(Backoff(attempts)), cause
}
