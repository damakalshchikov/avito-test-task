package http

import (
	"context"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/api"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
)

// PartnerGetProfile отдаёт профиль заведения, аутентифицированного по API-ключу
func (s *Server) PartnerGetProfile(ctx context.Context, _ api.PartnerGetProfileRequestObject) (api.PartnerGetProfileResponseObject, error) {
	restaurant, err := mustRestaurant(ctx)
	if err != nil {
		return nil, err
	}
	return api.PartnerGetProfile200JSONResponse(toRestaurantProfile(restaurant)), nil
}

// PartnerUpdateProfile обновляет профиль заведения
func (s *Server) PartnerUpdateProfile(ctx context.Context, request api.PartnerUpdateProfileRequestObject) (api.PartnerUpdateProfileResponseObject, error) {
	restaurant, err := mustRestaurant(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, domain.Invalid("Пустое тело запроса")
	}

	update := domain.RestaurantUpdate{
		DeliveryFee:    request.Body.DeliveryFee,
		MinOrderAmount: request.Body.MinOrderAmount,
		Description:    request.Body.Description,
	}
	if request.Body.Status != nil {
		status := domain.RestaurantStatus(*request.Body.Status)
		update.Status = &status
	}

	webhookURL, err := nullableValue(request.Body.WebhookUrl)
	if err != nil {
		return nil, domain.Invalid("Некорректный webhook_url").Wrap(err)
	}
	update.WebhookURL = webhookURL

	updated, err := s.partner.UpdateProfile(ctx, restaurant.ID, update)
	if err != nil {
		return nil, err
	}
	return api.PartnerUpdateProfile200JSONResponse(toRestaurantProfile(updated)), nil
}

// PartnerSyncMenu полностью синхронизирует меню заведения
func (s *Server) PartnerSyncMenu(ctx context.Context, request api.PartnerSyncMenuRequestObject) (api.PartnerSyncMenuResponseObject, error) {
	restaurant, err := mustRestaurant(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, domain.Invalid("Пустое тело запроса")
	}

	result, err := s.partner.SyncMenu(ctx, restaurant.ID, toMenuSync(*request.Body))
	if err != nil {
		return nil, err
	}
	return api.PartnerSyncMenu200JSONResponse{
		CategoriesTotal: result.CategoriesTotal,
		ItemsCreated:    result.ItemsCreated,
		ItemsUpdated:    result.ItemsUpdated,
		ItemsDisabled:   result.ItemsDisabled,
	}, nil
}

// PartnerUpdateMenuItem меняет доступность, остаток или цену одной позиции
func (s *Server) PartnerUpdateMenuItem(ctx context.Context, request api.PartnerUpdateMenuItemRequestObject) (api.PartnerUpdateMenuItemResponseObject, error) {
	restaurant, err := mustRestaurant(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, domain.Invalid("Пустое тело запроса")
	}

	update := domain.MenuItemUpdate{
		IsAvailable: request.Body.IsAvailable,
		Price:       request.Body.Price,
	}

	stockQty, err := nullableValue(request.Body.StockQty)
	if err != nil {
		return nil, domain.Invalid("Некорректный stock_qty").Wrap(err)
	}
	update.StockQty = stockQty

	item, err := s.partner.UpdateMenuItem(ctx, restaurant.ID, request.ExternalId, update)
	if err != nil {
		return nil, err
	}
	return api.PartnerUpdateMenuItem200JSONResponse(toMenuItem(item)), nil
}

// AdminCreateRestaurant подключает заведение и выдаёт учётные данные
func (s *Server) AdminCreateRestaurant(ctx context.Context, request api.AdminCreateRestaurantRequestObject) (api.AdminCreateRestaurantResponseObject, error) {
	if request.Body == nil {
		return nil, domain.Invalid("Пустое тело запроса")
	}

	credentials, err := s.partner.CreateRestaurant(ctx, domain.Restaurant{
		Slug:           request.Body.Slug,
		Name:           request.Body.Name,
		Description:    value(request.Body.Description),
		Address:        request.Body.Address,
		DeliveryFee:    value(request.Body.DeliveryFee),
		MinOrderAmount: value(request.Body.MinOrderAmount),
	})
	if err != nil {
		return nil, err
	}
	return api.AdminCreateRestaurant201JSONResponse{
		Restaurant:    toRestaurantProfile(credentials.Restaurant),
		ApiKey:        credentials.APIKey,
		WebhookSecret: credentials.WebhookSecret,
	}, nil
}
