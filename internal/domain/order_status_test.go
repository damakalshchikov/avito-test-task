package domain_test

import (
	"testing"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
)

func TestCanTransitionHappyPath(t *testing.T) {
	t.Parallel()

	chain := []struct {
		from  domain.OrderStatus
		to    domain.OrderStatus
		actor domain.Actor
	}{
		{domain.OrderNew, domain.OrderAccepted, domain.ActorRestaurant},
		{domain.OrderAccepted, domain.OrderCooking, domain.ActorRestaurant},
		{domain.OrderCooking, domain.OrderReady, domain.ActorRestaurant},
		{domain.OrderReady, domain.OrderDelivering, domain.ActorRestaurant},
		{domain.OrderDelivering, domain.OrderDelivered, domain.ActorRestaurant},
	}

	for _, step := range chain {
		if err := domain.CanTransition(step.from, step.to, step.actor); err != nil {
			t.Errorf("переход %s -> %s должен быть разрешён: %v", step.from, step.to, err)
		}
	}
}

func TestCanTransitionRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		from  domain.OrderStatus
		to    domain.OrderStatus
		actor domain.Actor
		kind  domain.Kind
	}{
		{
			name: "пропуск шага в цепочке", from: domain.OrderAccepted, to: domain.OrderReady,
			actor: domain.ActorRestaurant, kind: domain.KindConflict,
		},
		{
			name: "движение назад", from: domain.OrderReady, to: domain.OrderCooking,
			actor: domain.ActorRestaurant, kind: domain.KindConflict,
		},
		{
			name: "отмена после начала готовки", from: domain.OrderCooking, to: domain.OrderCancelled,
			actor: domain.ActorUser, kind: domain.KindConflict,
		},
		{
			name: "пользователь принимает заказ за заведение", from: domain.OrderNew, to: domain.OrderAccepted,
			actor: domain.ActorUser, kind: domain.KindConflict,
		},
		{
			name: "заведение отменяет вместо отклонения", from: domain.OrderNew, to: domain.OrderCancelled,
			actor: domain.ActorRestaurant, kind: domain.KindConflict,
		},
		{
			name: "переход из доставленного", from: domain.OrderDelivered, to: domain.OrderCancelled,
			actor: domain.ActorUser, kind: domain.KindConflict,
		},
		{
			name: "переход из отменённого", from: domain.OrderCancelled, to: domain.OrderAccepted,
			actor: domain.ActorRestaurant, kind: domain.KindConflict,
		},
		{
			name: "переход в тот же статус", from: domain.OrderCooking, to: domain.OrderCooking,
			actor: domain.ActorRestaurant, kind: domain.KindConflict,
		},
		{
			name: "неизвестный статус", from: domain.OrderNew, to: domain.OrderStatus("frozen"),
			actor: domain.ActorRestaurant, kind: domain.KindInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := domain.CanTransition(tt.from, tt.to, tt.actor)
			if err == nil {
				t.Fatalf("переход %s -> %s (%s) должен быть запрещён", tt.from, tt.to, tt.actor)
			}
			domainErr, ok := domain.AsError(err)
			if !ok {
				t.Fatalf("ожидалась доменная ошибка, получено %T", err)
			}
			if domainErr.Kind != tt.kind {
				t.Errorf("Kind = %v, want %v", domainErr.Kind, tt.kind)
			}
		})
	}
}

func TestOrderStatusProperties(t *testing.T) {
	t.Parallel()

	final := []domain.OrderStatus{domain.OrderDelivered, domain.OrderRejected, domain.OrderCancelled}
	for _, status := range final {
		if !status.IsFinal() {
			t.Errorf("%s должен быть финальным", status)
		}
		if len(status.NextStatuses(domain.ActorRestaurant)) != 0 {
			t.Errorf("из финального %s не должно быть переходов", status)
		}
	}

	notFinal := []domain.OrderStatus{
		domain.OrderNew, domain.OrderAccepted, domain.OrderCooking,
		domain.OrderReady, domain.OrderDelivering,
	}
	for _, status := range notFinal {
		if status.IsFinal() {
			t.Errorf("%s не должен быть финальным", status)
		}
	}

	returnsStock := map[domain.OrderStatus]bool{
		domain.OrderRejected:  true,
		domain.OrderCancelled: true,
		domain.OrderDelivered: false,
		domain.OrderNew:       false,
	}
	for status, want := range returnsStock {
		if got := status.ReturnsStock(); got != want {
			t.Errorf("%s.ReturnsStock() = %v, want %v", status, got, want)
		}
	}

	canCancel := map[domain.OrderStatus]bool{
		domain.OrderNew:       true,
		domain.OrderAccepted:  true,
		domain.OrderCooking:   false,
		domain.OrderDelivered: false,
	}
	for status, want := range canCancel {
		if got := status.CanCancel(); got != want {
			t.Errorf("%s.CanCancel() = %v, want %v", status, got, want)
		}
	}
}

func TestNextStatusesByActor(t *testing.T) {
	t.Parallel()

	restaurant := domain.OrderNew.NextStatuses(domain.ActorRestaurant)
	if len(restaurant) != 2 {
		t.Errorf("заведению из new доступно %d переходов, want 2", len(restaurant))
	}

	user := domain.OrderNew.NextStatuses(domain.ActorUser)
	if len(user) != 1 || user[0] != domain.OrderCancelled {
		t.Errorf("пользователю из new доступно %v, want [cancelled]", user)
	}
}
