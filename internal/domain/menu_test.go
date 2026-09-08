package domain_test

import (
	"testing"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
)

func TestMenuItemOrderable(t *testing.T) {
	t.Parallel()

	stock := func(n int) *int { return &n }

	tests := []struct {
		name string
		item domain.MenuItem
		qty  int
		want bool
	}{
		{"без учёта остатков доступна в любом количестве", domain.MenuItem{IsAvailable: true}, 100, true},
		{"снятая с продажи недоступна", domain.MenuItem{IsAvailable: false, StockQty: stock(10)}, 1, false},
		{"остатка хватает", domain.MenuItem{IsAvailable: true, StockQty: stock(3)}, 3, true},
		{"остатка не хватает", domain.MenuItem{IsAvailable: true, StockQty: stock(2)}, 3, false},
		{"нулевой остаток", domain.MenuItem{IsAvailable: true, StockQty: stock(0)}, 1, false},
		{"нулевое количество не заказывается", domain.MenuItem{IsAvailable: true}, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.item.Orderable(tt.qty); got != tt.want {
				t.Errorf("Orderable(%d) = %v, want %v", tt.qty, got, tt.want)
			}
		})
	}
}

func TestMenuItemAvailable(t *testing.T) {
	t.Parallel()

	qty := 5
	if got := (domain.MenuItem{StockQty: &qty}).Available(); got != 5 {
		t.Errorf("Available() = %d, want 5", got)
	}
	if got := (domain.MenuItem{}).Available(); got != -1 {
		t.Errorf("Available() без учёта остатков = %d, want -1", got)
	}
}

func TestMenuSyncValidate(t *testing.T) {
	t.Parallel()

	validItem := domain.MenuSyncItem{ExternalID: "item-1", Name: "Борщ", Price: 100}

	tests := []struct {
		name    string
		menu    domain.MenuSync
		wantErr bool
	}{
		{
			name: "корректное меню",
			menu: domain.MenuSync{Categories: []domain.MenuSyncCategory{
				{Name: "Супы", Items: []domain.MenuSyncItem{validItem}},
			}},
		},
		{
			name:    "пустое меню",
			menu:    domain.MenuSync{},
			wantErr: true,
		},
		{
			name: "категория без названия",
			menu: domain.MenuSync{Categories: []domain.MenuSyncCategory{
				{Name: "", Items: []domain.MenuSyncItem{validItem}},
			}},
			wantErr: true,
		},
		{
			name: "дубликат категории",
			menu: domain.MenuSync{Categories: []domain.MenuSyncCategory{
				{Name: "Супы", Items: []domain.MenuSyncItem{validItem}},
				{Name: "Супы"},
			}},
			wantErr: true,
		},
		{
			name: "дубликат external_id в разных категориях",
			menu: domain.MenuSync{Categories: []domain.MenuSyncCategory{
				{Name: "Супы", Items: []domain.MenuSyncItem{validItem}},
				{Name: "Горячее", Items: []domain.MenuSyncItem{validItem}},
			}},
			wantErr: true,
		},
		{
			name: "позиция без external_id",
			menu: domain.MenuSync{Categories: []domain.MenuSyncCategory{
				{Name: "Супы", Items: []domain.MenuSyncItem{{Name: "Борщ", Price: 100}}},
			}},
			wantErr: true,
		},
		{
			name: "отрицательная цена",
			menu: domain.MenuSync{Categories: []domain.MenuSyncCategory{
				{Name: "Супы", Items: []domain.MenuSyncItem{{ExternalID: "item-1", Name: "Борщ", Price: -1}}},
			}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.menu.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("ожидалась ошибка валидации")
				}
				domainErr, ok := domain.AsError(err)
				if !ok {
					t.Fatalf("ожидалась доменная ошибка, получено %T", err)
				}
				if domainErr.Kind != domain.KindInvalid {
					t.Errorf("Kind = %v, want KindInvalid", domainErr.Kind)
				}
				return
			}
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
		})
	}
}
