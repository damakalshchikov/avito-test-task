"""Тесты API заведения"""

from fastapi.testclient import TestClient

from app.schemas import OrderStatus
from app.store import store
from tests.conftest import KitchenStub, make_order


def test_health(client: TestClient) -> None:
    response = client.get("/health")
    assert response.status_code == 200
    assert response.json() == {"status": "ok"}


def test_menu_exposes_stock(client: TestClient) -> None:
    response = client.get("/menu")
    assert response.status_code == 200

    categories = response.json()["categories"]
    assert categories
    items = [item for category in categories for item in category["items"]]
    assert any(item["external_id"] == "soup-borsch" for item in items)


def test_update_item_pushes_to_platform(client: TestClient, kitchen: KitchenStub) -> None:
    response = client.patch("/menu/items/soup-borsch", json={"stock_qty": 3, "price": 41000})
    assert response.status_code == 200

    body = response.json()
    assert body["stock_qty"] == 3
    assert body["price"] == 41000
    assert kitchen.pushed == ["soup-borsch"]


def test_update_unknown_item(client: TestClient, kitchen: KitchenStub) -> None:
    assert client.patch("/menu/items/nope", json={"stock_qty": 1}).status_code == 404
    assert not kitchen.pushed


def test_update_rejects_negative_stock(client: TestClient, kitchen: KitchenStub) -> None:
    assert client.patch("/menu/items/soup-borsch", json={"stock_qty": -1}).status_code == 422
    assert not kitchen.pushed


def test_orders_listing_and_filter(client: TestClient) -> None:
    store.save_order(make_order("a", 1, status=OrderStatus.NEW))
    store.save_order(make_order("b", 2, status=OrderStatus.COOKING))

    assert len(client.get("/orders").json()) == 2

    cooking = client.get("/orders", params={"order_status": "cooking"}).json()
    assert [order["id"] for order in cooking] == ["b"]


def test_get_order(client: TestClient) -> None:
    store.save_order(make_order("a", 7))

    assert client.get("/orders/a").json()["number"] == 7
    assert client.get("/orders/missing").status_code == 404


def test_manual_accept(client: TestClient, kitchen: KitchenStub) -> None:
    store.save_order(make_order("a", 1, status=OrderStatus.NEW))

    assert client.post("/orders/a/accept").status_code == 200
    assert kitchen.accepted == ["a"]

    # Повторное принятие уже принятого заказа отклоняется
    assert client.post("/orders/a/accept").status_code == 409


def test_manual_reject(client: TestClient, kitchen: KitchenStub) -> None:
    store.save_order(make_order("a", 1, status=OrderStatus.NEW))

    response = client.post("/orders/a/reject", json={"reason": "Сломалась плита"})
    assert response.status_code == 200
    assert kitchen.rejected == [("a", "Сломалась плита")]


def test_manual_reject_requires_reason(client: TestClient, kitchen: KitchenStub) -> None:
    store.save_order(make_order("a", 1, status=OrderStatus.NEW))

    assert client.post("/orders/a/reject", json={"reason": ""}).status_code == 422
    assert not kitchen.rejected


def test_pause_and_resume(client: TestClient, kitchen: KitchenStub) -> None:
    assert client.post("/pause").json() == {"status": "paused"}
    assert store.paused is True

    assert client.post("/resume").json() == {"status": "active"}
    assert store.paused is False
    assert kitchen.profile_status == ["paused", "active"]
