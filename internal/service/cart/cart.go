// Package cart реализует сценарии корзины пользователя
package cart

import (
	"context"

	"github.com/google/uuid"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
)

// Repository доступ к корзине
type Repository interface {
	Get(ctx context.Context, userID uuid.UUID) (domain.Cart, error)
	SetItem(ctx context.Context, userID, menuItemID uuid.UUID, quantity int) (domain.Cart, error)
	Clear(ctx context.Context, userID uuid.UUID) error
}

// MenuRepository доступ к позициям меню
type MenuRepository interface {
	GetItemByID(ctx context.Context, id uuid.UUID) (domain.MenuItem, error)
}

// RestaurantRepository доступ к условиям доставки заведения
type RestaurantRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (domain.Restaurant, error)
}

// Service сценарии корзины
type Service struct {
	carts       Repository
	menu        MenuRepository
	restaurants RestaurantRepository
}

// New создаёт сервис корзины
func New(carts Repository, menu MenuRepository, restaurants RestaurantRepository) *Service {
	return &Service{carts: carts, menu: menu, restaurants: restaurants}
}

// View корзина вместе с рассчитанной стоимостью доставки
type View struct {
	Cart        domain.Cart
	DeliveryFee int64
	Total       int64
}

// Get возвращает корзину пользователя
func (s *Service) Get(ctx context.Context, userID uuid.UUID) (View, error) {
	cart, err := s.carts.Get(ctx, userID)
	if err != nil {
		return View{}, err
	}
	return s.view(ctx, cart)
}

// SetItem задаёт количество позиции в корзине
func (s *Service) SetItem(ctx context.Context, userID, menuItemID uuid.UUID, quantity int) (View, error) {
	if err := domain.ValidateCartQuantity(quantity); err != nil {
		return View{}, err
	}

	if quantity > 0 {
		item, err := s.menu.GetItemByID(ctx, menuItemID)
		if err != nil {
			return View{}, err
		}
		if !item.Orderable(quantity) {
			return View{}, domain.ErrItemsUnavailable.WithDetails(
				domain.UnavailableDetails([]domain.CartItem{{MenuItem: item, Quantity: quantity}})...)
		}
	}

	cart, err := s.carts.SetItem(ctx, userID, menuItemID, quantity)
	if err != nil {
		return View{}, err
	}
	return s.view(ctx, cart)
}

// Clear удаляет корзину пользователя
func (s *Service) Clear(ctx context.Context, userID uuid.UUID) error {
	return s.carts.Clear(ctx, userID)
}

// view дополняет корзину условиями доставки заведения
func (s *Service) view(ctx context.Context, cart domain.Cart) (View, error) {
	if cart.IsEmpty() {
		return View{Cart: cart}, nil
	}

	restaurant, err := s.restaurants.GetByID(ctx, cart.RestaurantID)
	if err != nil {
		return View{}, err
	}
	subtotal := cart.Subtotal()
	return View{
		Cart:        cart,
		DeliveryFee: restaurant.DeliveryFee,
		Total:       subtotal + restaurant.DeliveryFee,
	}, nil
}
