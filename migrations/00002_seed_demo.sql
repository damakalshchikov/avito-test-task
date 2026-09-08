-- Демо-данные для локального запуска. У демо-заведения фиксированный набор данных

-- +goose Up
-- +goose StatementBegin
INSERT INTO users (id, name, phone) VALUES
    ('11111111-1111-1111-1111-111111111111', 'Иван Петров', '+79990000001'),
    ('22222222-2222-2222-2222-222222222222', 'Мария Сидорова', '+79990000002');

INSERT INTO restaurants (id, slug, name, description, address, status, api_key_hash, webhook_secret,
                         delivery_fee, min_order_amount)
VALUES (
    'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
    'demo-bistro',
    'Demo Bistro',
    'Демонстрационное заведение, подключённое через сервис-пример restaurant/',
    'Москва, ул. Примерная, 1',
    'active',
    sha256('demo-restaurant-key'::bytea),
    'demo-webhook-secret',
    19900,
    50000
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM restaurants WHERE id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa';
DELETE FROM users WHERE id IN ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222');
-- +goose StatementEnd
