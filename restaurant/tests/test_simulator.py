"""Тесты симулятора"""

import pytest

from app import simulator
from app.schemas import OrderStatus
from app.store import store
from tests.conftest import KitchenStub, make_order


@pytest.fixture(autouse=True)
def clean_store() -> None:
    """Пустое хранилище перед каждым тестом"""
    store.__init__()  # type: ignore[misc]


async def test_advances_accepted_order_to_delivered(kitchen: KitchenStub) -> None:
    order = make_order(status=OrderStatus.ACCEPTED)
    store.save_order(order)

    chain = [
        OrderStatus.COOKING,
        OrderStatus.READY,
        OrderStatus.DELIVERING,
        OrderStatus.DELIVERED,
    ]
    for expected in chain:
        assert await simulator.advance_once(kitchen) == 1
        saved = store.order(order.id)
        assert saved is not None
        assert saved.status is expected

    assert await simulator.advance_once(kitchen) == 0
    assert [status for _, status in kitchen.statuses] == chain


async def test_ignores_orders_outside_cooking_chain(kitchen: KitchenStub) -> None:
    store.save_order(make_order("new-order", 1, status=OrderStatus.NEW))
    store.save_order(make_order("cancelled-order", 2, status=OrderStatus.CANCELLED))
    store.save_order(make_order("rejected-order", 3, status=OrderStatus.REJECTED))

    assert await simulator.advance_once(kitchen) == 0
    assert not kitchen.statuses


async def test_platform_conflict_does_not_break_batch(kitchen: KitchenStub) -> None:
    """Заказ мог быть отменён между шагами, остальные должны двигаться"""

    async def fail_first(order_id: str, status: OrderStatus) -> dict[str, object]:
        if order_id == "conflicting":
            from app.kitchen import KitchenError

            raise KitchenError("POST", "status", 409, "invalid_transition")
        kitchen.statuses.append((order_id, status))
        return {}

    kitchen.update_order_status = fail_first  # type: ignore[method-assign]

    store.save_order(make_order("conflicting", 1, status=OrderStatus.ACCEPTED))
    store.save_order(make_order("healthy", 2, status=OrderStatus.ACCEPTED))

    assert await simulator.advance_once(kitchen) == 1

    stuck = store.order("conflicting")
    assert stuck is not None
    assert stuck.status is OrderStatus.ACCEPTED

    moved = store.order("healthy")
    assert moved is not None
    assert moved.status is OrderStatus.COOKING
