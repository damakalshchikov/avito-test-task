package catalog_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/service/catalog"
)

type restaurantRepoStub struct {
	restaurant domain.Restaurant
	err        error
	lastFilter domain.RestaurantFilter
}

func (s *restaurantRepoStub) List(_ context.Context, filter domain.RestaurantFilter) ([]domain.Restaurant, int, error) {
	s.lastFilter = filter
	if s.err != nil {
		return nil, 0, s.err
	}
	return []domain.Restaurant{s.restaurant}, 1, nil
}

func (s *restaurantRepoStub) GetByID(_ context.Context, _ uuid.UUID) (domain.Restaurant, error) {
	if s.err != nil {
		return domain.Restaurant{}, s.err
	}
	return s.restaurant, nil
}

type menuRepoStub struct {
	categories []domain.MenuCategory
	called     bool
}

func (s *menuRepoStub) GetMenu(_ context.Context, _ uuid.UUID) ([]domain.MenuCategory, error) {
	s.called = true
	return s.categories, nil
}

func TestPageNormalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		in         catalog.Page
		wantNumber int
		wantSize   int
		wantOffset int
	}{
		{"пустая страница получает значения по умолчанию", catalog.Page{}, 1, 20, 0},
		{"нулевой номер приводится к первой", catalog.Page{Number: 0, Size: 10}, 1, 10, 0},
		{"размер сверх максимума обрезается", catalog.Page{Number: 2, Size: 500}, 2, 100, 100},
		{"вторая страница даёт смещение", catalog.Page{Number: 3, Size: 20}, 3, 20, 40},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.in.Normalize()
			if got.Number != tt.wantNumber || got.Size != tt.wantSize {
				t.Errorf("Normalize() = %+v, want number=%d size=%d", got, tt.wantNumber, tt.wantSize)
			}
			if got.Offset() != tt.wantOffset {
				t.Errorf("Offset() = %d, want %d", got.Offset(), tt.wantOffset)
			}
		})
	}
}

func TestListRestaurantsHidesDisabled(t *testing.T) {
	t.Parallel()

	repo := &restaurantRepoStub{restaurant: domain.Restaurant{ID: uuid.New(), Status: domain.RestaurantActive}}
	service := catalog.New(repo, &menuRepoStub{})

	list, err := service.ListRestaurants(context.Background(), "борщ", catalog.Page{Number: 2, Size: 5})
	if err != nil {
		t.Fatalf("ListRestaurants: %v", err)
	}

	if list.Total != 1 || len(list.Items) != 1 {
		t.Errorf("получено %d записей, total=%d; want 1 и 1", len(list.Items), list.Total)
	}
	if repo.lastFilter.Search != "борщ" {
		t.Errorf("Search = %q, want «борщ»", repo.lastFilter.Search)
	}
	if repo.lastFilter.Limit != 5 || repo.lastFilter.Offset != 5 {
		t.Errorf("Limit/Offset = %d/%d, want 5/5", repo.lastFilter.Limit, repo.lastFilter.Offset)
	}

	want := map[domain.RestaurantStatus]bool{domain.RestaurantActive: true, domain.RestaurantPaused: true}
	if len(repo.lastFilter.Statuses) != len(want) {
		t.Fatalf("статусов в фильтре %d, want %d", len(repo.lastFilter.Statuses), len(want))
	}
	for _, status := range repo.lastFilter.Statuses {
		if !want[status] {
			t.Errorf("в фильтре неожиданный статус %q", status)
		}
	}
}

func TestGetRestaurantDisabledIsNotFound(t *testing.T) {
	t.Parallel()

	repo := &restaurantRepoStub{restaurant: domain.Restaurant{ID: uuid.New(), Status: domain.RestaurantDisabled}}
	service := catalog.New(repo, &menuRepoStub{})

	_, err := service.GetRestaurant(context.Background(), uuid.New())
	domainErr, ok := domain.AsError(err)
	if !ok {
		t.Fatalf("ожидалась доменная ошибка, получено %v", err)
	}
	if domainErr.Kind != domain.KindNotFound {
		t.Errorf("Kind = %v, want KindNotFound", domainErr.Kind)
	}
}

func TestGetMenuPausedRestaurantIsVisible(t *testing.T) {
	t.Parallel()

	repo := &restaurantRepoStub{restaurant: domain.Restaurant{ID: uuid.New(), Status: domain.RestaurantPaused}}
	menu := &menuRepoStub{categories: []domain.MenuCategory{{Name: "Супы"}}}
	service := catalog.New(repo, menu)

	categories, err := service.GetMenu(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("GetMenu: %v", err)
	}
	if !menu.called {
		t.Error("меню должно запрашиваться и для заведения на паузе")
	}
	if len(categories) != 1 {
		t.Errorf("получено %d категорий, want 1", len(categories))
	}
}

func TestGetMenuDisabledRestaurantIsNotFound(t *testing.T) {
	t.Parallel()

	repo := &restaurantRepoStub{restaurant: domain.Restaurant{ID: uuid.New(), Status: domain.RestaurantDisabled}}
	menu := &menuRepoStub{}
	service := catalog.New(repo, menu)

	if _, err := service.GetMenu(context.Background(), uuid.New()); err == nil {
		t.Fatal("ожидалась ошибка «не найдено»")
	}
	if menu.called {
		t.Error("меню отключённого заведения запрашиваться не должно")
	}
}
