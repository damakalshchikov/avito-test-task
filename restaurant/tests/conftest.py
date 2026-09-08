"""Тесты заведения"""

import hashlib
import hmac
import json
from collections.abc import Iterator
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

import pytest
from fastapi.testclient import TestClient

from app.kitchen import KitchenClient
from app.main import app
from app.schemas import Order, OrderItem, OrderStatus
from app.settings import settings
from app.store import store

MENU_PATH = Path(__file__).resolve().parents[1] / "data" / "menu.json"


class KitchenStub(KitchenClient):
    """Заглушка платформы"""

    def __init__(self) -> None:
        self.accepted: list[str] = []
        self.rejected: list[tuple[str, str]] = []
        self.statuses: list[tuple[str, OrderStatus]] = []
        self.pushed: list[str] = []
        self.profile_status: list[str] = []
        self.fail_accept = False

    async def accept_order(
        self, order_id: str, expected_at: datetime | None = None
    ) -> dict[str, Any]:
        if self.fail_accept:
            from app.kitchen import KitchenError

            raise KitchenError("POST", "accept", 409, "invalid_transition")
        self.accepted.append(order_id)
        return {}

    async def reject_order(self, order_id: str, reason: str) -> dict[str, Any]:
        self.rejected.append((order_id, reason))
        return {}

    async def update_order_status(self, order_id: str, status: OrderStatus) -> dict[str, Any]:
        self.statuses.append((order_id, status))
        return {}

    async def push_item(self, item: Any) -> dict[str, Any]:
        self.pushed.append(item.external_id)
        return {}

    async def set_status(self, status: str) -> dict[str, Any]:
        self.profile_status.append(status)
        return {}

    async def close(self) -> None:
        return None


@pytest.fixture
def kitchen() -> KitchenStub:
    """Заглушка платформы"""
    return KitchenStub()


@pytest.fixture
def client(kitchen: KitchenStub) -> Iterator[TestClient]:
    """Клиент приложения с загруженным меню и заглушкой платформы"""
    store.__init__()
    store.load_menu(MENU_PATH)
    store.paused = False

    app.state.kitchen = kitchen
    with TestClient(app) as test_client:
        app.state.kitchen = kitchen
        yield test_client


def make_order(order_id: str = "order-1", number: int = 1, **overrides: Any) -> Order:
    """Заказ по позиции из демо-меню"""
    payload: dict[str, Any] = {
        "id": order_id,
        "number": number,
        "status": OrderStatus.NEW,
        "items": [
            OrderItem(
                menu_item_id="menu-1",
                external_id="soup-borsch",
                name="Борщ с говядиной",
                price=39000,
                quantity=2,
            )
        ],
        "delivery_address": "Москва, ул. Примерная, 10",
        "subtotal": 78000,
        "delivery_fee": 19900,
        "total": 97900,
        "created_at": datetime.now(UTC),
        "updated_at": datetime.now(UTC),
    }
    payload.update(overrides)
    return Order.model_validate(payload)


def webhook_body(event_type: str, order: Order, event_id: str = "event-1") -> bytes:
    """Тело webhook-а от платформы"""
    return json.dumps(
        {
            "event_id": event_id,
            "type": event_type,
            "occurred_at": datetime.now(UTC).isoformat(),
            "order": json.loads(order.model_dump_json()),
        }
    ).encode()


def sign(body: bytes, secret: str | None = None) -> str:
    """Подпись тела секретом заведения"""
    key = (secret or settings.webhook_secret).encode()
    return "sha256=" + hmac.new(key, body, hashlib.sha256).hexdigest()
