#!/usr/bin/env bash
# Сквозная проверка сценариев поднятого стека
set -euo pipefail

KITCHEN=${KITCHEN_URL:-http://localhost:8080}
RESTAURANT=${RESTAURANT_URL:-http://localhost:8090}
USER_ID=${USER_ID:-11111111-1111-1111-1111-111111111111}
OTHER_USER_ID=${OTHER_USER_ID:-22222222-2222-2222-2222-222222222222}
API_KEY=${API_KEY:-demo-restaurant-key}
ADMIN_TOKEN=${ADMIN_TOKEN:-dev-admin-token}
DEMO_RESTAURANT=${DEMO_RESTAURANT:-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa}

DELIVERY_TIMEOUT=${DELIVERY_TIMEOUT:-60}

red() { printf '\033[31m%s\033[0m\n' "$1"; }
green() { printf '\033[32m%s\033[0m\n' "$1"; }
step() { printf '\n\033[1m%s\033[0m\n' "$1"; }

failures=0

check() {
	if [ "$2" = "$3" ]; then
		green "  ok   $1: $3"
	else
		red "  FAIL $1: ожидалось $2, получено $3"
		failures=$((failures + 1))
	fi
}

request() {
	local method=$1 url=$2 body=${3:-}
	shift 3 || shift 2
	local args=(-sS -o /tmp/e2e_body -w '%{http_code}' -X "$method" "$url" -H 'Content-Type: application/json')
	[ -n "$body" ] && args+=(-d "$body")
	for header in "$@"; do args+=(-H "$header"); done

	local code
	code=$(curl "${args[@]}")
	echo "$code"
}

json() {
	python3 -c "
import json, sys
with open('/tmp/e2e_body') as f:
    data = json.load(f)
for key in '$1'.split('.'):
    data = data[int(key)] if key.isdigit() else data[key]
print(data)
"
}

json_len() {
	python3 -c "
import json, sys
with open('/tmp/e2e_body') as f:
    data = json.load(f)
for key in '$1'.split('.'):
    data = data[int(key)] if key.isdigit() else data[key]
print(len(data))
"
}

menu_item() {
	request GET "$KITCHEN/api/v1/restaurants/$DEMO_RESTAURANT/menu" '' "X-User-Id: $USER_ID" >/dev/null
	python3 -c "
import json
with open('/tmp/e2e_body') as f:
    menu = json.load(f)
for category in menu['categories']:
    for item in category['items']:
        if item['external_id'] == '$1':
            print(item['id'])
"
}

step '1. Сервисы подняты'
check 'kitchen /health' 200 "$(request GET "$KITCHEN/health")"
check 'restaurant /health' 200 "$(request GET "$RESTAURANT/health")"

step '2. Каталог и меню'
check 'список заведений без X-User-Id' 401 "$(request GET "$KITCHEN/api/v1/restaurants")"
check 'список заведений' 200 "$(request GET "$KITCHEN/api/v1/restaurants" '' "X-User-Id: $USER_ID")"
check 'поиск демо-заведения' 1 \
	"$(request GET "$KITCHEN/api/v1/restaurants?search=bistro" '' "X-User-Id: $USER_ID" >/dev/null; json pagination.total)"
check 'меню заведения' 200 "$(request GET "$KITCHEN/api/v1/restaurants/$DEMO_RESTAURANT/menu" '' "X-User-Id: $USER_ID")"

burger=$(menu_item main-burger)
water=$(menu_item drink-water)
[ -n "$burger" ] || {
	red 'позиция main-burger не найдена: заведение не синхронизировало меню'
	exit 1
}

step '3. Корзина'
request DELETE "$KITCHEN/api/v1/cart" '' "X-User-Id: $USER_ID" >/dev/null
check 'добавление позиции' 200 \
	"$(request PUT "$KITCHEN/api/v1/cart/items" "{\"menu_item_id\":\"$burger\",\"quantity\":2}" "X-User-Id: $USER_ID")"
check 'позиций в корзине' 1 "$(json_len items)"
check 'позиция без остатка' 409 \
	"$(request PUT "$KITCHEN/api/v1/cart/items" "{\"menu_item_id\":\"$water\",\"quantity\":1}" "X-User-Id: $USER_ID")"
check 'код ошибки' items_unavailable "$(json code)"
check 'количество сверх остатка' 409 \
	"$(request PUT "$KITCHEN/api/v1/cart/items" "{\"menu_item_id\":\"$burger\",\"quantity\":99}" "X-User-Id: $USER_ID")"

step '4. Оформление заказа'
check 'создание заказа' 201 \
	"$(request POST "$KITCHEN/api/v1/orders" '{"delivery_address":"Москва, ул. Примерная, 10","comment":"Позвонить"}' \
		"X-User-Id: $USER_ID" "Idempotency-Key: e2e-$$")"
order_id=$(json id)
check 'статус нового заказа' new "$(json status)"
check 'корзина очищена' 200 "$(request GET "$KITCHEN/api/v1/cart" '' "X-User-Id: $USER_ID")"
check 'позиций в корзине после заказа' 0 "$(json_len items)"

check 'повтор с тем же ключом идемпотентности' 201 \
	"$(request POST "$KITCHEN/api/v1/orders" '{"delivery_address":"Москва, ул. Примерная, 10"}' \
		"X-User-Id: $USER_ID" "Idempotency-Key: e2e-$$")"
check 'вернулся тот же заказ' "$order_id" "$(json id)"

check 'чужой заказ не виден' 404 \
	"$(request GET "$KITCHEN/api/v1/orders/$order_id" '' "X-User-Id: $OTHER_USER_ID")"

step '5. Заведение получило заказ и ведёт его до доставки'
request GET "$KITCHEN/partner/v1/orders" '' "X-Api-Key: $API_KEY" >/dev/null
check 'заказ виден заведению' 200 \
	"$(request GET "$KITCHEN/partner/v1/orders/$order_id" '' "X-Api-Key: $API_KEY")"
check 'external_id позиции доехал' main-burger "$(json items.0.external_id)"

printf '  ждём доставки (до %s с): ' "$DELIVERY_TIMEOUT"
status=""
for _ in $(seq 1 "$DELIVERY_TIMEOUT"); do
	request GET "$KITCHEN/api/v1/orders/$order_id" '' "X-User-Id: $USER_ID" >/dev/null
	status=$(json status)
	printf '%s ' "$status"
	[ "$status" = "delivered" ] && break
	sleep 1
done
printf '\n'
check 'заказ доставлен' delivered "$status"

request GET "$KITCHEN/api/v1/orders/$order_id" '' "X-User-Id: $USER_ID" >/dev/null
check 'переходов в истории' 6 "$(json_len history)"

step '6. Недопустимые переходы'
check 'отмена доставленного заказа' 409 \
	"$(request POST "$KITCHEN/api/v1/orders/$order_id/cancel" '' "X-User-Id: $USER_ID")"
check 'код ошибки' invalid_transition "$(json code)"

step '7. Отмена пользователем возвращает остатки'
request PATCH "$KITCHEN/partner/v1/menu/items/main-burger" '{"stock_qty":10}' "X-Api-Key: $API_KEY" >/dev/null
stock_before=$(json stock_qty)

request PUT "$KITCHEN/api/v1/cart/items" "{\"menu_item_id\":\"$burger\",\"quantity\":2}" "X-User-Id: $USER_ID" >/dev/null
request POST "$KITCHEN/api/v1/orders" '{"delivery_address":"Москва, ул. Примерная, 10"}' "X-User-Id: $USER_ID" >/dev/null
cancel_id=$(json id)

request PATCH "$KITCHEN/partner/v1/menu/items/main-burger" '{}' "X-Api-Key: $API_KEY" >/dev/null
check 'остаток списан' "$((stock_before - 2))" "$(json stock_qty)"

check 'отмена заказа' 200 "$(request POST "$KITCHEN/api/v1/orders/$cancel_id/cancel" '' "X-User-Id: $USER_ID")"
check 'статус после отмены' cancelled "$(json status)"

request PATCH "$KITCHEN/partner/v1/menu/items/main-burger" '{}' "X-Api-Key: $API_KEY" >/dev/null
check 'остаток вернулся' "$stock_before" "$(json stock_qty)"

step '8. Партнёрский доступ'
check 'профиль с чужим ключом' 401 "$(request GET "$KITCHEN/partner/v1/me" '' 'X-Api-Key: wrong-key')"
check 'профиль заведения' 200 "$(request GET "$KITCHEN/partner/v1/me" '' "X-Api-Key: $API_KEY")"
check 'webhook настроен заведением' "http://restaurant:8090/webhooks/kitchen" "$(json webhook_url)"

step '9. Онбординг нового заведения'
check 'онбординг без токена' 401 \
	"$(request POST "$KITCHEN/admin/v1/restaurants" '{"slug":"e2e-cafe","name":"E2E Cafe","address":"Москва"}')"
slug="e2e-cafe-$$"
check 'онбординг заведения' 201 \
	"$(request POST "$KITCHEN/admin/v1/restaurants" "{\"slug\":\"$slug\",\"name\":\"E2E Cafe\",\"address\":\"Москва\"}" \
		"X-Admin-Token: $ADMIN_TOKEN")"
new_key=$(json api_key)
check 'выданный ключ работает' 200 "$(request GET "$KITCHEN/partner/v1/me" '' "X-Api-Key: $new_key")"
check 'занятый slug' 409 \
	"$(request POST "$KITCHEN/admin/v1/restaurants" "{\"slug\":\"$slug\",\"name\":\"Дубль\",\"address\":\"Москва\"}" \
		"X-Admin-Token: $ADMIN_TOKEN")"

step '10. Собственный API заведения'
check 'заказы у заведения' 200 "$(request GET "$RESTAURANT/orders")"
check 'меню заведения' 200 "$(request GET "$RESTAURANT/menu")"

printf '\n'
if [ "$failures" -eq 0 ]; then
	green 'Все проверки пройдены'
	exit 0
fi
red "Провалено проверок: $failures"
exit 1
