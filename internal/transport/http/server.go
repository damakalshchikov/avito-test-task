package http

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/api"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/service/cart"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/service/catalog"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/service/order"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/service/partner"
)

// CatalogService клиентские сценарии каталога
type CatalogService interface {
	ListRestaurants(ctx context.Context, search string, page catalog.Page) (catalog.RestaurantList, error)
	GetRestaurant(ctx context.Context, id uuid.UUID) (domain.Restaurant, error)
	GetMenu(ctx context.Context, restaurantID uuid.UUID) ([]domain.MenuCategory, error)
}

// PartnerService партнёрские и административные сценарии
type PartnerService interface {
	Authenticator
	GetProfile(ctx context.Context, restaurantID uuid.UUID) (domain.Restaurant, error)
	UpdateProfile(ctx context.Context, restaurantID uuid.UUID, update domain.RestaurantUpdate) (domain.Restaurant, error)
	SyncMenu(ctx context.Context, restaurantID uuid.UUID, menu domain.MenuSync) (domain.MenuSyncResult, error)
	UpdateMenuItem(ctx context.Context, restaurantID uuid.UUID, externalID string, update domain.MenuItemUpdate) (domain.MenuItem, error)
	CreateRestaurant(ctx context.Context, restaurant domain.Restaurant) (partner.Credentials, error)
}

// CartService сценарии корзины
type CartService interface {
	Get(ctx context.Context, userID uuid.UUID) (cart.View, error)
	SetItem(ctx context.Context, userID, menuItemID uuid.UUID, quantity int) (cart.View, error)
	Clear(ctx context.Context, userID uuid.UUID) error
}

// OrderService сценарии заказа со стороны пользователя и заведения
type OrderService interface {
	Create(ctx context.Context, input domain.CreateOrder) (domain.Order, error)
	ListForUser(ctx context.Context, userID uuid.UUID, page catalog.Page) (order.List, error)
	GetForUser(ctx context.Context, userID, orderID uuid.UUID) (domain.Order, error)
	Cancel(ctx context.Context, userID, orderID uuid.UUID) (domain.Order, error)
	ListForRestaurant(ctx context.Context, restaurantID uuid.UUID, filter order.PartnerFilter) (order.List, error)
	GetForRestaurant(ctx context.Context, restaurantID, orderID uuid.UUID) (domain.Order, error)
	Accept(ctx context.Context, restaurantID, orderID uuid.UUID, expectedAt *time.Time) (domain.Order, error)
	Reject(ctx context.Context, restaurantID, orderID uuid.UUID, reason string) (domain.Order, error)
	UpdateStatus(ctx context.Context, restaurantID, orderID uuid.UUID, status domain.OrderStatus) (domain.Order, error)
}

// Server реализует сгенерированный по OpenAPI интерфейс api.StrictServerInterface
type Server struct {
	catalog CatalogService
	partner PartnerService
	cart    CartService
	order   OrderService
}

// NewServer собирает реализацию API из сервисов
func NewServer(deps Services) *Server {
	return &Server{
		catalog: deps.Catalog,
		partner: deps.Partner,
		cart:    deps.Cart,
		order:   deps.Order,
	}
}

// Services набор сервисов, на которые опирается транспорт
type Services struct {
	Catalog CatalogService
	Partner PartnerService
	Cart    CartService
	Order   OrderService
}

var _ api.StrictServerInterface = (*Server)(nil)
