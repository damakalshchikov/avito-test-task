package http

import (
	"context"
	"time"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/api"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/service/catalog"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/service/order"
)

// CreateOrder оформляет заказ из корзины
func (s *Server) CreateOrder(ctx context.Context, request api.CreateOrderRequestObject) (api.CreateOrderResponseObject, error) {
	userID, err := mustUserID(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, domain.Invalid("Пустое тело запроса")
	}

	created, err := s.order.Create(ctx, domain.CreateOrder{
		UserID:          userID,
		DeliveryAddress: request.Body.DeliveryAddress,
		Comment:         value(request.Body.Comment),
		IdempotencyKey:  value(request.Params.IdempotencyKey),
	})
	if err != nil {
		return nil, err
	}
	return api.CreateOrder201JSONResponse(toOrder(created)), nil
}

// ListOrders отдаёт заказы пользователя
func (s *Server) ListOrders(ctx context.Context, request api.ListOrdersRequestObject) (api.ListOrdersResponseObject, error) {
	userID, err := mustUserID(ctx)
	if err != nil {
		return nil, err
	}

	list, err := s.order.ListForUser(ctx, userID, catalog.Page{
		Number: value(request.Params.Page),
		Size:   value(request.Params.PageSize),
	})
	if err != nil {
		return nil, err
	}
	return api.ListOrders200JSONResponse(toOrderList(list)), nil
}

// GetOrder отдаёт заказ пользователя с историей статусов
func (s *Server) GetOrder(ctx context.Context, request api.GetOrderRequestObject) (api.GetOrderResponseObject, error) {
	userID, err := mustUserID(ctx)
	if err != nil {
		return nil, err
	}

	found, err := s.order.GetForUser(ctx, userID, request.OrderId)
	if err != nil {
		return nil, err
	}
	return api.GetOrder200JSONResponse(toOrder(found)), nil
}

// CancelOrder отменяет заказ по инициативе пользователя
func (s *Server) CancelOrder(ctx context.Context, request api.CancelOrderRequestObject) (api.CancelOrderResponseObject, error) {
	userID, err := mustUserID(ctx)
	if err != nil {
		return nil, err
	}

	cancelled, err := s.order.Cancel(ctx, userID, request.OrderId)
	if err != nil {
		return nil, err
	}
	return api.CancelOrder200JSONResponse(toOrder(cancelled)), nil
}

// PartnerListOrders отдаёт заказы заведения
func (s *Server) PartnerListOrders(ctx context.Context, request api.PartnerListOrdersRequestObject) (api.PartnerListOrdersResponseObject, error) {
	restaurant, err := mustRestaurant(ctx)
	if err != nil {
		return nil, err
	}

	filter := order.PartnerFilter{
		Since: request.Params.Since,
		Page: catalog.Page{
			Number: value(request.Params.Page),
			Size:   value(request.Params.PageSize),
		},
	}
	if request.Params.Status != nil {
		status := domain.OrderStatus(*request.Params.Status)
		filter.Status = &status
	}

	list, err := s.order.ListForRestaurant(ctx, restaurant.ID, filter)
	if err != nil {
		return nil, err
	}
	return api.PartnerListOrders200JSONResponse(toPartnerOrderList(list)), nil
}

// PartnerGetOrder отдаёт заказ заведения
func (s *Server) PartnerGetOrder(ctx context.Context, request api.PartnerGetOrderRequestObject) (api.PartnerGetOrderResponseObject, error) {
	restaurant, err := mustRestaurant(ctx)
	if err != nil {
		return nil, err
	}

	found, err := s.order.GetForRestaurant(ctx, restaurant.ID, request.OrderId)
	if err != nil {
		return nil, err
	}
	return api.PartnerGetOrder200JSONResponse(toPartnerOrder(found)), nil
}

// PartnerAcceptOrder принимает заказ заведением
func (s *Server) PartnerAcceptOrder(ctx context.Context, request api.PartnerAcceptOrderRequestObject) (api.PartnerAcceptOrderResponseObject, error) {
	restaurant, err := mustRestaurant(ctx)
	if err != nil {
		return nil, err
	}

	var expectedAt *time.Time
	if request.Body != nil {
		expectedAt = request.Body.ExpectedAt
	}

	accepted, err := s.order.Accept(ctx, restaurant.ID, request.OrderId, expectedAt)
	if err != nil {
		return nil, err
	}
	return api.PartnerAcceptOrder200JSONResponse(toPartnerOrder(accepted)), nil
}

// PartnerRejectOrder отклоняет заказ с причиной
func (s *Server) PartnerRejectOrder(ctx context.Context, request api.PartnerRejectOrderRequestObject) (api.PartnerRejectOrderResponseObject, error) {
	restaurant, err := mustRestaurant(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, domain.Invalid("Пустое тело запроса")
	}

	rejected, err := s.order.Reject(ctx, restaurant.ID, request.OrderId, request.Body.Reason)
	if err != nil {
		return nil, err
	}
	return api.PartnerRejectOrder200JSONResponse(toPartnerOrder(rejected)), nil
}

// PartnerUpdateOrderStatus продвигает заказ по цепочке готовки и доставки
func (s *Server) PartnerUpdateOrderStatus(ctx context.Context, request api.PartnerUpdateOrderStatusRequestObject) (api.PartnerUpdateOrderStatusResponseObject, error) {
	restaurant, err := mustRestaurant(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, domain.Invalid("Пустое тело запроса")
	}

	updated, err := s.order.UpdateStatus(ctx, restaurant.ID, request.OrderId, domain.OrderStatus(request.Body.Status))
	if err != nil {
		return nil, err
	}
	return api.PartnerUpdateOrderStatus200JSONResponse(toPartnerOrder(updated)), nil
}
