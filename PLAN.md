# План: MVP «Авито.Кухня» (основной сервис + сервис-пример заведения)

## Context

Тестовое задание для стажёра Backend (`backend-trainee-assignment-autumn-2026.md`). Нужно спроектировать и реализовать MVP платформы доставки еды: основной API-сервис (для клиентского web и для заведений) и отдельный сервис-пример одного заведения, интегрированный с основным через API. Оценивается в первую очередь проектирование: CJM-диаграммы, C4, схема БД с миграциями, OpenAPI, README с обоснованием упрощений. Репозиторий сейчас пустой (только файл задания, ветка `main`).

Зафиксированные решения:
- Основной сервис (**kitchen**): Go 1.23+, chi, pgx v5, oapi-codegen (кодогенерация сервера из OpenAPI), goose-миграции (встроенные, применяются при старте), golangci-lint.
- Сервис заведения (**restaurant**): Python 3.12, FastAPI, httpx, ruff + mypy.
- БД: PostgreSQL 16. У сервиса заведения своё хранилище (in-memory + JSON-файл меню) — обосновать как упрощение.
- Интеграция двусторонняя: Кухня шлёт заведению webhook о заказах (через outbox с ретраями), заведение через partner-API Кухни синхронизирует меню/остатки и двигает статусы заказов. Polling `GET /partner/v1/orders` — запасной канал.
- Аутентификация: заведения — `X-Api-Key`; пользователи — заголовок `X-User-Id` без проверки (по заданию auth не нужна); admin-эндпоинт — статический `X-Admin-Token` из env.
- Диаграммы: PlantUML (`.puml` + отрендеренные `.svg` в репо для README).
- Git: ветками и коммитами управляет пользователь. Я не выполняю git-команды; каждый этап ниже соответствует одной feature-ветке, в конце этапа останавливаюсь и сообщаю, что готово к коммиту.

## Структура репозитория (monorepo)

```
.
├── cmd/kitchen/main.go              # точка входа основного сервиса
├── internal/
│   ├── config/                      # env-конфиг
│   ├── api/                         # сгенерированный oapi-codegen код (types + strict server)
│   ├── transport/http/              # хендлеры (реализация сгенерированного интерфейса), middleware (auth, logging, recover), маппинг ошибок
│   ├── domain/                      # сущности, статусы заказа, state machine, доменные ошибки
│   ├── service/                     # use cases: catalog, cart, order, partner, admin
│   ├── repository/postgres/         # pgx-репозитории, транзакции (UnitOfWork)
│   └── outbox/                      # воркер доставки webhook-ов с ретраями
├── migrations/                      # goose SQL: 0001_init.sql ... + 00NN_seed_demo.sql
├── api/openapi.yaml                 # единая OpenAPI 3.0 спека (client + partner + admin)
├── restaurant/                      # сервис заведения (Python)
│   ├── app/{main.py, kitchen_client.py, store.py, simulator.py, schemas.py, routes/}
│   ├── data/menu.json
│   ├── pyproject.toml, Dockerfile
├── docs/
│   ├── diagrams/*.puml + *.svg      # cjm-user, cjm-restaurant, c4-context, c4-container, c4-component, er, seq-order-flow
│   ├── db.md                        # описание схемы БД
│   └── api.md                       # обзор API и сценариев
├── scripts/smoke.sh                 # e2e-сценарий через curl
├── docker-compose.yml
├── Makefile                         # build, lint, test, generate, diagrams, up, smoke
├── .golangci.yml, oapi-codegen.yaml
├── README.md
└── PLAN.md                          # этот план (задание просит приложить prompt/plan)
```

## Доменная модель и схема БД

Деньги — `integer` в копейках. Идентификаторы — UUID. Везде `created_at/updated_at timestamptz`.

- **restaurants**: id, slug, name, description, address, status (`active|paused|disabled`), api_key_hash (sha256), webhook_url nullable, webhook_secret, delivery_fee, min_order_amount.
- **menu_categories**: id, restaurant_id FK, name, sort_order. unique(restaurant_id, name).
- **menu_items**: id, restaurant_id FK, category_id FK, external_id (id в системе заведения), name, description, price, is_available bool, stock_qty int nullable (NULL = без учёта остатков), image_url. unique(restaurant_id, external_id).
- **users**: id, name, phone (seed-данные, auth нет).
- **carts**: id, user_id FK, restaurant_id FK. unique(user_id) — одна активная корзина, привязанная к одному заведению.
- **cart_items**: cart_id FK, menu_item_id FK, quantity. PK(cart_id, menu_item_id).
- **orders**: id, number (serial, человекочитаемый), user_id FK, restaurant_id FK, status, delivery_address (snapshot), comment, subtotal, delivery_fee, total, reject_reason nullable, expected_at nullable. Индексы: (restaurant_id, status, created_at), (user_id, created_at).
- **order_items**: id, order_id FK, menu_item_id FK, name/price snapshot, quantity.
- **order_status_history**: id, order_id FK, from_status, to_status, actor (`user|restaurant|system`), reason, created_at.
- **outbox_events**: id, restaurant_id FK, event_type, payload jsonb, status (`pending|delivered|failed`), attempts, next_attempt_at, last_error, delivered_at. Индекс (status, next_attempt_at).

**State machine заказа** (`internal/domain/order_status.go`, покрыть unit-тестами):
```
new ──accept──▶ accepted ──▶ cooking ──▶ ready ──▶ delivering ──▶ delivered
 │ reject (restaurant)                                             
 └──────────▶ rejected
new|accepted ──cancel (user)──▶ cancelled
```
Переход `accepted→cooking→ready→delivering→delivered` только в этом порядке, инициатор — заведение. `rejected/cancelled` возвращают остатки в `menu_items.stock_qty`.

**Остатки**: при создании заказа в одной транзакции `SELECT ... FOR UPDATE` по позициям, проверка `is_available` и `stock_qty >= qty`, декремент. Иначе `409 Conflict` с перечнем проблемных позиций — это ключевой сценарий «товар закончился».

## API (`api/openapi.yaml`, единый формат ошибок `{code, message, details[]}`)

**Клиентский API** `/api/v1` (заголовок `X-User-Id`):
- `GET /restaurants?search=&page=&page_size=` — только `active`.
- `GET /restaurants/{id}`, `GET /restaurants/{id}/menu` — категории с позициями, флаги доступности.
- `GET /cart`, `PUT /cart/items` (`{menu_item_id, quantity}`; quantity=0 удаляет), `DELETE /cart`. Позиция другого заведения → `409 cart_restaurant_mismatch` (клиент явно очищает корзину).
- `POST /orders` (`{delivery_address, comment}`, из корзины, заголовок `Idempotency-Key` опционально) → 201; `409` при нехватке остатков/паузе заведения/пустой корзине/меньше min_order.
- `GET /orders`, `GET /orders/{id}` (с историей статусов), `POST /orders/{id}/cancel`.

**Партнёрский API** `/partner/v1` (заголовок `X-Api-Key`):
- `GET /me`, `PATCH /me` (`status`, `webhook_url`, `delivery_fee`, `min_order_amount`).
- `PUT /menu` — полный upsert меню по `external_id` (категории + позиции); отсутствующие в payload позиции помечаются `is_available=false`, не удаляются (заказы ссылаются на них).
- `PATCH /menu/items/{external_id}` — `is_available`, `stock_qty`, `price`.
- `GET /orders?status=&since=&page=` — polling-fallback; `GET /orders/{id}`.
- `POST /orders/{id}/accept`, `POST /orders/{id}/reject` (`{reason}`), `POST /orders/{id}/status` (`{status}`) — валидируется state machine, `409` при недопустимом переходе.

**Admin API** `/admin/v1` (заголовок `X-Admin-Token`):
- `POST /restaurants` — онбординг заведения, возвращает plaintext API-ключ один раз.

**Webhook Кухня → заведение**: `POST {webhook_url}`, тело `{event_id, type: "order.created"|"order.cancelled", occurred_at, order}`, заголовок `X-Kitchen-Signature: sha256=HMAC(webhook_secret, body)`. Заведение дедуплицирует по `event_id`. Ретраи по экспоненте (1s, 5s, 30s, 2m, 10m), максимум 8 попыток, затем `failed`.

## Сервис заведения (`restaurant/`, FastAPI)

- Стартап: загружает `data/menu.json`, вызывает `PATCH /partner/v1/me` (свой `webhook_url`, `status=active`) и `PUT /partner/v1/menu`. Ретраит до готовности Кухни.
- `POST /webhooks/kitchen` — проверка HMAC, дедуп по `event_id`, сохраняет заказ, автоматически принимает (`accept`) или отклоняет (`reject`), если позиции нет в локальном меню или локально закончилась.
- `simulator.py` — фоновая задача «кухня»: двигает принятые заказы `cooking → ready → delivering → delivered` с настраиваемыми задержками (env `SIMULATION_STEP_SECONDS`, по умолчанию 5), каждый шаг вызывает `POST /partner/v1/orders/{id}/status`.
- Собственный API заведения (демонстрация CJM ресторана): `GET /orders`, `POST /orders/{id}/accept|reject` (ручной режим при `AUTO_ACCEPT=false`), `PATCH /menu/items/{id}` (остаток/доступность → проброс в Кухню), `POST /pause`, `POST /resume`, `GET /health`.
- Хранилище: in-memory dict (обосновать в README: у реального заведения своя учётная система, для MVP-примера персистентность не нужна).

## Диаграммы (`docs/diagrams/`)

- `cjm-user.puml` (activity): поиск заведения → меню → корзина → оформление → отслеживание; ветки: товар закончился, заведение на паузе, корзина другого заведения, отказ заведения, отмена пользователем.
- `cjm-restaurant.puml` (activity): онбординг/ключ → настройка webhook → синк меню → получение заказа → принять/отклонить → готовка → доставка; ветки: webhook не доставлен → polling, изменение остатков, пауза.
- `c4-context.puml`, `c4-container.puml` (Level 2: web-клиент, kitchen API, PostgreSQL, outbox worker, restaurant service), `c4-component.puml` (Level 3 для kitchen: transport/service/repository/outbox).
- `er.puml` — схема БД. `seq-order-flow.puml` — sequence создания заказа и webhook-цикла.
- Рендер: `make diagrams` через `docker run plantuml/plantuml -tsvg`; svg коммитятся, README ссылается на них.

## Этапы реализации (каждый = одна feature-ветка пользователя)

1. **Каркас** — `go.mod`, `cmd/kitchen`, конфиг, health-эндпоинт, `docker-compose.yml` (postgres + kitchen), `Makefile`, `.golangci.yml`, `oapi-codegen.yaml`, `restaurant/pyproject.toml` с ruff/mypy, `PLAN.md`, README-скелет.
2. **Проектирование** — `api/openapi.yaml` полностью, миграции `0001_init.sql` (схема) + `0002_seed_demo.sql` (2 пользователя, демо-заведение `demo-bistro` с фиксированным ключом `demo-restaurant-key`, документируется как упрощение), все `.puml` + svg, `docs/db.md`, `docs/api.md`. Генерация `internal/api` через oapi-codegen (strict-server, chi).
3. **Каталог и партнёрский доступ** — domain, репозитории restaurants/menu, middleware `X-Api-Key`/`X-User-Id`/`X-Admin-Token`, эндпоинты restaurants/menu (client), `me`/`menu` (partner), admin onboarding. Unit-тесты сервисов на моках репозиториев.
4. **Корзина и заказы** — cart, order creation в транзакции с резервированием остатков, state machine + history, client `orders`, partner `orders` (accept/reject/status/polling). Unit-тесты state machine и сценария нехватки остатков.
5. **Webhook-доставка** — запись в `outbox_events` в той же транзакции, что и заказ; воркер с ретраями и HMAC-подписью; `order.cancelled` событие.
6. **Сервис заведения** — FastAPI-приложение по описанию выше, Dockerfile, добавление в compose с `depends_on: kitchen (healthy)`.
7. **E2E и документация** — `scripts/smoke.sh` (полный happy-path до `delivered` + сценарий out-of-stock + отмена), финальный README (описание, архитектура с C4, схема БД, сценарии, запуск, упрощения, ссылки на PLAN.md).

## Верификация

- `make lint` — golangci-lint для Go, `ruff check` + `mypy` для Python, чисто.
- `make test` — `go test ./...` (state machine, сервисы), `pytest` для restaurant (webhook-обработчик, симулятор).
- `docker compose up --build` — поднимаются postgres, kitchen (миграции применились, `/health` 200), restaurant (в логах: меню синхронизировано).
- `make smoke` (`scripts/smoke.sh`): список заведений → меню → добавить в корзину → создать заказ → в течение ~30 с статус доходит до `delivered`; попытка заказать больше остатка → `409`; отмена в статусе `new` → `cancelled`, остаток восстановлен; `GET /partner/v1/orders` с ключом отдаёт заказы.
- Диаграммы рендерятся без ошибок `make diagrams`; OpenAPI валидна (oapi-codegen генерируется без ошибок).
