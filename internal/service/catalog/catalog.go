// Package catalog реализует клиентские сценарии каталога
package catalog

import (
	"context"

	"github.com/google/uuid"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
)

// Границы постраничной выдачи
const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// RestaurantRepository доступ к заведениям, нужный каталогу
type RestaurantRepository interface {
	List(ctx context.Context, filter domain.RestaurantFilter) ([]domain.Restaurant, int, error)
	GetByID(ctx context.Context, id uuid.UUID) (domain.Restaurant, error)
}

// MenuRepository доступ к меню, нужный каталогу
type MenuRepository interface {
	GetMenu(ctx context.Context, restaurantID uuid.UUID) ([]domain.MenuCategory, error)
}

// Service сценарии каталога
type Service struct {
	restaurants RestaurantRepository
	menu        MenuRepository
}

// New создаёт сервис каталога
func New(restaurants RestaurantRepository, menu MenuRepository) *Service {
	return &Service{restaurants: restaurants, menu: menu}
}

// Page параметры постраничной выдачи, пришедшие из запроса
type Page struct {
	Number int
	Size   int
}

// Normalize приводит номер страницы и размер к допустимым значениям
func (p Page) Normalize() Page {
	if p.Number < 1 {
		p.Number = 1
	}
	switch {
	case p.Size < 1:
		p.Size = defaultPageSize
	case p.Size > maxPageSize:
		p.Size = maxPageSize
	}
	return p
}

func (p Page) Offset() int { return (p.Number - 1) * p.Size }

// RestaurantList страница каталога заведений
type RestaurantList struct {
	Items []domain.Restaurant
	Page  Page
	Total int
}

// ListRestaurants возвращает страницу активных заведений с поиском по названию
func (s *Service) ListRestaurants(ctx context.Context, search string, page Page) (RestaurantList, error) {
	page = page.Normalize()

	restaurants, total, err := s.restaurants.List(ctx, domain.RestaurantFilter{
		Search:   search,
		Statuses: []domain.RestaurantStatus{domain.RestaurantActive, domain.RestaurantPaused},
		Limit:    page.Size,
		Offset:   page.Offset(),
	})
	if err != nil {
		return RestaurantList{}, err
	}
	return RestaurantList{Items: restaurants, Page: page, Total: total}, nil
}

// GetRestaurant возвращает карточку заведения, скрывая отключённые платформой
func (s *Service) GetRestaurant(ctx context.Context, id uuid.UUID) (domain.Restaurant, error) {
	restaurant, err := s.restaurants.GetByID(ctx, id)
	if err != nil {
		return domain.Restaurant{}, err
	}
	if restaurant.Status == domain.RestaurantDisabled {
		return domain.Restaurant{}, domain.ErrRestaurantNotFound
	}
	return restaurant, nil
}

// GetMenu возвращает меню заведения
func (s *Service) GetMenu(ctx context.Context, restaurantID uuid.UUID) ([]domain.MenuCategory, error) {
	if _, err := s.GetRestaurant(ctx, restaurantID); err != nil {
		return nil, err
	}
	return s.menu.GetMenu(ctx, restaurantID)
}
