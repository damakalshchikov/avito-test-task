// Package partner реализует сценарии заведения
package partner

import (
	"context"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
)

// RestaurantRepository доступ к заведениям, нужный партнёрским сценариям
type RestaurantRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (domain.Restaurant, error)
	GetByAPIKeyHash(ctx context.Context, hash []byte) (domain.Restaurant, error)
	Create(ctx context.Context, restaurant domain.Restaurant, apiKeyHash []byte) (domain.Restaurant, error)
	Update(ctx context.Context, id uuid.UUID, update domain.RestaurantUpdate) (domain.Restaurant, error)
}

// MenuRepository доступ к меню, нужный партнёрским сценариям
type MenuRepository interface {
	Sync(ctx context.Context, restaurantID uuid.UUID, menu domain.MenuSync) (domain.MenuSyncResult, error)
	UpdateItem(ctx context.Context, restaurantID uuid.UUID, externalID string, update domain.MenuItemUpdate) (domain.MenuItem, error)
}

// Service партнёрские и административные сценарии
type Service struct {
	restaurants RestaurantRepository
	menu        MenuRepository
}

// New создаёт партнёрский сервис
func New(restaurants RestaurantRepository, menu MenuRepository) *Service {
	return &Service{restaurants: restaurants, menu: menu}
}

// Authenticate находит заведение по API-ключу из заголовка X-Api-Key
func (s *Service) Authenticate(ctx context.Context, apiKey string) (domain.Restaurant, error) {
	if apiKey == "" {
		return domain.Restaurant{}, domain.ErrUnauthorized
	}

	restaurant, err := s.restaurants.GetByAPIKeyHash(ctx, domain.HashAPIKey(apiKey))
	if err != nil {
		return domain.Restaurant{}, err
	}
	if restaurant.Status == domain.RestaurantDisabled {
		return domain.Restaurant{}, domain.ErrUnauthorized
	}
	return restaurant, nil
}

// GetProfile возвращает профиль заведения
func (s *Service) GetProfile(ctx context.Context, restaurantID uuid.UUID) (domain.Restaurant, error) {
	return s.restaurants.GetByID(ctx, restaurantID)
}

// UpdateProfile применяет частичное обновление профиля
func (s *Service) UpdateProfile(ctx context.Context, restaurantID uuid.UUID, update domain.RestaurantUpdate) (domain.Restaurant, error) {
	if update.Status != nil {
		switch *update.Status {
		case domain.RestaurantActive, domain.RestaurantPaused:
		case domain.RestaurantDisabled:
			return domain.Restaurant{}, domain.Invalid("Статус disabled выставляется только платформой", domain.Detail{
				Code: domain.CodeValidation, Message: "Допустимы значения active и paused", Field: "status",
			})
		default:
			return domain.Restaurant{}, domain.Invalid("Неизвестный статус заведения", domain.Detail{
				Code: domain.CodeValidation, Message: "Допустимы значения active и paused", Field: "status",
			})
		}
	}

	if update.WebhookURL != nil && *update.WebhookURL != nil {
		if err := validateWebhookURL(**update.WebhookURL); err != nil {
			return domain.Restaurant{}, err
		}
	}
	if update.DeliveryFee != nil && *update.DeliveryFee < 0 {
		return domain.Restaurant{}, domain.Invalid("Отрицательная стоимость доставки", domain.Detail{
			Code: domain.CodeValidation, Message: "Значение не может быть отрицательным", Field: "delivery_fee",
		})
	}
	if update.MinOrderAmount != nil && *update.MinOrderAmount < 0 {
		return domain.Restaurant{}, domain.Invalid("Отрицательная минимальная сумма заказа", domain.Detail{
			Code: domain.CodeValidation, Message: "Значение не может быть отрицательным", Field: "min_order_amount",
		})
	}

	return s.restaurants.Update(ctx, restaurantID, update)
}

// SyncMenu полностью синхронизирует меню заведения
func (s *Service) SyncMenu(ctx context.Context, restaurantID uuid.UUID, menu domain.MenuSync) (domain.MenuSyncResult, error) {
	if err := menu.Validate(); err != nil {
		return domain.MenuSyncResult{}, err
	}
	return s.menu.Sync(ctx, restaurantID, menu)
}

// UpdateMenuItem меняет доступность, остаток или цену одной позиции
func (s *Service) UpdateMenuItem(ctx context.Context, restaurantID uuid.UUID, externalID string,
	update domain.MenuItemUpdate,
) (domain.MenuItem, error) {
	if externalID == "" {
		return domain.MenuItem{}, domain.Invalid("Не указан external_id позиции")
	}
	if update.Price != nil && *update.Price < 0 {
		return domain.MenuItem{}, domain.Invalid("Отрицательная цена", domain.Detail{
			Code: domain.CodeValidation, Message: "Цена не может быть отрицательной", Field: "price",
		})
	}
	if update.StockQty != nil && *update.StockQty != nil && **update.StockQty < 0 {
		return domain.MenuItem{}, domain.Invalid("Отрицательный остаток", domain.Detail{
			Code: domain.CodeValidation, Message: "Остаток не может быть отрицательным", Field: "stock_qty",
		})
	}
	return s.menu.UpdateItem(ctx, restaurantID, externalID, update)
}

// Credentials учётные данные, выдаваемые заведению при онбординге
type Credentials struct {
	Restaurant    domain.Restaurant
	APIKey        string
	WebhookSecret string
}

// CreateRestaurant подключает новое заведение к платформе
func (s *Service) CreateRestaurant(ctx context.Context, restaurant domain.Restaurant) (Credentials, error) {
	restaurant.Slug = strings.TrimSpace(restaurant.Slug)
	restaurant.Name = strings.TrimSpace(restaurant.Name)
	restaurant.Address = strings.TrimSpace(restaurant.Address)

	if !domain.ValidSlug(restaurant.Slug) {
		return Credentials{}, domain.Invalid("Некорректный slug", domain.Detail{
			Code:    domain.CodeValidation,
			Message: "Допустимы строчные латинские буквы, цифры и дефис, от 3 до 64 символов",
			Field:   "slug",
		})
	}
	if restaurant.Name == "" {
		return Credentials{}, domain.Invalid("Не указано название", domain.Detail{
			Code: domain.CodeValidation, Message: "Название обязательно", Field: "name",
		})
	}
	if restaurant.Address == "" {
		return Credentials{}, domain.Invalid("Не указан адрес", domain.Detail{
			Code: domain.CodeValidation, Message: "Адрес обязателен", Field: "address",
		})
	}
	if restaurant.DeliveryFee < 0 || restaurant.MinOrderAmount < 0 {
		return Credentials{}, domain.Invalid("Отрицательная сумма", domain.Detail{
			Code: domain.CodeValidation, Message: "Суммы не могут быть отрицательными", Field: "delivery_fee",
		})
	}

	apiKey, err := domain.GenerateAPIKey()
	if err != nil {
		return Credentials{}, err
	}
	webhookSecret, err := domain.GenerateWebhookSecret()
	if err != nil {
		return Credentials{}, err
	}

	restaurant.Status = domain.RestaurantActive
	restaurant.WebhookSecret = webhookSecret

	created, err := s.restaurants.Create(ctx, restaurant, domain.HashAPIKey(apiKey))
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{Restaurant: created, APIKey: apiKey, WebhookSecret: webhookSecret}, nil
}

func validateWebhookURL(raw string) error {
	invalid := domain.Invalid("Некорректный webhook_url", domain.Detail{
		Code: domain.CodeValidation, Message: "Ожидается абсолютный http(s) URL", Field: "webhook_url",
	})

	parsed, err := url.Parse(raw)
	if err != nil {
		return invalid.Wrap(err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return invalid
	}
	if parsed.Host == "" {
		return invalid
	}
	return nil
}
