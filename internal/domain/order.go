package domain

import (
	"time"

	"github.com/google/uuid"
)

// Order заказ пользователя
type Order struct {
	ID              uuid.UUID
	Number          int64
	UserID          uuid.UUID
	RestaurantID    uuid.UUID
	RestaurantName  string
	Status          OrderStatus
	DeliveryAddress string
	Comment         string
	Subtotal        int64
	DeliveryFee     int64
	Total           int64
	RejectReason    *string
	ExpectedAt      *time.Time
	Customer        Customer
	Items           []OrderItem
	History         []OrderStatusChange
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// OrderItem позиция заказа со снимком названия и цены
type OrderItem struct {
	ID         uuid.UUID
	MenuItemID uuid.UUID
	ExternalID string
	Name       string
	Price      int64
	Quantity   int
}

// OrderStatusChange запись в истории переходов заказа
type OrderStatusChange struct {
	FromStatus *OrderStatus
	ToStatus   OrderStatus
	Actor      Actor
	Reason     string
	CreatedAt  time.Time
}

// Customer данные заказчика, которые видит заведение
type Customer struct {
	Name  string
	Phone string
}

// CreateOrder параметры оформления заказа из корзины
type CreateOrder struct {
	UserID          uuid.UUID
	DeliveryAddress string
	Comment         string
	IdempotencyKey  string
}

// OrderFilter параметры выборки заказов
type OrderFilter struct {
	UserID       *uuid.UUID
	RestaurantID *uuid.UUID
	Status       *OrderStatus
	Since        *time.Time
	Limit        int
	Offset       int
}

// StatusChange переход, который применяется к заказу
type StatusChange struct {
	To         OrderStatus
	Actor      Actor
	Reason     string
	ExpectedAt *time.Time
}

// Validate проверяет параметры оформления заказа
func (c CreateOrder) Validate() error {
	const (
		minAddressLen = 5
		maxAddressLen = 500
		maxCommentLen = 1000
	)

	address := []rune(c.DeliveryAddress)
	if len(address) < minAddressLen {
		return Invalid("Слишком короткий адрес доставки", Detail{
			Code: CodeValidation, Message: "Адрес обязателен", Field: "delivery_address",
		})
	}
	if len(address) > maxAddressLen {
		return Invalid("Слишком длинный адрес доставки", Detail{
			Code: CodeValidation, Message: "Не более 500 символов", Field: "delivery_address",
		})
	}
	if len([]rune(c.Comment)) > maxCommentLen {
		return Invalid("Слишком длинный комментарий", Detail{
			Code: CodeValidation, Message: "Не более 1000 символов", Field: "comment",
		})
	}
	return nil
}
