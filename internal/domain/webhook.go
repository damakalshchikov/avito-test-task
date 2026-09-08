package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
)

// WebhookEventType тип события, отправляемого заведению
type WebhookEventType string

// События, о которых платформа уведомляет заведение
const (
	// WebhookOrderCreated пользователь оформил заказ
	WebhookOrderCreated WebhookEventType = "order.created"
	// WebhookOrderCancelled пользователь отменил заказ до начала готовки
	WebhookOrderCancelled WebhookEventType = "order.cancelled"
)

// SignatureHeader заголовок с подписью тела webhook-а
const SignatureHeader = "X-Kitchen-Signature"

// WebhookEvent тело запроса, которое платформа отправляет заведению
type WebhookEvent struct {
	EventID    uuid.UUID        `json:"event_id"`
	Type       WebhookEventType `json:"type"`
	OccurredAt time.Time        `json:"occurred_at"`
	Order      WebhookOrder     `json:"order"`
}

// WebhookOrder заказ в теле события, повторяет схему PartnerOrder
type WebhookOrder struct {
	ID              uuid.UUID          `json:"id"`
	Number          int64              `json:"number"`
	Status          OrderStatus        `json:"status"`
	Items           []WebhookOrderItem `json:"items"`
	DeliveryAddress string             `json:"delivery_address"`
	Comment         string             `json:"comment,omitempty"`
	Customer        WebhookCustomer    `json:"customer"`
	Subtotal        int64              `json:"subtotal"`
	DeliveryFee     int64              `json:"delivery_fee"`
	Total           int64              `json:"total"`
	RejectReason    *string            `json:"reject_reason,omitempty"`
	ExpectedAt      *time.Time         `json:"expected_at,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

// WebhookOrderItem позиция заказа в теле события
type WebhookOrderItem struct {
	MenuItemID uuid.UUID `json:"menu_item_id"`
	ExternalID string    `json:"external_id"`
	Name       string    `json:"name"`
	Price      int64     `json:"price"`
	Quantity   int       `json:"quantity"`
}

// WebhookCustomer контакты заказчика для доставки
type WebhookCustomer struct {
	Name  string `json:"name"`
	Phone string `json:"phone,omitempty"`
}

// NewWebhookEvent собирает событие по заказу
func NewWebhookEvent(eventType WebhookEventType, order Order) WebhookEvent {
	items := make([]WebhookOrderItem, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, WebhookOrderItem{
			MenuItemID: item.MenuItemID,
			ExternalID: item.ExternalID,
			Name:       item.Name,
			Price:      item.Price,
			Quantity:   item.Quantity,
		})
	}

	return WebhookEvent{
		EventID:    uuid.New(),
		Type:       eventType,
		OccurredAt: time.Now().UTC(),
		Order: WebhookOrder{
			ID:              order.ID,
			Number:          order.Number,
			Status:          order.Status,
			Items:           items,
			DeliveryAddress: order.DeliveryAddress,
			Comment:         order.Comment,
			Customer:        WebhookCustomer{Name: order.Customer.Name, Phone: order.Customer.Phone},
			Subtotal:        order.Subtotal,
			DeliveryFee:     order.DeliveryFee,
			Total:           order.Total,
			RejectReason:    order.RejectReason,
			ExpectedAt:      order.ExpectedAt,
			CreatedAt:       order.CreatedAt,
			UpdatedAt:       order.UpdatedAt,
		},
	}
}

// SignWebhook возвращает значение заголовка подписи для тела запроса
func SignWebhook(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// VerifyWebhook проверяет подпись тела запроса за постоянное время
func VerifyWebhook(secret string, body []byte, signature string) bool {
	expected := SignWebhook(secret, body)
	return hmac.Equal([]byte(expected), []byte(signature))
}
