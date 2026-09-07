package domain

import "fmt"

// OrderStatus статус заказа
type OrderStatus string

// Жизненный цикл заказа
const (
	OrderNew        OrderStatus = "new"
	OrderAccepted   OrderStatus = "accepted"
	OrderCooking    OrderStatus = "cooking"
	OrderReady      OrderStatus = "ready"
	OrderDelivering OrderStatus = "delivering"
	OrderDelivered  OrderStatus = "delivered"
	OrderRejected   OrderStatus = "rejected"
	OrderCancelled  OrderStatus = "cancelled"
)

// Actor инициатор перехода статуса
type Actor string

// Возможные инициаторы перехода
const (
	ActorUser       Actor = "user"
	ActorRestaurant Actor = "restaurant"
	ActorSystem     Actor = "system"
)

var allowedTransitions = map[OrderStatus]map[OrderStatus]Actor{
	OrderNew: {
		OrderAccepted:  ActorRestaurant,
		OrderRejected:  ActorRestaurant,
		OrderCancelled: ActorUser,
	},
	OrderAccepted: {
		OrderCooking:   ActorRestaurant,
		OrderCancelled: ActorUser,
	},
	OrderCooking:    {OrderReady: ActorRestaurant},
	OrderReady:      {OrderDelivering: ActorRestaurant},
	OrderDelivering: {OrderDelivered: ActorRestaurant},
}

// Valid сообщает, известен ли статус
func (s OrderStatus) Valid() bool {
	switch s {
	case OrderNew, OrderAccepted, OrderCooking, OrderReady,
		OrderDelivering, OrderDelivered, OrderRejected, OrderCancelled:
		return true
	default:
		return false
	}
}

// IsFinal сообщает, что заказ завершён и переходы из него невозможны
func (s OrderStatus) IsFinal() bool {
	switch s {
	case OrderDelivered, OrderRejected, OrderCancelled:
		return true
	default:
		return false
	}
}

// ReturnsStock сообщает, что при переходе в этот статус зарезервированные остатки нужно вернуть в меню
func (s OrderStatus) ReturnsStock() bool {
	return s == OrderRejected || s == OrderCancelled
}

// CanCancel сообщает, может ли пользователь отменить заказ в этом статусе
func (s OrderStatus) CanCancel() bool {
	_, ok := allowedTransitions[s][OrderCancelled]
	return ok
}

// NextStatuses возвращает статусы, доступные инициатору из текущего
func (s OrderStatus) NextStatuses(actor Actor) []OrderStatus {
	var next []OrderStatus
	for to, required := range allowedTransitions[s] {
		if required == actor {
			next = append(next, to)
		}
	}
	return next
}

// CanTransition проверяет допустимость перехода и права инициатора
func CanTransition(from, to OrderStatus, actor Actor) error {
	if !from.Valid() || !to.Valid() {
		return Invalid("Неизвестный статус заказа")
	}
	if from == to {
		return Conflict(CodeInvalidTransition, fmt.Sprintf("Заказ уже в статусе %q", to))
	}
	if from.IsFinal() {
		return Conflict(CodeInvalidTransition,
			fmt.Sprintf("Заказ завершён в статусе %q, изменить его нельзя", from))
	}

	required, ok := allowedTransitions[from][to]
	if !ok {
		return Conflict(CodeInvalidTransition,
			fmt.Sprintf("Переход из %q в %q не предусмотрен", from, to))
	}
	if required != actor {
		return Conflict(CodeInvalidTransition,
			fmt.Sprintf("Перевести заказ из %q в %q может только %s", from, to, required))
	}
	return nil
}
