package domain_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
)

func TestSignWebhook(t *testing.T) {
	t.Parallel()

	body := []byte(`{"event_id":"1"}`)
	signature := domain.SignWebhook("secret", body)

	if !strings.HasPrefix(signature, "sha256=") {
		t.Errorf("подпись = %q, ожидался префикс sha256=", signature)
	}
	if signature != domain.SignWebhook("secret", body) {
		t.Error("подпись одного тела должна быть стабильной")
	}
	if signature == domain.SignWebhook("other", body) {
		t.Error("подписи с разными секретами не должны совпадать")
	}
	if signature == domain.SignWebhook("secret", []byte(`{"event_id":"2"}`)) {
		t.Error("подписи разных тел не должны совпадать")
	}
}

func TestVerifyWebhook(t *testing.T) {
	t.Parallel()

	body := []byte(`{"order":{}}`)
	signature := domain.SignWebhook("secret", body)

	if !domain.VerifyWebhook("secret", body, signature) {
		t.Error("корректная подпись должна проходить проверку")
	}
	if domain.VerifyWebhook("secret", []byte(`{"order":{"id":1}}`), signature) {
		t.Error("подменённое тело не должно проходить проверку")
	}
	if domain.VerifyWebhook("other", body, signature) {
		t.Error("чужой секрет не должен проходить проверку")
	}
	if domain.VerifyWebhook("secret", body, "sha256=deadbeef") {
		t.Error("произвольная подпись не должна проходить проверку")
	}
}

func TestNewWebhookEventCarriesExternalIDs(t *testing.T) {
	t.Parallel()

	order := domain.Order{
		ID:              uuid.New(),
		Number:          17,
		Status:          domain.OrderNew,
		DeliveryAddress: "Москва, ул. Примерная, 1",
		Customer:        domain.Customer{Name: "Иван", Phone: "+79990000001"},
		Subtotal:        100000,
		DeliveryFee:     19900,
		Total:           119900,
		Items: []domain.OrderItem{
			{MenuItemID: uuid.New(), ExternalID: "soup-borsch", Name: "Борщ", Price: 39000, Quantity: 2},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	event := domain.NewWebhookEvent(domain.WebhookOrderCreated, order)

	if event.EventID == uuid.Nil {
		t.Error("событию нужен идентификатор для дедупликации на стороне заведения")
	}
	if event.Type != domain.WebhookOrderCreated {
		t.Errorf("тип = %q, want order.created", event.Type)
	}
	if len(event.Order.Items) != 1 || event.Order.Items[0].ExternalID != "soup-borsch" {
		t.Error("заведение узнаёт свои позиции по external_id, он обязан быть в событии")
	}
	if event.Order.Customer.Name != "Иван" {
		t.Error("контакты заказчика нужны заведению для доставки")
	}

	// Два события по одному заказу различимы
	if domain.NewWebhookEvent(domain.WebhookOrderCreated, order).EventID == event.EventID {
		t.Error("идентификаторы разных событий не должны совпадать")
	}
}

func TestWebhookEventJSONMatchesContract(t *testing.T) {
	t.Parallel()

	event := domain.NewWebhookEvent(domain.WebhookOrderCancelled, domain.Order{
		ID:     uuid.New(),
		Number: 1,
		Status: domain.OrderCancelled,
		Items:  []domain.OrderItem{{ExternalID: "item-1", Name: "Блюдо", Price: 100, Quantity: 1}},
	})

	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Поля из схемы WebhookEvent в api/openapi.yaml
	for _, field := range []string{"event_id", "type", "occurred_at", "order"} {
		if _, ok := decoded[field]; !ok {
			t.Errorf("в теле события нет обязательного поля %q", field)
		}
	}

	order, ok := decoded["order"].(map[string]any)
	if !ok {
		t.Fatal("поле order должно быть объектом")
	}
	for _, field := range []string{"id", "number", "status", "items", "subtotal", "delivery_fee", "total"} {
		if _, ok := order[field]; !ok {
			t.Errorf("в заказе нет обязательного поля %q", field)
		}
	}
}
