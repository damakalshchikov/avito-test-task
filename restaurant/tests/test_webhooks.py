"""Тесты webhook-ов"""

from fastapi.testclient import TestClient

from app.schemas import OrderStatus
from app.store import store
from tests.conftest import KitchenStub, make_order, sign, webhook_body

HEADER = "X-Kitchen-Signature"


def post(client: TestClient, body: bytes, signature: str) -> int:
    """Отправляет webhook и возвращает код ответа"""
    response = client.post("/webhooks/kitchen", content=body, headers={HEADER: signature})
    return response.status_code


def test_accepts_order_with_valid_signature(client: TestClient, kitchen: KitchenStub) -> None:
    order = make_order()
    body = webhook_body("order.created", order)

    assert post(client, body, sign(body)) == 204
    assert kitchen.accepted == [order.id]
    assert not kitchen.rejected

    saved = store.order(order.id)
    assert saved is not None
    assert saved.status is OrderStatus.ACCEPTED


def test_rejects_wrong_signature(client: TestClient, kitchen: KitchenStub) -> None:
    order = make_order()
    body = webhook_body("order.created", order)

    assert post(client, body, sign(body, secret="чужой")) == 401  # noqa: S106
    assert not kitchen.accepted
    assert store.order(order.id) is None


def test_rejects_missing_signature(client: TestClient, kitchen: KitchenStub) -> None:
    body = webhook_body("order.created", make_order())

    assert client.post("/webhooks/kitchen", content=body).status_code == 401
    assert not kitchen.accepted


def test_rejects_tampered_body(client: TestClient, kitchen: KitchenStub) -> None:
    body = webhook_body("order.created", make_order())
    signature = sign(body)
    tampered = body.replace(b'"quantity": 2', b'"quantity": 99')

    assert post(client, tampered, signature) == 401
    assert not kitchen.accepted


def test_deduplicates_repeated_event(client: TestClient, kitchen: KitchenStub) -> None:
    """Платформа повторяет доставку при сбоях"""
    order = make_order()
    body = webhook_body("order.created", order, event_id="event-repeat")
    signature = sign(body)

    assert post(client, body, signature) == 204
    assert post(client, body, signature) == 204

    assert kitchen.accepted == [order.id]


def test_rejects_order_with_unknown_item(client: TestClient, kitchen: KitchenStub) -> None:
    order = make_order()
    order.items[0].external_id = "unknown-dish"
    body = webhook_body("order.created", order)

    assert post(client, body, sign(body)) == 204
    assert not kitchen.accepted
    assert len(kitchen.rejected) == 1
    assert "unknown-dish" in kitchen.rejected[0][1]


def test_rejects_order_without_stock(client: TestClient, kitchen: KitchenStub) -> None:
    store.update_item("soup-borsch", stock_qty=1)
    body = webhook_body("order.created", make_order())

    assert post(client, body, sign(body)) == 204
    assert not kitchen.accepted
    assert len(kitchen.rejected) == 1


def test_rejects_while_paused(client: TestClient, kitchen: KitchenStub) -> None:
    store.paused = True
    body = webhook_body("order.created", make_order())

    assert post(client, body, sign(body)) == 204
    assert not kitchen.accepted
    assert "не принимает заказы" in kitchen.rejected[0][1]


def test_accept_decrements_local_stock(client: TestClient, kitchen: KitchenStub) -> None:
    item = store.item("soup-borsch")
    assert item is not None
    before = item.stock_qty
    assert before is not None

    body = webhook_body("order.created", make_order())
    assert post(client, body, sign(body)) == 204

    after = store.item("soup-borsch")
    assert after is not None
    assert after.stock_qty == before - 2


def test_cancel_returns_local_stock(client: TestClient, kitchen: KitchenStub) -> None:
    order = make_order()
    created = webhook_body("order.created", order, event_id="created")
    assert post(client, created, sign(created)) == 204

    item = store.item("soup-borsch")
    assert item is not None
    reserved = item.stock_qty
    assert reserved is not None

    order.status = OrderStatus.CANCELLED
    cancelled = webhook_body("order.cancelled", order, event_id="cancelled")
    assert post(client, cancelled, sign(cancelled)) == 204

    restored = store.item("soup-borsch")
    assert restored is not None
    assert restored.stock_qty == reserved + 2

    saved = store.order(order.id)
    assert saved is not None
    assert saved.status is OrderStatus.CANCELLED


def test_platform_error_on_accept_keeps_order_new(client: TestClient, kitchen: KitchenStub) -> None:
    """Ошибка платформы не должна списывать остатки заведения"""
    kitchen.fail_accept = True
    item = store.item("soup-borsch")
    assert item is not None
    before = item.stock_qty

    body = webhook_body("order.created", make_order())
    assert post(client, body, sign(body)) == 204

    after = store.item("soup-borsch")
    assert after is not None
    assert after.stock_qty == before

    saved = store.order("order-1")
    assert saved is not None
    assert saved.status is OrderStatus.NEW
