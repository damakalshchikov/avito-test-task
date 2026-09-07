package cart_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/service/cart"
)

type cartRepoStub struct {
	cart     domain.Cart
	setCalls int
}

func (s *cartRepoStub) Get(_ context.Context, _ uuid.UUID) (domain.Cart, error) {
	return s.cart, nil
}

func (s *cartRepoStub) SetItem(_ context.Context, _, _ uuid.UUID, _ int) (domain.Cart, error) {
	s.setCalls++
	return s.cart, nil
}

func (s *cartRepoStub) Clear(_ context.Context, _ uuid.UUID) error { return nil }

type menuRepoStub struct {
	item domain.MenuItem
}

func (s *menuRepoStub) GetItemByID(_ context.Context, _ uuid.UUID) (domain.MenuItem, error) {
	return s.item, nil
}

type restaurantRepoStub struct {
	restaurant domain.Restaurant
}

func (s *restaurantRepoStub) GetByID(_ context.Context, _ uuid.UUID) (domain.Restaurant, error) {
	return s.restaurant, nil
}

func stock(n int) *int { return &n }

func TestGetEmptyCartHasNoDelivery(t *testing.T) {
	t.Parallel()

	service := cart.New(&cartRepoStub{}, &menuRepoStub{}, &restaurantRepoStub{
		restaurant: domain.Restaurant{DeliveryFee: 19900},
	})

	view, err := service.Get(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !view.Cart.IsEmpty() {
		t.Error("корзина должна быть пустой")
	}
	if view.DeliveryFee != 0 || view.Total != 0 {
		t.Errorf("для пустой корзины доставка и total = %d/%d, want 0/0", view.DeliveryFee, view.Total)
	}
}

func TestGetCartCalculatesTotal(t *testing.T) {
	t.Parallel()

	filled := domain.Cart{
		ID:           uuid.New(),
		RestaurantID: uuid.New(),
		Items: []domain.CartItem{
			{MenuItem: domain.MenuItem{Price: 39000, IsAvailable: true}, Quantity: 2},
			{MenuItem: domain.MenuItem{Price: 22000, IsAvailable: true}, Quantity: 1},
		},
	}
	service := cart.New(&cartRepoStub{cart: filled}, &menuRepoStub{}, &restaurantRepoStub{
		restaurant: domain.Restaurant{DeliveryFee: 19900},
	})

	view, err := service.Get(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got := view.Cart.Subtotal(); got != 100000 {
		t.Errorf("Subtotal = %d, want 100000", got)
	}
	if view.Total != 119900 {
		t.Errorf("Total = %d, want 119900", view.Total)
	}
}

func TestSetItemRejectsUnavailable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		item     domain.MenuItem
		quantity int
	}{
		{"снятая с продажи", domain.MenuItem{IsAvailable: false, StockQty: stock(10)}, 1},
		{"остатка не хватает", domain.MenuItem{IsAvailable: true, StockQty: stock(2)}, 3},
		{"нулевой остаток", domain.MenuItem{IsAvailable: true, StockQty: stock(0)}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			carts := &cartRepoStub{}
			service := cart.New(carts, &menuRepoStub{item: tt.item}, &restaurantRepoStub{})

			_, err := service.SetItem(context.Background(), uuid.New(), uuid.New(), tt.quantity)
			domainErr, ok := domain.AsError(err)
			if !ok {
				t.Fatalf("ожидалась доменная ошибка, получено %v", err)
			}
			if domainErr.Code != domain.CodeItemsUnavailable {
				t.Errorf("Code = %q, want %q", domainErr.Code, domain.CodeItemsUnavailable)
			}
			if len(domainErr.Details) != 1 {
				t.Fatalf("деталей %d, want 1", len(domainErr.Details))
			}
			if domainErr.Details[0].RequestedQty != tt.quantity {
				t.Errorf("RequestedQty = %d, want %d", domainErr.Details[0].RequestedQty, tt.quantity)
			}
			if carts.setCalls != 0 {
				t.Error("недоступная позиция не должна попадать в корзину")
			}
		})
	}
}

func TestSetItemValidatesQuantity(t *testing.T) {
	t.Parallel()

	carts := &cartRepoStub{}
	service := cart.New(carts, &menuRepoStub{item: domain.MenuItem{IsAvailable: true}}, &restaurantRepoStub{})

	for _, quantity := range []int{-1, 100} {
		_, err := service.SetItem(context.Background(), uuid.New(), uuid.New(), quantity)
		domainErr, ok := domain.AsError(err)
		if !ok || domainErr.Kind != domain.KindInvalid {
			t.Errorf("количество %d должно отклоняться как невалидное, получено %v", quantity, err)
		}
	}
	if carts.setCalls != 0 {
		t.Error("некорректное количество не должно доходить до репозитория")
	}
}

func TestSetItemZeroQuantitySkipsAvailabilityCheck(t *testing.T) {
	t.Parallel()

	carts := &cartRepoStub{}
	// Позиция снята с продажи, но удалить её из корзины пользователь обязан мочь
	service := cart.New(carts, &menuRepoStub{item: domain.MenuItem{IsAvailable: false}}, &restaurantRepoStub{})

	if _, err := service.SetItem(context.Background(), uuid.New(), uuid.New(), 0); err != nil {
		t.Fatalf("удаление позиции должно проходить: %v", err)
	}
	if carts.setCalls != 1 {
		t.Error("удаление должно доходить до репозитория")
	}
}
