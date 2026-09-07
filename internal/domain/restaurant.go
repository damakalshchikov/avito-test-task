package domain

import (
	"time"

	"github.com/google/uuid"
)

// RestaurantStatus состояние заведения на платформе
type RestaurantStatus string

// Возможные статусы заведения
const (
	RestaurantActive RestaurantStatus = "active"
	RestaurantPaused RestaurantStatus = "paused"
	RestaurantDisabled RestaurantStatus = "disabled"
)

// Valid сообщает, известен ли статус
func (s RestaurantStatus) Valid() bool {
	switch s {
	case RestaurantActive, RestaurantPaused, RestaurantDisabled:
		return true
	default:
		return false
	}
}

// CanAcceptOrders сообщает, можно ли сейчас оформить заказ в этом заведении
func (s RestaurantStatus) CanAcceptOrders() bool {
	return s == RestaurantActive
}

// Restaurant заведение-партнёр платформы
type Restaurant struct {
	ID             uuid.UUID
	Slug           string
	Name           string
	Description    string
	Address        string
	Status         RestaurantStatus
	WebhookURL     *string
	WebhookSecret  string
	DeliveryFee    int64
	MinOrderAmount int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// RestaurantFilter параметры выборки заведений в каталоге
type RestaurantFilter struct {
	Search string
	Statuses []RestaurantStatus
	Limit    int
	Offset   int
}

// RestaurantUpdate частичное обновление профиля заведения самим заведением
type RestaurantUpdate struct {
	Status         *RestaurantStatus
	WebhookURL     **string
	DeliveryFee    *int64
	MinOrderAmount *int64
	Description    *string
}
