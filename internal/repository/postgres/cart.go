package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
)

// CartRepo доступ к таблицам carts и cart_items
type CartRepo struct {
	pool *pgxpool.Pool
}

// NewCartRepo создаёт репозиторий корзины
func NewCartRepo(pool *pgxpool.Pool) *CartRepo {
	return &CartRepo{pool: pool}
}

// Get возвращает корзину пользователя вместе с актуальным состоянием позиций
func (r *CartRepo) Get(ctx context.Context, userID uuid.UUID) (domain.Cart, error) {
	return getCart(ctx, r.pool, userID)
}

// SetItem задаёт количество позиции в корзине
func (r *CartRepo) SetItem(ctx context.Context, userID, menuItemID uuid.UUID, quantity int) (domain.Cart, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Cart{}, fmt.Errorf("begin cart tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var itemRestaurantID uuid.UUID
	err = tx.QueryRow(ctx,
		"SELECT restaurant_id FROM menu_items WHERE id = $1",
		menuItemID).Scan(&itemRestaurantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Cart{}, domain.ErrMenuItemNotFound
	}
	if err != nil {
		return domain.Cart{}, fmt.Errorf("lookup menu item: %w", err)
	}

	var cartID, cartRestaurantID uuid.UUID
	err = tx.QueryRow(ctx,
		"SELECT id, restaurant_id FROM carts WHERE user_id = $1 FOR UPDATE",
		userID).Scan(&cartID, &cartRestaurantID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if quantity == 0 {
			return domain.Cart{}, nil
		}
		if err := tx.QueryRow(ctx,
			"INSERT INTO carts (user_id, restaurant_id) VALUES ($1, $2) RETURNING id",
			userID, itemRestaurantID).Scan(&cartID); err != nil {
			return domain.Cart{}, fmt.Errorf("create cart: %w", err)
		}
	case err != nil:
		return domain.Cart{}, fmt.Errorf("lock cart: %w", err)
	case cartRestaurantID != itemRestaurantID:
		return domain.Cart{}, domain.ErrCartRestaurantMismatch
	}

	if quantity == 0 {
		_, err = tx.Exec(ctx,
			"DELETE FROM cart_items WHERE cart_id = $1 AND menu_item_id = $2", cartID, menuItemID)
	} else {
		_, err = tx.Exec(ctx,
			`INSERT INTO cart_items (cart_id, menu_item_id, quantity) VALUES ($1, $2, $3)
			 ON CONFLICT (cart_id, menu_item_id)
			 DO UPDATE SET quantity = EXCLUDED.quantity, updated_at = now()`,
			cartID, menuItemID, quantity)
	}
	if err != nil {
		return domain.Cart{}, fmt.Errorf("set cart item: %w", err)
	}

	// Корзина без позиций не хранится
	if _, err := tx.Exec(ctx,
		`DELETE FROM carts WHERE id = $1
		 AND NOT EXISTS (SELECT 1 FROM cart_items WHERE cart_id = $1)`, cartID); err != nil {
		return domain.Cart{}, fmt.Errorf("drop empty cart: %w", err)
	}

	cart, err := getCart(ctx, tx, userID)
	if err != nil {
		return domain.Cart{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Cart{}, fmt.Errorf("commit cart tx: %w", err)
	}
	return cart, nil
}

// Clear удаляет корзину пользователя
func (r *CartRepo) Clear(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, "DELETE FROM carts WHERE user_id = $1", userID); err != nil {
		return fmt.Errorf("clear cart: %w", err)
	}
	return nil
}

// querier общий интерфейс пула и транзакции, чтобы читать корзину
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func getCart(ctx context.Context, q querier, userID uuid.UUID) (domain.Cart, error) {
	var cart domain.Cart
	err := q.QueryRow(ctx,
		"SELECT id, user_id, restaurant_id FROM carts WHERE user_id = $1",
		userID).Scan(&cart.ID, &cart.UserID, &cart.RestaurantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Cart{}, nil
	}
	if err != nil {
		return domain.Cart{}, fmt.Errorf("get cart: %w", err)
	}

	rows, err := q.Query(ctx,
		`SELECT ci.quantity, `+prefixedMenuItemColumns+`
		 FROM cart_items ci
		 JOIN menu_items mi ON mi.id = ci.menu_item_id
		 WHERE ci.cart_id = $1
		 ORDER BY mi.name`, cart.ID)
	if err != nil {
		return domain.Cart{}, fmt.Errorf("list cart items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item domain.CartItem
		if err := rows.Scan(&item.Quantity, &item.MenuItem.ID, &item.MenuItem.CategoryID,
			&item.MenuItem.ExternalID, &item.MenuItem.Name, &item.MenuItem.Description,
			&item.MenuItem.Price, &item.MenuItem.IsAvailable, &item.MenuItem.StockQty,
			&item.MenuItem.ImageURL, &item.MenuItem.CreatedAt, &item.MenuItem.UpdatedAt); err != nil {
			return domain.Cart{}, fmt.Errorf("scan cart item: %w", err)
		}
		cart.Items = append(cart.Items, item)
	}
	if err := rows.Err(); err != nil {
		return domain.Cart{}, fmt.Errorf("iterate cart items: %w", err)
	}
	return cart, nil
}
