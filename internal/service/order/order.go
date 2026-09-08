// Package order реализует сценарии оформления и ведения заказа
package order

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/service/catalog"
)

// Repository доступ к заказам
type Repository interface {
	Create(ctx context.Context, cart domain.Cart, input domain.CreateOrder) (domain.Order, error)
	FindByIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) (*domain.Order, error)
	Get(ctx context.Context, id uuid.UUID) (domain.Order, error)
	List(ctx context.Context, filter domain.OrderFilter) ([]domain.Order, int, error)
	ChangeStatus(ctx context.Context, id uuid.UUID, change domain.StatusChange) (domain.Order, error)
}

// CartRepository доступ к корзине, из которой оформляется заказ
type CartRepository interface {
	Get(ctx context.Context, userID uuid.UUID) (domain.Cart, error)
}

// Service сценарии заказа
type Service struct {
	orders Repository
	carts  CartRepository
}

// New создаёт сервис заказов
func New(orders Repository, carts CartRepository) *Service {
	return &Service{orders: orders, carts: carts}
}

// List страница заказов
type List struct {
	Items []domain.Order
	Page  catalog.Page
	Total int
}

// Create оформляет заказ из корзины пользователя
func (s *Service) Create(ctx context.Context, input domain.CreateOrder) (domain.Order, error) {
	if err := input.Validate(); err != nil {
		return domain.Order{}, err
	}

	if input.IdempotencyKey != "" {
		existing, err := s.orders.FindByIdempotencyKey(ctx, input.UserID, input.IdempotencyKey)
		if err != nil {
			return domain.Order{}, err
		}
		if existing != nil {
			return *existing, nil
		}
	}

	cart, err := s.carts.Get(ctx, input.UserID)
	if err != nil {
		return domain.Order{}, err
	}
	if cart.IsEmpty() {
		return domain.Order{}, domain.ErrCartEmpty
	}
	return s.orders.Create(ctx, cart, input)
}

// ListForUser возвращает заказы пользователя
func (s *Service) ListForUser(ctx context.Context, userID uuid.UUID, page catalog.Page) (List, error) {
	page = page.Normalize()

	orders, total, err := s.orders.List(ctx, domain.OrderFilter{
		UserID: &userID,
		Limit:  page.Size,
		Offset: page.Offset(),
	})
	if err != nil {
		return List{}, err
	}
	return List{Items: orders, Page: page, Total: total}, nil
}

// GetForUser возвращает заказ пользователя вместе с историей статусов
func (s *Service) GetForUser(ctx context.Context, userID, orderID uuid.UUID) (domain.Order, error) {
	order, err := s.orders.Get(ctx, orderID)
	if err != nil {
		return domain.Order{}, err
	}
	if order.UserID != userID {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	return order, nil
}

// Cancel отменяет заказ по инициативе пользователя
func (s *Service) Cancel(ctx context.Context, userID, orderID uuid.UUID) (domain.Order, error) {
	if _, err := s.GetForUser(ctx, userID, orderID); err != nil {
		return domain.Order{}, err
	}
	return s.orders.ChangeStatus(ctx, orderID, domain.StatusChange{
		To:     domain.OrderCancelled,
		Actor:  domain.ActorUser,
		Reason: "Отменён пользователем",
	})
}

// PartnerFilter параметры выборки заказов заведением
type PartnerFilter struct {
	Status *domain.OrderStatus
	Since  *time.Time
	Page   catalog.Page
}

// ListForRestaurant возвращает заказы заведения
func (s *Service) ListForRestaurant(ctx context.Context, restaurantID uuid.UUID, filter PartnerFilter) (List, error) {
	if filter.Status != nil && !filter.Status.Valid() {
		return List{}, domain.Invalid("Неизвестный статус заказа", domain.Detail{
			Code: domain.CodeValidation, Message: "Недопустимое значение", Field: "status",
		})
	}
	page := filter.Page.Normalize()

	orders, total, err := s.orders.List(ctx, domain.OrderFilter{
		RestaurantID: &restaurantID,
		Status:       filter.Status,
		Since:        filter.Since,
		Limit:        page.Size,
		Offset:       page.Offset(),
	})
	if err != nil {
		return List{}, err
	}
	return List{Items: orders, Page: page, Total: total}, nil
}

// GetForRestaurant возвращает заказ, адресованный этому заведению
func (s *Service) GetForRestaurant(ctx context.Context, restaurantID, orderID uuid.UUID) (domain.Order, error) {
	order, err := s.orders.Get(ctx, orderID)
	if err != nil {
		return domain.Order{}, err
	}
	if order.RestaurantID != restaurantID {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	return order, nil
}

// Accept принимает заказ заведением
func (s *Service) Accept(ctx context.Context, restaurantID, orderID uuid.UUID, expectedAt *time.Time) (domain.Order, error) {
	return s.changeByRestaurant(ctx, restaurantID, orderID, domain.StatusChange{
		To:         domain.OrderAccepted,
		Actor:      domain.ActorRestaurant,
		ExpectedAt: expectedAt,
	})
}

// Reject отклоняет заказ с указанием причины
func (s *Service) Reject(ctx context.Context, restaurantID, orderID uuid.UUID, reason string) (domain.Order, error) {
	if reason == "" {
		return domain.Order{}, domain.Invalid("Не указана причина отказа", domain.Detail{
			Code: domain.CodeValidation, Message: "Причина обязательна", Field: "reason",
		})
	}
	return s.changeByRestaurant(ctx, restaurantID, orderID, domain.StatusChange{
		To:     domain.OrderRejected,
		Actor:  domain.ActorRestaurant,
		Reason: reason,
	})
}

// UpdateStatus продвигает заказ по цепочке готовки и доставки
func (s *Service) UpdateStatus(ctx context.Context, restaurantID, orderID uuid.UUID, status domain.OrderStatus) (domain.Order, error) {
	return s.changeByRestaurant(ctx, restaurantID, orderID, domain.StatusChange{
		To:    status,
		Actor: domain.ActorRestaurant,
	})
}

// changeByRestaurant проверяет принадлежность заказа заведению и меняет статус
func (s *Service) changeByRestaurant(ctx context.Context, restaurantID, orderID uuid.UUID,
	change domain.StatusChange,
) (domain.Order, error) {
	if _, err := s.GetForRestaurant(ctx, restaurantID, orderID); err != nil {
		return domain.Order{}, err
	}
	return s.orders.ChangeStatus(ctx, orderID, change)
}
