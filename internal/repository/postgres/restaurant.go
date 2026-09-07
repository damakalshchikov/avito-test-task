package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
)

// уникальный индекс на restaurants.slug
const uniqueViolation = "23505"

// RestaurantRepo доступ к таблице restaurants
type RestaurantRepo struct {
	pool *pgxpool.Pool
}

// NewRestaurantRepo создаёт репозиторий заведений
func NewRestaurantRepo(pool *pgxpool.Pool) *RestaurantRepo {
	return &RestaurantRepo{pool: pool}
}

const restaurantColumns = `id, slug, name, description, address, status, webhook_url,
	webhook_secret, delivery_fee, min_order_amount, created_at, updated_at`

func scanRestaurant(row pgx.Row) (domain.Restaurant, error) {
	var r domain.Restaurant
	err := row.Scan(&r.ID, &r.Slug, &r.Name, &r.Description, &r.Address, &r.Status,
		&r.WebhookURL, &r.WebhookSecret, &r.DeliveryFee, &r.MinOrderAmount, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

// List возвращает страницу заведений и общее количество подходящих по фильтру
func (r *RestaurantRepo) List(ctx context.Context, filter domain.RestaurantFilter) ([]domain.Restaurant, int, error) {
	var (
		conditions []string
		args       []any
	)
	if len(filter.Statuses) > 0 {
		args = append(args, filter.Statuses)
		conditions = append(conditions, fmt.Sprintf("status = ANY($%d)", len(args)))
	}
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		conditions = append(conditions, fmt.Sprintf("name ILIKE $%d", len(args)))
	}

	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	var total int
	if err := r.pool.QueryRow(ctx, "SELECT count(*) FROM restaurants"+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count restaurants: %w", err)
	}
	if total == 0 {
		return nil, 0, nil
	}

	args = append(args, filter.Limit, filter.Offset)
	query := fmt.Sprintf("SELECT %s FROM restaurants%s ORDER BY name, id LIMIT $%d OFFSET $%d",
		restaurantColumns, where, len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list restaurants: %w", err)
	}
	defer rows.Close()

	restaurants := make([]domain.Restaurant, 0, filter.Limit)
	for rows.Next() {
		restaurant, err := scanRestaurant(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan restaurant: %w", err)
		}
		restaurants = append(restaurants, restaurant)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate restaurants: %w", err)
	}
	return restaurants, total, nil
}

// GetByID возвращает заведение по идентификатору
func (r *RestaurantRepo) GetByID(ctx context.Context, id uuid.UUID) (domain.Restaurant, error) {
	query := "SELECT " + restaurantColumns + " FROM restaurants WHERE id = $1"
	restaurant, err := scanRestaurant(r.pool.QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Restaurant{}, domain.ErrRestaurantNotFound
	}
	if err != nil {
		return domain.Restaurant{}, fmt.Errorf("get restaurant: %w", err)
	}
	return restaurant, nil
}

// GetByAPIKeyHash находит заведение по sha256-хешу API-ключа
func (r *RestaurantRepo) GetByAPIKeyHash(ctx context.Context, hash []byte) (domain.Restaurant, error) {
	query := "SELECT " + restaurantColumns + " FROM restaurants WHERE api_key_hash = $1"
	restaurant, err := scanRestaurant(r.pool.QueryRow(ctx, query, hash))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Restaurant{}, domain.ErrUnauthorized
	}
	if err != nil {
		return domain.Restaurant{}, fmt.Errorf("get restaurant by api key: %w", err)
	}
	return restaurant, nil
}

// Create добавляет заведение
func (r *RestaurantRepo) Create(ctx context.Context, restaurant domain.Restaurant, apiKeyHash []byte) (domain.Restaurant, error) {
	query := `INSERT INTO restaurants (slug, name, description, address, status, api_key_hash,
			webhook_secret, delivery_fee, min_order_amount)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING ` + restaurantColumns

	created, err := scanRestaurant(r.pool.QueryRow(ctx, query,
		restaurant.Slug, restaurant.Name, restaurant.Description, restaurant.Address,
		restaurant.Status, apiKeyHash, restaurant.WebhookSecret,
		restaurant.DeliveryFee, restaurant.MinOrderAmount))

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return domain.Restaurant{}, domain.ErrSlugTaken.Wrap(err)
	}
	if err != nil {
		return domain.Restaurant{}, fmt.Errorf("create restaurant: %w", err)
	}
	return created, nil
}

// Update применяет частичное обновление профиля заведения
func (r *RestaurantRepo) Update(ctx context.Context, id uuid.UUID, update domain.RestaurantUpdate) (domain.Restaurant, error) {
	var (
		assignments []string
		args        []any
	)
	addAssignment := func(column string, value any) {
		args = append(args, value)
		assignments = append(assignments, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	if update.Status != nil {
		addAssignment("status", *update.Status)
	}
	if update.WebhookURL != nil {
		addAssignment("webhook_url", *update.WebhookURL)
	}
	if update.DeliveryFee != nil {
		addAssignment("delivery_fee", *update.DeliveryFee)
	}
	if update.MinOrderAmount != nil {
		addAssignment("min_order_amount", *update.MinOrderAmount)
	}
	if update.Description != nil {
		addAssignment("description", *update.Description)
	}
	if len(assignments) == 0 {
		return r.GetByID(ctx, id)
	}
	assignments = append(assignments, "updated_at = now()")

	args = append(args, id)
	query := fmt.Sprintf("UPDATE restaurants SET %s WHERE id = $%d RETURNING %s",
		strings.Join(assignments, ", "), len(args), restaurantColumns)

	restaurant, err := scanRestaurant(r.pool.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Restaurant{}, domain.ErrRestaurantNotFound
	}
	if err != nil {
		return domain.Restaurant{}, fmt.Errorf("update restaurant: %w", err)
	}
	return restaurant, nil
}
