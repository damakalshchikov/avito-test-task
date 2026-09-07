package http

import (
	"context"

	"github.com/google/uuid"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/api"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/service/catalog"
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

// Server реализует сгенерированный по OpenAPI интерфейс api.StrictServerInterface
type Server struct {
	catalog CatalogService
	partner PartnerService
}

// NewServer собирает реализацию API из сервисов
func NewServer(catalogService CatalogService, partnerService PartnerService) *Server {
	return &Server{catalog: catalogService, partner: partnerService}
}

var _ api.StrictServerInterface = (*Server)(nil)
