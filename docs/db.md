# Схема базы данных

СУБД: PostgreSQL 16. Миграции: [goose](https://github.com/pressly/goose), SQL-файлы в [`migrations/`](../migrations), встраиваются в бинарник и применяются при старте сервиса `kitchen`.

ER-диаграмма: [`docs/diagrams/er.svg`](diagrams/er.svg) (исходник `er.puml`).

## Принципы

- **Идентификаторы**: `uuid` (генерируются БД через `gen_random_uuid()`), кроме `order_status_history.id` (`bigserial`, только для порядка). У заказа дополнительно есть `number bigserial` - человекочитаемый номер для клиента и заведения.
- **Деньги**: `bigint` в минорных единицах (копейках), типы `numeric` и `float` не используются.
- **Время**: везде `timestamptz`, `created_at`/`updated_at`.
- **Статусы**: `text` с `CHECK`-ограничением вместо `enum`-типов. Добавление значения в `CHECK` дешевле, чем `ALTER TYPE`, переходы валидирует доменный слой.
- **Snapshot-данные**: в `orders` и `order_items` копируются адрес, название и цена позиции на момент заказа. Изменение меню заведения не влияет на уже созданные заказы.
- **Удаление позиций меню**: позиции, пропавшие из синхронизации заведения, помечаются `is_available=false`, а не удаляются, поскольку на них ссылаются `order_items` (`ON DELETE RESTRICT`).

## Таблицы

### Заведения и меню

| Таблица | Назначение | Ключевые поля |
|---|---|---|
| `restaurants` | Заведение-партнёр | `slug` (уникальный), `status` (`active/paused/disabled`), `api_key_hash` (sha256 ключа, сам ключ не хранится), `webhook_url` (NULL - работает через polling), `webhook_secret` (HMAC для подписи webhook-ов), `delivery_fee`, `min_order_amount` |
| `menu_categories` | Категории меню | `UNIQUE (restaurant_id, name)`, `sort_order` |
| `menu_items` | Позиции меню | `external_id` - id в системе заведения, `UNIQUE (restaurant_id, external_id)`; `is_available`; `stock_qty` (NULL - без учёта остатков) |

Данные платформы и заведения связаны через `external_id`: заведение синхронизирует меню своими идентификаторами и получает их же в `order_items.external_id` при уведомлении о заказе. Внутри платформы используются собственные `uuid`.

### Пользователи и корзина

| Таблица | Назначение | Ключевые поля |
|---|---|---|
| `users` | Пользователи площадки | Аутентификация вне рамок MVP: пользователи создаются seed-миграцией, идентифицируются заголовком `X-User-Id` |
| `carts` | Корзина | `UNIQUE (user_id)` - одна корзина на пользователя; `restaurant_id` - корзина привязана к одному заведению |
| `cart_items` | Позиции корзины | `PK (cart_id, menu_item_id)`, `quantity > 0` |

### Заказы

| Таблица | Назначение | Ключевые поля |
|---|---|---|
| `orders` | Заказ | `number` (bigserial), `status`, snapshot `delivery_address`, суммы `subtotal/delivery_fee/total`, `reject_reason`, `expected_at`, `idempotency_key` с `UNIQUE (user_id, idempotency_key)` |
| `order_items` | Позиции заказа | snapshot `name`, `price`, `external_id`; `menu_item_id` с `ON DELETE RESTRICT` |
| `order_status_history` | Аудит переходов | `from_status` (NULL при создании), `to_status`, `actor` (`user/restaurant/system`), `reason` |

### Интеграция с заведениями

| Таблица | Назначение | Ключевые поля |
|---|---|---|
| `outbox_events` | Transactional outbox для webhook-ов | `event_type` (`order.created`, `order.cancelled`), `payload jsonb`, `status` (`pending/delivered/failed`), `attempts`, `next_attempt_at`, `last_error` |

Событие записывается в той же транзакции, что и заказ, поэтому состояние «заказ создан, уведомление отсутствует» недостижимо. Фоновый воркер выбирает `pending`-события по частичному индексу `(next_attempt_at) WHERE status = 'pending'`, отправляет `POST webhook_url` с HMAC-подписью и при ошибке откладывает следующую попытку по возрастающему расписанию. После исчерпания попыток событие получает статус `failed`, заказ остаётся доступен заведению через `GET /partner/v1/orders`.

## Индексы

| Индекс | Назначение |
|---|---|
| `restaurants_status_name_idx (status, name)` | Список активных заведений с поиском по имени |
| `menu_items (restaurant_id, external_id)` unique | Upsert при синхронизации меню |
| `orders_restaurant_status_idx (restaurant_id, status, created_at DESC)` | Polling заведением новых заказов |
| `orders_restaurant_updated_idx (restaurant_id, updated_at DESC)` | Polling по `since` |
| `orders_user_idx (user_id, created_at DESC)` | История заказов пользователя |
| `outbox_events_pending_idx (next_attempt_at) WHERE status='pending'` | Выборка воркером только недоставленных событий |

## Seed-данные

`00002_seed_demo.sql` создаёт двух пользователей и заведение `demo-bistro` с фиксированным API-ключом `demo-restaurant-key` и секретом `demo-webhook-secret`. Упрощение для локального запуска: сервис-пример `restaurant/` подключается без ручного онбординга. Новые заведения подключаются через `POST /admin/v1/restaurants`.

## Масштабирование

Направления развития схемы без изменения структуры:

- Партиционирование `orders` и `order_status_history` по `created_at` при росте объёма истории.
- Очистка `outbox_events` после доставки по retention; при росте нагрузки outbox заменяется на брокер сообщений без изменения кода сервисов.
- Кэширование запросов каталога (`restaurants`, `menu_items`): данные меняются только по инициативе заведения.
