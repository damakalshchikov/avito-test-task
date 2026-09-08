package http

import (
	"context"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/api"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/service/catalog"
)

// ListRestaurants отдаёт страницу каталога заведений
func (s *Server) ListRestaurants(ctx context.Context, request api.ListRestaurantsRequestObject) (api.ListRestaurantsResponseObject, error) {
	list, err := s.catalog.ListRestaurants(ctx, value(request.Params.Search), catalog.Page{
		Number: value(request.Params.Page),
		Size:   value(request.Params.PageSize),
	})
	if err != nil {
		return nil, err
	}
	return api.ListRestaurants200JSONResponse(toRestaurantList(list)), nil
}

// GetRestaurant отдаёт карточку заведения
func (s *Server) GetRestaurant(ctx context.Context, request api.GetRestaurantRequestObject) (api.GetRestaurantResponseObject, error) {
	restaurant, err := s.catalog.GetRestaurant(ctx, request.RestaurantId)
	if err != nil {
		return nil, err
	}
	return api.GetRestaurant200JSONResponse(toRestaurant(restaurant)), nil
}

// GetRestaurantMenu отдаёт меню заведения
func (s *Server) GetRestaurantMenu(ctx context.Context, request api.GetRestaurantMenuRequestObject) (api.GetRestaurantMenuResponseObject, error) {
	categories, err := s.catalog.GetMenu(ctx, request.RestaurantId)
	if err != nil {
		return nil, err
	}
	return api.GetRestaurantMenu200JSONResponse(toMenu(request.RestaurantId, categories)), nil
}
