package domain

import "github.com/google/uuid"

// Cart корзина пользователя
type Cart struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	RestaurantID uuid.UUID
	Items        []CartItem
}

// CartItem позиция корзины вместе с актуальным состоянием меню
type CartItem struct {
	MenuItem MenuItem
	Quantity int
}

// IsEmpty сообщает, что в корзине нет позиций
func (c Cart) IsEmpty() bool {
	return len(c.Items) == 0
}

// Subtotal возвращает стоимость позиций без доставки
func (c Cart) Subtotal() int64 {
	var subtotal int64
	for _, item := range c.Items {
		subtotal += item.MenuItem.Price * int64(item.Quantity)
	}
	return subtotal
}

// UnavailableItems возвращает позиции, которые нельзя заказать в нужном количестве
func (c Cart) UnavailableItems() []CartItem {
	var unavailable []CartItem
	for _, item := range c.Items {
		if !item.MenuItem.Orderable(item.Quantity) {
			unavailable = append(unavailable, item)
		}
	}
	return unavailable
}

// UnavailableDetails описывает недоступные позиции для ответа клиенту
func UnavailableDetails(items []CartItem) []Detail {
	details := make([]Detail, 0, len(items))
	for _, item := range items {
		message := "Позиция снята с продажи"
		if item.MenuItem.IsAvailable {
			message = "Недостаточно остатка"
		}
		details = append(details, Detail{
			Code:         "out_of_stock",
			Message:      message,
			MenuItemID:   item.MenuItem.ID,
			RequestedQty: item.Quantity,
			AvailableQty: item.MenuItem.Available(),
		})
	}
	return details
}

// MaxCartItemQuantity предельное количество одной позиции в корзине
const MaxCartItemQuantity = 99

// ValidateCartQuantity проверяет запрошенное количество позиции
func ValidateCartQuantity(quantity int) error {
	if quantity < 0 {
		return Invalid("Отрицательное количество", Detail{
			Code: CodeValidation, Message: "Количество не может быть отрицательным", Field: "quantity",
		})
	}
	if quantity > MaxCartItemQuantity {
		return Invalid("Слишком большое количество", Detail{
			Code: CodeValidation, Message: "Не более 99 единиц одной позиции", Field: "quantity",
		})
	}
	return nil
}
