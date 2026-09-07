package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
)

// MenuRepo доступ к таблицам menu_categories и menu_items
type MenuRepo struct {
	pool *pgxpool.Pool
}

// NewMenuRepo создаёт репозиторий меню
func NewMenuRepo(pool *pgxpool.Pool) *MenuRepo {
	return &MenuRepo{pool: pool}
}

const menuItemColumns = `id, category_id, external_id, name, description, price,
	is_available, stock_qty, image_url, created_at, updated_at`

func scanMenuItem(row pgx.Row) (domain.MenuItem, error) {
	var item domain.MenuItem
	err := row.Scan(&item.ID, &item.CategoryID, &item.ExternalID, &item.Name, &item.Description,
		&item.Price, &item.IsAvailable, &item.StockQty, &item.ImageURL, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

// GetMenu возвращает категории заведения вместе с позициями, отсортированные по sort_order
func (r *MenuRepo) GetMenu(ctx context.Context, restaurantID uuid.UUID) ([]domain.MenuCategory, error) {
	categoryRows, err := r.pool.Query(ctx,
		`SELECT id, name, sort_order FROM menu_categories
		 WHERE restaurant_id = $1 ORDER BY sort_order, name`, restaurantID)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer categoryRows.Close()

	var categories []domain.MenuCategory
	positions := make(map[uuid.UUID]int)
	for categoryRows.Next() {
		var category domain.MenuCategory
		if err := categoryRows.Scan(&category.ID, &category.Name, &category.SortOrder); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		positions[category.ID] = len(categories)
		categories = append(categories, category)
	}
	if err := categoryRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate categories: %w", err)
	}
	if len(categories) == 0 {
		return nil, nil
	}

	itemRows, err := r.pool.Query(ctx,
		"SELECT "+menuItemColumns+" FROM menu_items WHERE restaurant_id = $1 ORDER BY name", restaurantID)
	if err != nil {
		return nil, fmt.Errorf("list menu items: %w", err)
	}
	defer itemRows.Close()

	for itemRows.Next() {
		item, err := scanMenuItem(itemRows)
		if err != nil {
			return nil, fmt.Errorf("scan menu item: %w", err)
		}
		if position, ok := positions[item.CategoryID]; ok {
			categories[position].Items = append(categories[position].Items, item)
		}
	}
	if err := itemRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate menu items: %w", err)
	}
	return categories, nil
}

// GetItemByExternalID возвращает позицию заведения по её внешнему идентификатору
func (r *MenuRepo) GetItemByExternalID(ctx context.Context, restaurantID uuid.UUID, externalID string) (domain.MenuItem, error) {
	query := "SELECT " + menuItemColumns + " FROM menu_items WHERE restaurant_id = $1 AND external_id = $2"
	item, err := scanMenuItem(r.pool.QueryRow(ctx, query, restaurantID, externalID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MenuItem{}, domain.ErrMenuItemNotFound
	}
	if err != nil {
		return domain.MenuItem{}, fmt.Errorf("get menu item: %w", err)
	}
	return item, nil
}

// UpdateItem применяет точечное изменение позиции заведением
func (r *MenuRepo) UpdateItem(ctx context.Context, restaurantID uuid.UUID, externalID string,
	update domain.MenuItemUpdate,
) (domain.MenuItem, error) {
	var (
		assignments []string
		args        []any
	)
	addAssignment := func(column string, value any) {
		args = append(args, value)
		assignments = append(assignments, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	if update.IsAvailable != nil {
		addAssignment("is_available", *update.IsAvailable)
	}
	if update.StockQty != nil {
		addAssignment("stock_qty", *update.StockQty)
	}
	if update.Price != nil {
		addAssignment("price", *update.Price)
	}
	if len(assignments) == 0 {
		return r.GetItemByExternalID(ctx, restaurantID, externalID)
	}
	assignments = append(assignments, "updated_at = now()")

	args = append(args, restaurantID, externalID)
	query := fmt.Sprintf(
		"UPDATE menu_items SET %s WHERE restaurant_id = $%d AND external_id = $%d RETURNING %s",
		strings.Join(assignments, ", "), len(args)-1, len(args), menuItemColumns)

	item, err := scanMenuItem(r.pool.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MenuItem{}, domain.ErrMenuItemNotFound
	}
	if err != nil {
		return domain.MenuItem{}, fmt.Errorf("update menu item: %w", err)
	}
	return item, nil
}

// Sync применяет присланное заведением меню целиком
func (r *MenuRepo) Sync(ctx context.Context, restaurantID uuid.UUID, menu domain.MenuSync) (domain.MenuSyncResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.MenuSyncResult{}, fmt.Errorf("begin sync tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	result := domain.MenuSyncResult{CategoriesTotal: len(menu.Categories)}
	presentExternalIDs := make([]string, 0)

	for _, category := range menu.Categories {
		var categoryID uuid.UUID
		err := tx.QueryRow(ctx,
			`INSERT INTO menu_categories (restaurant_id, name, sort_order)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (restaurant_id, name)
			 DO UPDATE SET sort_order = EXCLUDED.sort_order, updated_at = now()
			 RETURNING id`,
			restaurantID, category.Name, category.SortOrder).Scan(&categoryID)
		if err != nil {
			return domain.MenuSyncResult{}, fmt.Errorf("upsert category %q: %w", category.Name, err)
		}

		for _, item := range category.Items {
			var inserted bool
			err := tx.QueryRow(ctx,
				`INSERT INTO menu_items (restaurant_id, category_id, external_id, name, description,
					price, is_available, stock_qty, image_url)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
				 ON CONFLICT (restaurant_id, external_id) DO UPDATE SET
					category_id = EXCLUDED.category_id,
					name = EXCLUDED.name,
					description = EXCLUDED.description,
					price = EXCLUDED.price,
					is_available = EXCLUDED.is_available,
					stock_qty = EXCLUDED.stock_qty,
					image_url = EXCLUDED.image_url,
					updated_at = now()
				 RETURNING (xmax = 0) AS inserted`,
				restaurantID, categoryID, item.ExternalID, item.Name, item.Description,
				item.Price, item.IsAvailable, item.StockQty, item.ImageURL).Scan(&inserted)
			if err != nil {
				return domain.MenuSyncResult{}, fmt.Errorf("upsert menu item %q: %w", item.ExternalID, err)
			}

			if inserted {
				result.ItemsCreated++
			} else {
				result.ItemsUpdated++
			}
			presentExternalIDs = append(presentExternalIDs, item.ExternalID)
		}
	}

	tag, err := tx.Exec(ctx,
		`UPDATE menu_items SET is_available = false, updated_at = now()
		 WHERE restaurant_id = $1 AND is_available AND external_id <> ALL($2)`,
		restaurantID, presentExternalIDs)
	if err != nil {
		return domain.MenuSyncResult{}, fmt.Errorf("disable missing menu items: %w", err)
	}
	result.ItemsDisabled = int(tag.RowsAffected())

	if err := tx.Commit(ctx); err != nil {
		return domain.MenuSyncResult{}, fmt.Errorf("commit sync tx: %w", err)
	}
	return result, nil
}
