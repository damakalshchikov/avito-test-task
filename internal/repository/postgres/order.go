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

// OrderRepo доступ к заказам, их позициям и истории статусов
type OrderRepo struct {
	pool *pgxpool.Pool
}

// NewOrderRepo создаёт репозиторий заказов
func NewOrderRepo(pool *pgxpool.Pool) *OrderRepo {
	return &OrderRepo{pool: pool}
}

const orderColumns = `o.id, o.number, o.user_id, o.restaurant_id, o.status, o.delivery_address,
	o.comment, o.subtotal, o.delivery_fee, o.total, o.reject_reason, o.expected_at,
	o.created_at, o.updated_at, r.name, u.name, u.phone`

func scanOrder(row pgx.Row) (domain.Order, error) {
	var o domain.Order
	err := row.Scan(&o.ID, &o.Number, &o.UserID, &o.RestaurantID, &o.Status, &o.DeliveryAddress,
		&o.Comment, &o.Subtotal, &o.DeliveryFee, &o.Total, &o.RejectReason, &o.ExpectedAt,
		&o.CreatedAt, &o.UpdatedAt, &o.RestaurantName, &o.Customer.Name, &o.Customer.Phone)
	return o, err
}

const orderFrom = ` FROM orders o
	JOIN restaurants r ON r.id = o.restaurant_id
	JOIN users u ON u.id = o.user_id`

// Create оформляет заказ из корзины в одной транзакции
func (r *OrderRepo) Create(ctx context.Context, cart domain.Cart, input domain.CreateOrder) (domain.Order, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Order{}, fmt.Errorf("begin order tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if input.IdempotencyKey != "" {
		existing, err := findByIdempotencyKey(ctx, tx, input.UserID, input.IdempotencyKey)
		if err != nil {
			return domain.Order{}, err
		}
		if existing != nil {
			return *existing, nil
		}
	}

	var (
		restaurantStatus domain.RestaurantStatus
		deliveryFee      int64
		minOrderAmount   int64
	)
	err = tx.QueryRow(ctx,
		"SELECT status, delivery_fee, min_order_amount FROM restaurants WHERE id = $1 FOR UPDATE",
		cart.RestaurantID).Scan(&restaurantStatus, &deliveryFee, &minOrderAmount)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, domain.ErrRestaurantNotFound
	}
	if err != nil {
		return domain.Order{}, fmt.Errorf("lock restaurant: %w", err)
	}
	if !restaurantStatus.CanAcceptOrders() {
		return domain.Order{}, domain.ErrRestaurantUnavailable
	}

	locked, err := lockMenuItems(ctx, tx, cart)
	if err != nil {
		return domain.Order{}, err
	}

	// Состояние позиций могло измениться с момента добавления в корзину
	// Поэтому доступность проверяется заново уже под блокировкой
	unavailable := locked.UnavailableItems()
	if len(unavailable) > 0 {
		return domain.Order{}, domain.ErrItemsUnavailable.WithDetails(domain.UnavailableDetails(unavailable)...)
	}

	subtotal := locked.Subtotal()
	if subtotal < minOrderAmount {
		return domain.Order{}, domain.ErrMinOrderAmount.WithDetails(domain.Detail{
			Code:    domain.CodeMinOrderAmount,
			Message: fmt.Sprintf("Минимальная сумма заказа %d, в корзине %d", minOrderAmount, subtotal),
		})
	}

	if err := decrementStock(ctx, tx, locked); err != nil {
		return domain.Order{}, err
	}

	order, err := insertOrder(ctx, tx, locked, input, subtotal, deliveryFee)
	if err != nil {
		return domain.Order{}, err
	}

	// Событие пишется в той же транзакции, что и заказ
	if err := enqueueEvent(ctx, tx, domain.WebhookOrderCreated, order); err != nil {
		return domain.Order{}, err
	}

	if _, err := tx.Exec(ctx, "DELETE FROM carts WHERE id = $1", cart.ID); err != nil {
		return domain.Order{}, fmt.Errorf("clear cart: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Order{}, fmt.Errorf("commit order tx: %w", err)
	}
	return order, nil
}

// lockMenuItems блокирует позиции корзины и возвращает их актуальное состояние
func lockMenuItems(ctx context.Context, tx pgx.Tx, cart domain.Cart) (domain.Cart, error) {
	ids := make([]uuid.UUID, 0, len(cart.Items))
	quantities := make(map[uuid.UUID]int, len(cart.Items))
	for _, item := range cart.Items {
		ids = append(ids, item.MenuItem.ID)
		quantities[item.MenuItem.ID] = item.Quantity
	}

	rows, err := tx.Query(ctx,
		"SELECT "+menuItemColumns+" FROM menu_items WHERE id = ANY($1) ORDER BY id FOR UPDATE", ids)
	if err != nil {
		return domain.Cart{}, fmt.Errorf("lock menu items: %w", err)
	}
	defer rows.Close()

	locked := domain.Cart{ID: cart.ID, UserID: cart.UserID, RestaurantID: cart.RestaurantID}
	for rows.Next() {
		item, err := scanMenuItem(rows)
		if err != nil {
			return domain.Cart{}, fmt.Errorf("scan locked menu item: %w", err)
		}
		locked.Items = append(locked.Items, domain.CartItem{MenuItem: item, Quantity: quantities[item.ID]})
	}
	if err := rows.Err(); err != nil {
		return domain.Cart{}, fmt.Errorf("iterate locked menu items: %w", err)
	}
	if len(locked.Items) != len(cart.Items) {
		return domain.Cart{}, domain.ErrMenuItemNotFound
	}
	return locked, nil
}

func decrementStock(ctx context.Context, tx pgx.Tx, cart domain.Cart) error {
	for _, item := range cart.Items {
		if item.MenuItem.StockQty == nil {
			continue
		}
		if _, err := tx.Exec(ctx,
			"UPDATE menu_items SET stock_qty = stock_qty - $1, updated_at = now() WHERE id = $2",
			item.Quantity, item.MenuItem.ID); err != nil {
			return fmt.Errorf("decrement stock: %w", err)
		}
	}
	return nil
}

func insertOrder(ctx context.Context, tx pgx.Tx, cart domain.Cart, input domain.CreateOrder,
	subtotal, deliveryFee int64,
) (domain.Order, error) {
	var idempotencyKey *string
	if input.IdempotencyKey != "" {
		idempotencyKey = &input.IdempotencyKey
	}

	var orderID uuid.UUID
	err := tx.QueryRow(ctx,
		`INSERT INTO orders (user_id, restaurant_id, status, delivery_address, comment,
			subtotal, delivery_fee, total, idempotency_key)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		input.UserID, cart.RestaurantID, domain.OrderNew, input.DeliveryAddress, input.Comment,
		subtotal, deliveryFee, subtotal+deliveryFee, idempotencyKey).Scan(&orderID)
	if err != nil {
		return domain.Order{}, fmt.Errorf("insert order: %w", err)
	}

	for _, item := range cart.Items {
		if _, err := tx.Exec(ctx,
			`INSERT INTO order_items (order_id, menu_item_id, external_id, name, price, quantity)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			orderID, item.MenuItem.ID, item.MenuItem.ExternalID,
			item.MenuItem.Name, item.MenuItem.Price, item.Quantity); err != nil {
			return domain.Order{}, fmt.Errorf("insert order item: %w", err)
		}
	}

	if err := appendHistory(ctx, tx, orderID, nil, domain.OrderNew, domain.ActorUser, ""); err != nil {
		return domain.Order{}, err
	}
	return getOrder(ctx, tx, orderID)
}

func appendHistory(ctx context.Context, tx pgx.Tx, orderID uuid.UUID,
	from *domain.OrderStatus, to domain.OrderStatus, actor domain.Actor, reason string,
) error {
	var reasonValue *string
	if reason != "" {
		reasonValue = &reason
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO order_status_history (order_id, from_status, to_status, actor, reason)
		 VALUES ($1, $2, $3, $4, $5)`,
		orderID, from, to, actor, reasonValue); err != nil {
		return fmt.Errorf("insert status history: %w", err)
	}
	return nil
}

func findByIdempotencyKey(ctx context.Context, q querier, userID uuid.UUID, key string) (*domain.Order, error) {
	var orderID uuid.UUID
	err := q.QueryRow(ctx,
		"SELECT id FROM orders WHERE user_id = $1 AND idempotency_key = $2", userID, key).Scan(&orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup idempotency key: %w", err)
	}

	order, err := getOrder(ctx, q, orderID)
	if err != nil {
		return nil, err
	}
	return &order, nil
}

// FindByIdempotencyKey ищет ранее созданный заказ по ключу идемпотентности
func (r *OrderRepo) FindByIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) (*domain.Order, error) {
	return findByIdempotencyKey(ctx, r.pool, userID, key)
}

// Get возвращает заказ с позициями и историей статусов
func (r *OrderRepo) Get(ctx context.Context, id uuid.UUID) (domain.Order, error) {
	return getOrder(ctx, r.pool, id)
}

func getOrder(ctx context.Context, q querier, id uuid.UUID) (domain.Order, error) {
	order, err := scanOrder(q.QueryRow(ctx, "SELECT "+orderColumns+orderFrom+" WHERE o.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	if err != nil {
		return domain.Order{}, fmt.Errorf("get order: %w", err)
	}

	if order.Items, err = getOrderItems(ctx, q, id); err != nil {
		return domain.Order{}, err
	}
	if order.History, err = getOrderHistory(ctx, q, id); err != nil {
		return domain.Order{}, err
	}
	return order, nil
}

func getOrderItems(ctx context.Context, q querier, orderID uuid.UUID) ([]domain.OrderItem, error) {
	rows, err := q.Query(ctx,
		`SELECT id, menu_item_id, external_id, name, price, quantity
		 FROM order_items WHERE order_id = $1 ORDER BY name`, orderID)
	if err != nil {
		return nil, fmt.Errorf("list order items: %w", err)
	}
	defer rows.Close()

	var items []domain.OrderItem
	for rows.Next() {
		var item domain.OrderItem
		if err := rows.Scan(&item.ID, &item.MenuItemID, &item.ExternalID,
			&item.Name, &item.Price, &item.Quantity); err != nil {
			return nil, fmt.Errorf("scan order item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate order items: %w", err)
	}
	return items, nil
}

func getOrderHistory(ctx context.Context, q querier, orderID uuid.UUID) ([]domain.OrderStatusChange, error) {
	rows, err := q.Query(ctx,
		`SELECT from_status, to_status, actor, reason, created_at
		 FROM order_status_history WHERE order_id = $1 ORDER BY created_at, id`, orderID)
	if err != nil {
		return nil, fmt.Errorf("list status history: %w", err)
	}
	defer rows.Close()

	var history []domain.OrderStatusChange
	for rows.Next() {
		var (
			change domain.OrderStatusChange
			reason *string
		)
		if err := rows.Scan(&change.FromStatus, &change.ToStatus,
			&change.Actor, &reason, &change.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan status change: %w", err)
		}
		if reason != nil {
			change.Reason = *reason
		}
		history = append(history, change)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate status history: %w", err)
	}
	return history, nil
}

// List возвращает страницу заказов по фильтру и общее количество
func (r *OrderRepo) List(ctx context.Context, filter domain.OrderFilter) ([]domain.Order, int, error) {
	var (
		conditions []string
		args       []any
	)
	addCondition := func(format string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(format, len(args)))
	}

	if filter.UserID != nil {
		addCondition("o.user_id = $%d", *filter.UserID)
	}
	if filter.RestaurantID != nil {
		addCondition("o.restaurant_id = $%d", *filter.RestaurantID)
	}
	if filter.Status != nil {
		addCondition("o.status = $%d", *filter.Status)
	}
	if filter.Since != nil {
		addCondition("o.updated_at > $%d", *filter.Since)
	}

	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	var total int
	if err := r.pool.QueryRow(ctx, "SELECT count(*) FROM orders o"+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count orders: %w", err)
	}
	if total == 0 {
		return nil, 0, nil
	}

	args = append(args, filter.Limit, filter.Offset)
	query := fmt.Sprintf("SELECT %s%s%s ORDER BY o.created_at DESC, o.id LIMIT $%d OFFSET $%d",
		orderColumns, orderFrom, where, len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list orders: %w", err)
	}
	defer rows.Close()

	orders := make([]domain.Order, 0, filter.Limit)
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan order: %w", err)
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate orders: %w", err)
	}

	for i := range orders {
		if orders[i].Items, err = getOrderItems(ctx, r.pool, orders[i].ID); err != nil {
			return nil, 0, err
		}
	}
	return orders, total, nil
}

// ChangeStatus переводит заказ в новый статус в одной транзакции
func (r *OrderRepo) ChangeStatus(ctx context.Context, id uuid.UUID, change domain.StatusChange) (domain.Order, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Order{}, fmt.Errorf("begin status tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current domain.OrderStatus
	err = tx.QueryRow(ctx, "SELECT status FROM orders WHERE id = $1 FOR UPDATE", id).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	if err != nil {
		return domain.Order{}, fmt.Errorf("lock order: %w", err)
	}

	if err := domain.CanTransition(current, change.To, change.Actor); err != nil {
		return domain.Order{}, err
	}

	var rejectReason *string
	if change.To == domain.OrderRejected && change.Reason != "" {
		rejectReason = &change.Reason
	}
	if _, err := tx.Exec(ctx,
		`UPDATE orders SET status = $1, updated_at = now(),
			reject_reason = COALESCE($2, reject_reason),
			expected_at = COALESCE($3, expected_at)
		 WHERE id = $4`,
		change.To, rejectReason, change.ExpectedAt, id); err != nil {
		return domain.Order{}, fmt.Errorf("update order status: %w", err)
	}

	if change.To.ReturnsStock() {
		if err := returnStock(ctx, tx, id); err != nil {
			return domain.Order{}, err
		}
	}

	if err := appendHistory(ctx, tx, id, &current, change.To, change.Actor, change.Reason); err != nil {
		return domain.Order{}, err
	}

	order, err := getOrder(ctx, tx, id)
	if err != nil {
		return domain.Order{}, err
	}

	// Об отмене заведение узнаёт из webhook-а
	// Об отказе не уведомляем, его инициирует само заведение
	if change.To == domain.OrderCancelled {
		if err := enqueueEvent(ctx, tx, domain.WebhookOrderCancelled, order); err != nil {
			return domain.Order{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Order{}, fmt.Errorf("commit status tx: %w", err)
	}
	return order, nil
}

// returnStock возвращает в меню остатки отменённого или отклонённого заказа
func returnStock(ctx context.Context, tx pgx.Tx, orderID uuid.UUID) error {
	if _, err := tx.Exec(ctx,
		`UPDATE menu_items mi SET stock_qty = mi.stock_qty + oi.quantity, updated_at = now()
		 FROM order_items oi
		 WHERE oi.order_id = $1 AND mi.id = oi.menu_item_id AND mi.stock_qty IS NOT NULL`,
		orderID); err != nil {
		return fmt.Errorf("return stock: %w", err)
	}
	return nil
}
