package http

import (
	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/api"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/service/cart"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/service/catalog"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/service/order"
)

// nullableFrom переводит необязательное значение в nullable-поле API
func nullableFrom[T any](p *T) nullable.Nullable[T] {
	if p == nil {
		return nullable.NewNullNullable[T]()
	}
	return nullable.NewNullableWithValue(*p)
}

// nullableValue разбирает nullable-поле запроса в три состояния: не прислано, прислано null, прислано значение
func nullableValue[T any](n nullable.Nullable[T]) (**T, error) {
	if !n.IsSpecified() {
		return nil, nil
	}
	if n.IsNull() {
		var none *T
		return &none, nil
	}
	v, err := n.Get()
	if err != nil {
		return nil, err
	}
	value := &v
	return &value, nil
}

// optional возвращает указатель на строку или nil для пустого значения
func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// toRestaurant переводит заведение в публичную схему каталога
func toRestaurant(r domain.Restaurant) api.Restaurant {
	return api.Restaurant{
		Id:             r.ID,
		Slug:           r.Slug,
		Name:           r.Name,
		Description:    optional(r.Description),
		Address:        r.Address,
		Status:         api.RestaurantStatus(r.Status),
		DeliveryFee:    r.DeliveryFee,
		MinOrderAmount: r.MinOrderAmount,
	}
}

// toRestaurantProfile добавляет к карточке поля, видимые только самому заведению
func toRestaurantProfile(r domain.Restaurant) api.RestaurantProfile {
	createdAt := r.CreatedAt
	return api.RestaurantProfile{
		Id:             r.ID,
		Slug:           r.Slug,
		Name:           r.Name,
		Description:    optional(r.Description),
		Address:        r.Address,
		Status:         api.RestaurantStatus(r.Status),
		DeliveryFee:    r.DeliveryFee,
		MinOrderAmount: r.MinOrderAmount,
		WebhookUrl:     nullableFrom(r.WebhookURL),
		CreatedAt:      &createdAt,
	}
}

func toRestaurantList(list catalog.RestaurantList) api.RestaurantList {
	items := make([]api.Restaurant, 0, len(list.Items))
	for _, restaurant := range list.Items {
		items = append(items, toRestaurant(restaurant))
	}
	return api.RestaurantList{Items: items, Pagination: toPagination(list.Page, list.Total)}
}

// toMenuItem переводит позицию меню, подставляя в is_available вычисленную доступность
func toMenuItem(item domain.MenuItem) api.MenuItem {
	return api.MenuItem{
		Id:          item.ID,
		ExternalId:  item.ExternalID,
		Name:        item.Name,
		Description: optional(item.Description),
		Price:       item.Price,
		IsAvailable: item.InStock(),
		StockQty:    nullableFrom(item.StockQty),
		ImageUrl:    item.ImageURL,
	}
}

func toMenu(restaurantID uuid.UUID, categories []domain.MenuCategory) api.Menu {
	apiCategories := make([]api.MenuCategory, 0, len(categories))
	for _, category := range categories {
		items := make([]api.MenuItem, 0, len(category.Items))
		for _, item := range category.Items {
			items = append(items, toMenuItem(item))
		}
		apiCategories = append(apiCategories, api.MenuCategory{
			Id:        category.ID,
			Name:      category.Name,
			SortOrder: category.SortOrder,
			Items:     items,
		})
	}
	return api.Menu{RestaurantId: restaurantID, Categories: apiCategories}
}

// toMenuSync переводит присланное заведением меню в доменную структуру
func toMenuSync(request api.MenuSyncRequest) domain.MenuSync {
	categories := make([]domain.MenuSyncCategory, 0, len(request.Categories))
	for _, category := range request.Categories {
		items := make([]domain.MenuSyncItem, 0, len(category.Items))
		for _, item := range category.Items {
			items = append(items, domain.MenuSyncItem{
				ExternalID:  item.ExternalId,
				Name:        item.Name,
				Description: value(item.Description),
				Price:       item.Price,
				IsAvailable: item.IsAvailable == nil || *item.IsAvailable,
				StockQty:    syncStock(item.StockQty),
				ImageURL:    item.ImageUrl,
			})
		}
		categories = append(categories, domain.MenuSyncCategory{
			Name:      category.Name,
			SortOrder: value(category.SortOrder),
			Items:     items,
		})
	}
	return domain.MenuSync{Categories: categories}
}

// syncStock понимает отсутствие и null одинаково: заведение не ведёт остатки
func syncStock(n nullable.Nullable[int]) *int {
	if !n.IsSpecified() || n.IsNull() {
		return nil
	}
	qty, err := n.Get()
	if err != nil {
		return nil
	}
	return &qty
}

func value[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// toCart переводит корзину в схему API
func toCart(view cart.View) api.Cart {
	items := make([]api.CartItem, 0, len(view.Cart.Items))
	for _, item := range view.Cart.Items {
		items = append(items, api.CartItem{
			MenuItemId:  item.MenuItem.ID,
			Name:        item.MenuItem.Name,
			Price:       item.MenuItem.Price,
			Quantity:    item.Quantity,
			IsAvailable: item.MenuItem.Orderable(item.Quantity),
		})
	}

	result := api.Cart{Items: items, Subtotal: view.Cart.Subtotal()}
	if view.Cart.IsEmpty() {
		return result
	}
	restaurantID := view.Cart.RestaurantID
	result.RestaurantId = &restaurantID
	result.DeliveryFee = &view.DeliveryFee
	result.Total = &view.Total
	return result
}

// toOrder переводит заказ в клиентскую схему
func toOrder(order domain.Order) api.Order {
	items := make([]api.OrderItem, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, api.OrderItem{
			MenuItemId: item.MenuItemID,
			Name:       item.Name,
			Price:      item.Price,
			Quantity:   item.Quantity,
		})
	}

	result := api.Order{
		Id:              order.ID,
		Number:          order.Number,
		RestaurantId:    order.RestaurantID,
		RestaurantName:  order.RestaurantName,
		Status:          api.OrderStatus(order.Status),
		Items:           items,
		DeliveryAddress: order.DeliveryAddress,
		Comment:         optional(order.Comment),
		Subtotal:        order.Subtotal,
		DeliveryFee:     order.DeliveryFee,
		Total:           order.Total,
		RejectReason:    order.RejectReason,
		ExpectedAt:      order.ExpectedAt,
		CreatedAt:       order.CreatedAt,
		UpdatedAt:       order.UpdatedAt,
	}
	if history := toHistory(order.History); history != nil {
		result.History = &history
	}
	return result
}

// toPartnerOrder переводит заказ в схему для заведения
func toPartnerOrder(order domain.Order) api.PartnerOrder {
	items := make([]api.PartnerOrderItem, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, api.PartnerOrderItem{
			MenuItemId: item.MenuItemID,
			ExternalId: item.ExternalID,
			Name:       item.Name,
			Price:      item.Price,
			Quantity:   item.Quantity,
		})
	}

	result := api.PartnerOrder{
		Id:              order.ID,
		Number:          order.Number,
		Status:          api.OrderStatus(order.Status),
		Items:           items,
		DeliveryAddress: order.DeliveryAddress,
		Comment:         optional(order.Comment),
		Customer: &api.OrderCustomer{
			Name:  order.Customer.Name,
			Phone: optional(order.Customer.Phone),
		},
		Subtotal:     order.Subtotal,
		DeliveryFee:  order.DeliveryFee,
		Total:        order.Total,
		RejectReason: order.RejectReason,
		ExpectedAt:   order.ExpectedAt,
		CreatedAt:    order.CreatedAt,
		UpdatedAt:    order.UpdatedAt,
	}
	if history := toHistory(order.History); history != nil {
		result.History = &history
	}
	return result
}

func toHistory(history []domain.OrderStatusChange) []api.OrderStatusChange {
	if len(history) == 0 {
		return nil
	}

	changes := make([]api.OrderStatusChange, 0, len(history))
	for _, change := range history {
		item := api.OrderStatusChange{
			ToStatus:   api.OrderStatus(change.ToStatus),
			Actor:      api.OrderStatusChangeActor(change.Actor),
			Reason:     optional(change.Reason),
			CreatedAt:  change.CreatedAt,
			FromStatus: nullable.NewNullNullable[string](),
		}
		if change.FromStatus != nil {
			item.FromStatus = nullable.NewNullableWithValue(string(*change.FromStatus))
		}
		changes = append(changes, item)
	}
	return changes
}

func toOrderList(list order.List) api.OrderList {
	items := make([]api.Order, 0, len(list.Items))
	for _, o := range list.Items {
		items = append(items, toOrder(o))
	}
	return api.OrderList{Items: items, Pagination: toPagination(list.Page, list.Total)}
}

func toPartnerOrderList(list order.List) api.PartnerOrderList {
	items := make([]api.PartnerOrder, 0, len(list.Items))
	for _, o := range list.Items {
		items = append(items, toPartnerOrder(o))
	}
	return api.PartnerOrderList{Items: items, Pagination: toPagination(list.Page, list.Total)}
}

func toPagination(page catalog.Page, total int) api.Pagination {
	return api.Pagination{Page: page.Number, PageSize: page.Size, Total: total}
}
