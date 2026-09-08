-- +goose Up
-- +goose StatementBegin

-- Заведения-партнёры платформы
CREATE TABLE restaurants (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug             text NOT NULL UNIQUE,
    name             text NOT NULL,
    description      text NOT NULL DEFAULT '',
    address          text NOT NULL,
    status           text NOT NULL DEFAULT 'active'
                     CHECK (status IN ('active', 'paused', 'disabled')),
    api_key_hash     bytea NOT NULL UNIQUE,
    webhook_url      text,
    webhook_secret   text NOT NULL,
    delivery_fee     bigint NOT NULL DEFAULT 0 CHECK (delivery_fee >= 0),
    min_order_amount bigint NOT NULL DEFAULT 0 CHECK (min_order_amount >= 0),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX restaurants_status_name_idx ON restaurants (status, name);

-- Категории меню заведения
CREATE TABLE menu_categories (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    name          text NOT NULL,
    sort_order    integer NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (restaurant_id, name)
);

-- Позиции меню
CREATE TABLE menu_items (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    category_id   uuid NOT NULL REFERENCES menu_categories (id) ON DELETE RESTRICT,
    external_id   text NOT NULL,
    name          text NOT NULL,
    description   text NOT NULL DEFAULT '',
    price         bigint NOT NULL CHECK (price >= 0),
    is_available  boolean NOT NULL DEFAULT true,
    stock_qty     integer CHECK (stock_qty IS NULL OR stock_qty >= 0),
    image_url     text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (restaurant_id, external_id)
);

CREATE INDEX menu_items_category_idx ON menu_items (category_id);

-- Пользователи площадки
CREATE TABLE users (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL,
    phone      text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Корзина
CREATE TABLE carts (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       uuid NOT NULL UNIQUE REFERENCES users (id) ON DELETE CASCADE,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE cart_items (
    cart_id      uuid NOT NULL REFERENCES carts (id) ON DELETE CASCADE,
    menu_item_id uuid NOT NULL REFERENCES menu_items (id) ON DELETE CASCADE,
    quantity     integer NOT NULL CHECK (quantity > 0),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (cart_id, menu_item_id)
);

-- Заказы
CREATE TABLE orders (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    number           bigserial NOT NULL UNIQUE,
    user_id          uuid NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    restaurant_id    uuid NOT NULL REFERENCES restaurants (id) ON DELETE RESTRICT,
    status           text NOT NULL DEFAULT 'new'
                     CHECK (status IN ('new', 'accepted', 'cooking', 'ready',
                                       'delivering', 'delivered', 'rejected', 'cancelled')),
    delivery_address text NOT NULL,
    comment          text NOT NULL DEFAULT '',
    subtotal         bigint NOT NULL CHECK (subtotal >= 0),
    delivery_fee     bigint NOT NULL CHECK (delivery_fee >= 0),
    total            bigint NOT NULL CHECK (total >= 0),
    reject_reason    text,
    expected_at      timestamptz,
    idempotency_key  text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, idempotency_key)
);

CREATE INDEX orders_restaurant_status_idx ON orders (restaurant_id, status, created_at DESC);
CREATE INDEX orders_restaurant_updated_idx ON orders (restaurant_id, updated_at DESC);
CREATE INDEX orders_user_idx ON orders (user_id, created_at DESC);

-- Позиции заказа
CREATE TABLE order_items (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id     uuid NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    menu_item_id uuid NOT NULL REFERENCES menu_items (id) ON DELETE RESTRICT,
    external_id  text NOT NULL,
    name         text NOT NULL,
    price        bigint NOT NULL CHECK (price >= 0),
    quantity     integer NOT NULL CHECK (quantity > 0)
);

CREATE INDEX order_items_order_idx ON order_items (order_id);

-- История переходов статусов
CREATE TABLE order_status_history (
    id          bigserial PRIMARY KEY,
    order_id    uuid NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    from_status text,
    to_status   text NOT NULL,
    actor       text NOT NULL CHECK (actor IN ('user', 'restaurant', 'system')),
    reason      text,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX order_status_history_order_idx ON order_status_history (order_id, created_at);

-- Transactional outbox
CREATE TABLE outbox_events (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    restaurant_id   uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    order_id        uuid NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    event_type      text NOT NULL,
    payload         jsonb NOT NULL,
    status          text NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'delivered', 'failed')),
    attempts        integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error      text,
    delivered_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX outbox_events_pending_idx ON outbox_events (next_attempt_at)
    WHERE status = 'pending';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS order_status_history;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS cart_items;
DROP TABLE IF EXISTS carts;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS menu_items;
DROP TABLE IF EXISTS menu_categories;
DROP TABLE IF EXISTS restaurants;
-- +goose StatementEnd
