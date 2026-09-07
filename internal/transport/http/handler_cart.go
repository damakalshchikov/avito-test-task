package http

import (
	"context"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/api"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
)

// GetCart отдаёт корзину пользователя
func (s *Server) GetCart(ctx context.Context, _ api.GetCartRequestObject) (api.GetCartResponseObject, error) {
	userID, err := mustUserID(ctx)
	if err != nil {
		return nil, err
	}

	view, err := s.cart.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	return api.GetCart200JSONResponse(toCart(view)), nil
}

// SetCartItem задаёт количество позиции в корзине
func (s *Server) SetCartItem(ctx context.Context, request api.SetCartItemRequestObject) (api.SetCartItemResponseObject, error) {
	userID, err := mustUserID(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, domain.Invalid("Пустое тело запроса")
	}

	view, err := s.cart.SetItem(ctx, userID, request.Body.MenuItemId, request.Body.Quantity)
	if err != nil {
		return nil, err
	}
	return api.SetCartItem200JSONResponse(toCart(view)), nil
}

// ClearCart очищает корзину пользователя
func (s *Server) ClearCart(ctx context.Context, _ api.ClearCartRequestObject) (api.ClearCartResponseObject, error) {
	userID, err := mustUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.cart.Clear(ctx, userID); err != nil {
		return nil, err
	}
	return api.ClearCart204Response{}, nil
}
