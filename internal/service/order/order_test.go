package order_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/service/catalog"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/service/order"
)

type orderRepoStub struct {
	order        domain.Order
	lastFilter   domain.OrderFilter
	lastChange   domain.StatusChange
	createCalls  int
	changeCalls  int
	getErr       error
	changeStatus domain.OrderStatus
	byKey        *domain.Order
	keyLookups   int
}

func (s *orderRepoStub) Create(_ context.Context, _ domain.Cart, _ domain.CreateOrder) (domain.Order, error) {
	s.createCalls++
	return s.order, nil
}

func (s *orderRepoStub) FindByIdempotencyKey(_ context.Context, _ uuid.UUID, _ string) (*domain.Order, error) {
	if s.byKey == nil {
		return nil, nil
	}
	s.keyLookups++
	return s.byKey, nil
}

func (s *orderRepoStub) Get(_ context.Context, _ uuid.UUID) (domain.Order, error) {
	if s.getErr != nil {
		return domain.Order{}, s.getErr
	}
	return s.order, nil
}

func (s *orderRepoStub) List(_ context.Context, filter domain.OrderFilter) ([]domain.Order, int, error) {
	s.lastFilter = filter
	return []domain.Order{s.order}, 1, nil
}

func (s *orderRepoStub) ChangeStatus(_ context.Context, _ uuid.UUID, change domain.StatusChange) (domain.Order, error) {
	s.changeCalls++
	s.lastChange = change
	updated := s.order
	updated.Status = change.To
	s.changeStatus = change.To
	return updated, nil
}

type cartRepoStub struct {
	cart domain.Cart
}

func (s *cartRepoStub) Get(_ context.Context, _ uuid.UUID) (domain.Cart, error) {
	return s.cart, nil
}

func filledCart(userID uuid.UUID) domain.Cart {
	return domain.Cart{
		ID:           uuid.New(),
		UserID:       userID,
		RestaurantID: uuid.New(),
		Items: []domain.CartItem{
			{MenuItem: domain.MenuItem{ID: uuid.New(), Price: 50000, IsAvailable: true}, Quantity: 1},
		},
	}
}

func TestCreateRejectsEmptyCart(t *testing.T) {
	t.Parallel()

	orders := &orderRepoStub{}
	service := order.New(orders, &cartRepoStub{})

	_, err := service.Create(context.Background(), domain.CreateOrder{
		UserID: uuid.New(), DeliveryAddress: "Москва, ул. Примерная, 1",
	})
	domainErr, ok := domain.AsError(err)
	if !ok || domainErr.Code != domain.CodeCartEmpty {
		t.Fatalf("ожидалась ошибка cart_empty, получено %v", err)
	}
	if orders.createCalls != 0 {
		t.Error("пустая корзина не должна доходить до репозитория")
	}
}

func TestCreateValidatesAddress(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	orders := &orderRepoStub{}
	service := order.New(orders, &cartRepoStub{cart: filledCart(userID)})

	for _, address := range []string{"", "дом"} {
		_, err := service.Create(context.Background(), domain.CreateOrder{
			UserID: userID, DeliveryAddress: address,
		})
		domainErr, ok := domain.AsError(err)
		if !ok || domainErr.Kind != domain.KindInvalid {
			t.Errorf("адрес %q должен отклоняться, получено %v", address, err)
		}
	}
	if orders.createCalls != 0 {
		t.Error("некорректный адрес не должен доходить до репозитория")
	}
}

func TestCreatePassesCartToRepository(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	orders := &orderRepoStub{order: domain.Order{ID: uuid.New(), Status: domain.OrderNew}}
	service := order.New(orders, &cartRepoStub{cart: filledCart(userID)})

	created, err := service.Create(context.Background(), domain.CreateOrder{
		UserID: userID, DeliveryAddress: "Москва, ул. Примерная, 1",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Status != domain.OrderNew {
		t.Errorf("статус нового заказа = %q, want new", created.Status)
	}
	if orders.createCalls != 1 {
		t.Errorf("вызовов репозитория %d, want 1", orders.createCalls)
	}
}

func TestCreateReturnsExistingOrderForSameKey(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	existing := domain.Order{ID: uuid.New(), UserID: userID, Number: 42, Status: domain.OrderNew}
	orders := &orderRepoStub{byKey: &existing}
	// Корзина пуста
	service := order.New(orders, &cartRepoStub{})

	created, err := service.Create(context.Background(), domain.CreateOrder{
		UserID:          userID,
		DeliveryAddress: "Москва, ул. Примерная, 1",
		IdempotencyKey:  "demo-key-1",
	})
	if err != nil {
		t.Fatalf("повтор с тем же ключом должен вернуть заказ: %v", err)
	}
	if created.ID != existing.ID {
		t.Errorf("вернулся другой заказ: %s, want %s", created.ID, existing.ID)
	}
	if orders.createCalls != 0 {
		t.Error("повторный запрос не должен создавать новый заказ")
	}
	if orders.keyLookups != 1 {
		t.Errorf("поисков по ключу %d, want 1", orders.keyLookups)
	}
}

func TestGetForUserHidesForeignOrder(t *testing.T) {
	t.Parallel()

	orders := &orderRepoStub{order: domain.Order{ID: uuid.New(), UserID: uuid.New()}}
	service := order.New(orders, &cartRepoStub{})

	_, err := service.GetForUser(context.Background(), uuid.New(), uuid.New())
	domainErr, ok := domain.AsError(err)
	if !ok || domainErr.Kind != domain.KindNotFound {
		t.Fatalf("чужой заказ должен отдавать not_found, получено %v", err)
	}
}

func TestCancelChecksOwnershipBeforeChange(t *testing.T) {
	t.Parallel()

	orders := &orderRepoStub{order: domain.Order{ID: uuid.New(), UserID: uuid.New(), Status: domain.OrderNew}}
	service := order.New(orders, &cartRepoStub{})

	if _, err := service.Cancel(context.Background(), uuid.New(), uuid.New()); err == nil {
		t.Fatal("отмена чужого заказа должна быть запрещена")
	}
	if orders.changeCalls != 0 {
		t.Error("проверка владельца должна выполняться до смены статуса")
	}
}

func TestCancelUsesUserActor(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	orders := &orderRepoStub{order: domain.Order{ID: uuid.New(), UserID: userID, Status: domain.OrderNew}}
	service := order.New(orders, &cartRepoStub{})

	cancelled, err := service.Cancel(context.Background(), userID, uuid.New())
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelled.Status != domain.OrderCancelled {
		t.Errorf("статус = %q, want cancelled", cancelled.Status)
	}
	if orders.lastChange.Actor != domain.ActorUser {
		t.Errorf("инициатор = %q, want user", orders.lastChange.Actor)
	}
}

func TestRestaurantOperationsCheckOwnership(t *testing.T) {
	t.Parallel()

	foreign := domain.Order{ID: uuid.New(), RestaurantID: uuid.New(), Status: domain.OrderNew}

	operations := map[string]func(*order.Service) error{
		"accept": func(s *order.Service) error {
			_, err := s.Accept(context.Background(), uuid.New(), uuid.New(), nil)
			return err
		},
		"reject": func(s *order.Service) error {
			_, err := s.Reject(context.Background(), uuid.New(), uuid.New(), "нет продуктов")
			return err
		},
		"status": func(s *order.Service) error {
			_, err := s.UpdateStatus(context.Background(), uuid.New(), uuid.New(), domain.OrderCooking)
			return err
		},
	}

	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			orders := &orderRepoStub{order: foreign}
			service := order.New(orders, &cartRepoStub{})

			if err := operation(service); err == nil {
				t.Fatal("операция над чужим заказом должна быть запрещена")
			}
			if orders.changeCalls != 0 {
				t.Error("смена статуса не должна вызываться для чужого заказа")
			}
		})
	}
}

func TestRejectRequiresReason(t *testing.T) {
	t.Parallel()

	restaurantID := uuid.New()
	orders := &orderRepoStub{order: domain.Order{RestaurantID: restaurantID, Status: domain.OrderNew}}
	service := order.New(orders, &cartRepoStub{})

	_, err := service.Reject(context.Background(), restaurantID, uuid.New(), "")
	domainErr, ok := domain.AsError(err)
	if !ok || domainErr.Kind != domain.KindInvalid {
		t.Fatalf("отказ без причины должен отклоняться, получено %v", err)
	}
	if orders.changeCalls != 0 {
		t.Error("отказ без причины не должен менять статус")
	}
}

func TestAcceptPassesExpectedAt(t *testing.T) {
	t.Parallel()

	restaurantID := uuid.New()
	orders := &orderRepoStub{order: domain.Order{RestaurantID: restaurantID, Status: domain.OrderNew}}
	service := order.New(orders, &cartRepoStub{})

	expected := time.Now().Add(30 * time.Minute)
	if _, err := service.Accept(context.Background(), restaurantID, uuid.New(), &expected); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if orders.lastChange.ExpectedAt == nil || !orders.lastChange.ExpectedAt.Equal(expected) {
		t.Error("ожидаемое время готовности должно доходить до репозитория")
	}
	if orders.lastChange.Actor != domain.ActorRestaurant {
		t.Errorf("инициатор = %q, want restaurant", orders.lastChange.Actor)
	}
}

func TestListForUserFiltersByUser(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	orders := &orderRepoStub{order: domain.Order{UserID: userID}}
	service := order.New(orders, &cartRepoStub{})

	list, err := service.ListForUser(context.Background(), userID, catalog.Page{Number: 2, Size: 5})
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if list.Total != 1 {
		t.Errorf("Total = %d, want 1", list.Total)
	}
	if orders.lastFilter.UserID == nil || *orders.lastFilter.UserID != userID {
		t.Error("в фильтр должен уходить идентификатор пользователя")
	}
	if orders.lastFilter.RestaurantID != nil {
		t.Error("клиентский список не должен фильтроваться по заведению")
	}
	if orders.lastFilter.Offset != 5 {
		t.Errorf("Offset = %d, want 5", orders.lastFilter.Offset)
	}
}

func TestListForRestaurantRejectsUnknownStatus(t *testing.T) {
	t.Parallel()

	orders := &orderRepoStub{}
	service := order.New(orders, &cartRepoStub{})
	unknown := domain.OrderStatus("frozen")

	_, err := service.ListForRestaurant(context.Background(), uuid.New(), order.PartnerFilter{Status: &unknown})
	domainErr, ok := domain.AsError(err)
	if !ok || domainErr.Kind != domain.KindInvalid {
		t.Fatalf("неизвестный статус должен отклоняться, получено %v", err)
	}
}

func TestListForRestaurantPassesSince(t *testing.T) {
	t.Parallel()

	restaurantID := uuid.New()
	orders := &orderRepoStub{order: domain.Order{RestaurantID: restaurantID}}
	service := order.New(orders, &cartRepoStub{})

	since := time.Now().Add(-time.Hour)
	status := domain.OrderNew
	if _, err := service.ListForRestaurant(context.Background(), restaurantID, order.PartnerFilter{
		Status: &status, Since: &since,
	}); err != nil {
		t.Fatalf("ListForRestaurant: %v", err)
	}

	if orders.lastFilter.RestaurantID == nil || *orders.lastFilter.RestaurantID != restaurantID {
		t.Error("в фильтр должен уходить идентификатор заведения")
	}
	if orders.lastFilter.Since == nil || !orders.lastFilter.Since.Equal(since) {
		t.Error("в фильтр должна уходить отсечка since")
	}
}
