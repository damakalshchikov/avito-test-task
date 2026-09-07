package partner_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/service/partner"
)

type restaurantRepoStub struct {
	restaurant  domain.Restaurant
	err         error
	lastUpdate  domain.RestaurantUpdate
	lastKeyHash []byte
	created     bool
}

func (s *restaurantRepoStub) GetByID(_ context.Context, _ uuid.UUID) (domain.Restaurant, error) {
	return s.restaurant, s.err
}

func (s *restaurantRepoStub) GetByAPIKeyHash(_ context.Context, hash []byte) (domain.Restaurant, error) {
	s.lastKeyHash = hash
	return s.restaurant, s.err
}

func (s *restaurantRepoStub) Create(_ context.Context, restaurant domain.Restaurant, apiKeyHash []byte) (domain.Restaurant, error) {
	if s.err != nil {
		return domain.Restaurant{}, s.err
	}
	s.created = true
	s.lastKeyHash = apiKeyHash
	restaurant.ID = uuid.New()
	return restaurant, nil
}

func (s *restaurantRepoStub) Update(_ context.Context, _ uuid.UUID, update domain.RestaurantUpdate) (domain.Restaurant, error) {
	s.lastUpdate = update
	return s.restaurant, s.err
}

type menuRepoStub struct {
	syncCalled bool
	lastUpdate domain.MenuItemUpdate
}

func (s *menuRepoStub) Sync(_ context.Context, _ uuid.UUID, _ domain.MenuSync) (domain.MenuSyncResult, error) {
	s.syncCalled = true
	return domain.MenuSyncResult{CategoriesTotal: 1, ItemsCreated: 2}, nil
}

func (s *menuRepoStub) UpdateItem(_ context.Context, _ uuid.UUID, _ string, update domain.MenuItemUpdate) (domain.MenuItem, error) {
	s.lastUpdate = update
	return domain.MenuItem{ExternalID: "item-1"}, nil
}

func TestAuthenticate(t *testing.T) {
	t.Parallel()

	t.Run("ключ хешируется перед поиском", func(t *testing.T) {
		t.Parallel()

		repo := &restaurantRepoStub{restaurant: domain.Restaurant{Status: domain.RestaurantActive}}
		service := partner.New(repo, &menuRepoStub{})

		if _, err := service.Authenticate(context.Background(), "demo-restaurant-key"); err != nil {
			t.Fatalf("Authenticate: %v", err)
		}
		if !bytes.Equal(repo.lastKeyHash, domain.HashAPIKey("demo-restaurant-key")) {
			t.Error("в репозиторий должен уходить sha256-хеш ключа, а не сам ключ")
		}
	})

	t.Run("пустой ключ отклоняется без обращения к БД", func(t *testing.T) {
		t.Parallel()

		repo := &restaurantRepoStub{restaurant: domain.Restaurant{Status: domain.RestaurantActive}}
		service := partner.New(repo, &menuRepoStub{})

		_, err := service.Authenticate(context.Background(), "")
		assertKind(t, err, domain.KindUnauthorized)
		if repo.lastKeyHash != nil {
			t.Error("при пустом ключе поход в репозиторий не нужен")
		}
	})

	t.Run("отключённое заведение не аутентифицируется", func(t *testing.T) {
		t.Parallel()

		repo := &restaurantRepoStub{restaurant: domain.Restaurant{Status: domain.RestaurantDisabled}}
		service := partner.New(repo, &menuRepoStub{})

		_, err := service.Authenticate(context.Background(), "any-key")
		assertKind(t, err, domain.KindUnauthorized)
	})
}

func TestUpdateProfileRejectsDisabledStatus(t *testing.T) {
	t.Parallel()

	service := partner.New(&restaurantRepoStub{}, &menuRepoStub{})
	status := domain.RestaurantDisabled

	_, err := service.UpdateProfile(context.Background(), uuid.New(), domain.RestaurantUpdate{Status: &status})
	assertKind(t, err, domain.KindInvalid)
}

func TestUpdateProfileWebhookURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"абсолютный http", "http://restaurant:8090/webhooks/kitchen", false},
		{"абсолютный https", "https://example.com/hook", false},
		{"относительный путь", "/webhooks/kitchen", true},
		{"без схемы", "restaurant:8090", true},
		{"чужая схема", "ftp://example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service := partner.New(&restaurantRepoStub{}, &menuRepoStub{})
			webhookURL := &tt.url

			_, err := service.UpdateProfile(context.Background(), uuid.New(), domain.RestaurantUpdate{WebhookURL: &webhookURL})
			if tt.wantErr {
				assertKind(t, err, domain.KindInvalid)
				return
			}
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
		})
	}
}

func TestUpdateProfileAllowsDisablingWebhook(t *testing.T) {
	t.Parallel()

	repo := &restaurantRepoStub{}
	service := partner.New(repo, &menuRepoStub{})

	var none *string
	if _, err := service.UpdateProfile(context.Background(), uuid.New(), domain.RestaurantUpdate{WebhookURL: &none}); err != nil {
		t.Fatalf("сброс webhook_url должен быть разрешён: %v", err)
	}
	if repo.lastUpdate.WebhookURL == nil || *repo.lastUpdate.WebhookURL != nil {
		t.Error("в репозиторий должен уйти явный сброс webhook_url в null")
	}
}

func TestSyncMenuValidatesBeforeWrite(t *testing.T) {
	t.Parallel()

	menu := &menuRepoStub{}
	service := partner.New(&restaurantRepoStub{}, menu)

	_, err := service.SyncMenu(context.Background(), uuid.New(), domain.MenuSync{})
	assertKind(t, err, domain.KindInvalid)
	if menu.syncCalled {
		t.Error("некорректное меню не должно доходить до репозитория")
	}
}

func TestUpdateMenuItemRejectsNegativeValues(t *testing.T) {
	t.Parallel()

	service := partner.New(&restaurantRepoStub{}, &menuRepoStub{})
	price := int64(-1)

	_, err := service.UpdateMenuItem(context.Background(), uuid.New(), "item-1", domain.MenuItemUpdate{Price: &price})
	assertKind(t, err, domain.KindInvalid)

	negative := -5
	stock := &negative
	_, err = service.UpdateMenuItem(context.Background(), uuid.New(), "item-1", domain.MenuItemUpdate{StockQty: &stock})
	assertKind(t, err, domain.KindInvalid)
}

func TestCreateRestaurantIssuesCredentials(t *testing.T) {
	t.Parallel()

	repo := &restaurantRepoStub{}
	service := partner.New(repo, &menuRepoStub{})

	credentials, err := service.CreateRestaurant(context.Background(), domain.Restaurant{
		Slug: "demo-bistro", Name: "Demo Bistro", Address: "Москва",
	})
	if err != nil {
		t.Fatalf("CreateRestaurant: %v", err)
	}

	if credentials.APIKey == "" || credentials.WebhookSecret == "" {
		t.Fatal("должны выдаваться и API-ключ, и webhook-секрет")
	}
	if !bytes.Equal(repo.lastKeyHash, domain.HashAPIKey(credentials.APIKey)) {
		t.Error("в БД должен сохраняться хеш выданного ключа")
	}
	if credentials.Restaurant.Status != domain.RestaurantActive {
		t.Errorf("статус нового заведения = %q, want active", credentials.Restaurant.Status)
	}
	if credentials.Restaurant.WebhookSecret != credentials.WebhookSecret {
		t.Error("секрет заведения должен совпадать с выданным")
	}
}

func TestCreateRestaurantValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		restaurant domain.Restaurant
	}{
		{"пустой slug", domain.Restaurant{Name: "Demo", Address: "Москва"}},
		{"slug с заглавными", domain.Restaurant{Slug: "Demo", Name: "Demo", Address: "Москва"}},
		{"без названия", domain.Restaurant{Slug: "demo-bistro", Address: "Москва"}},
		{"без адреса", domain.Restaurant{Slug: "demo-bistro", Name: "Demo"}},
		{"отрицательная доставка", domain.Restaurant{Slug: "demo-bistro", Name: "Demo", Address: "Москва", DeliveryFee: -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &restaurantRepoStub{}
			service := partner.New(repo, &menuRepoStub{})

			_, err := service.CreateRestaurant(context.Background(), tt.restaurant)
			assertKind(t, err, domain.KindInvalid)
			if repo.created {
				t.Error("некорректное заведение не должно создаваться")
			}
		})
	}
}

func assertKind(t *testing.T, err error, want domain.Kind) {
	t.Helper()

	if err == nil {
		t.Fatalf("ожидалась ошибка вида %v", want)
	}
	domainErr, ok := domain.AsError(err)
	if !ok {
		t.Fatalf("ожидалась доменная ошибка, получено %T: %v", err, err)
	}
	if domainErr.Kind != want {
		t.Errorf("Kind = %v, want %v", domainErr.Kind, want)
	}
}
