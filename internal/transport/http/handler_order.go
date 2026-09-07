package http

import (
	"context"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/api"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
)

// GetCart заглушка до реализации корзины
func (s *Server) GetCart(_ context.Context, _ api.GetCartRequestObject) (api.GetCartResponseObject, error) {
	return nil, domain.ErrNotImplemented
}

// ClearCart заглушка до реализации корзины
func (s *Server) ClearCart(_ context.Context, _ api.ClearCartRequestObject) (api.ClearCartResponseObject, error) {
	return nil, domain.ErrNotImplemented
}

// SetCartItem заглушка до реализации корзины
func (s *Server) SetCartItem(_ context.Context, _ api.SetCartItemRequestObject) (api.SetCartItemResponseObject, error) {
	return nil, domain.ErrNotImplemented
}

// CreateOrder заглушка до реализации заказов
func (s *Server) CreateOrder(_ context.Context, _ api.CreateOrderRequestObject) (api.CreateOrderResponseObject, error) {
	return nil, domain.ErrNotImplemented
}

// ListOrders заглушка до реализации заказов
func (s *Server) ListOrders(_ context.Context, _ api.ListOrdersRequestObject) (api.ListOrdersResponseObject, error) {
	return nil, domain.ErrNotImplemented
}

// GetOrder заглушка до реализации заказов
func (s *Server) GetOrder(_ context.Context, _ api.GetOrderRequestObject) (api.GetOrderResponseObject, error) {
	return nil, domain.ErrNotImplemented
}

// CancelOrder заглушка до реализации заказов
func (s *Server) CancelOrder(_ context.Context, _ api.CancelOrderRequestObject) (api.CancelOrderResponseObject, error) {
	return nil, domain.ErrNotImplemented
}

// PartnerListOrders заглушка до реализации заказов
func (s *Server) PartnerListOrders(_ context.Context, _ api.PartnerListOrdersRequestObject) (api.PartnerListOrdersResponseObject, error) {
	return nil, domain.ErrNotImplemented
}

// PartnerGetOrder заглушка до реализации заказов
func (s *Server) PartnerGetOrder(_ context.Context, _ api.PartnerGetOrderRequestObject) (api.PartnerGetOrderResponseObject, error) {
	return nil, domain.ErrNotImplemented
}

// PartnerAcceptOrder заглушка до реализации заказов
func (s *Server) PartnerAcceptOrder(_ context.Context, _ api.PartnerAcceptOrderRequestObject) (api.PartnerAcceptOrderResponseObject, error) {
	return nil, domain.ErrNotImplemented
}

// PartnerRejectOrder заглушка до реализации заказов
func (s *Server) PartnerRejectOrder(_ context.Context, _ api.PartnerRejectOrderRequestObject) (api.PartnerRejectOrderResponseObject, error) {
	return nil, domain.ErrNotImplemented
}

// PartnerUpdateOrderStatus заглушка до реализации заказов
func (s *Server) PartnerUpdateOrderStatus(_ context.Context, _ api.PartnerUpdateOrderStatusRequestObject) (api.PartnerUpdateOrderStatusResponseObject, error) {
	return nil, domain.ErrNotImplemented
}
