"""Симулятор кухни"""

import asyncio
import contextlib
import logging

from app.kitchen import KitchenClient, KitchenError
from app.schemas import OrderStatus
from app.store import store

logger = logging.getLogger(__name__)

# Цепочка, по которой заведение ведёт принятый заказ
COOKING_CHAIN: dict[OrderStatus, OrderStatus] = {
    OrderStatus.ACCEPTED: OrderStatus.COOKING,
    OrderStatus.COOKING: OrderStatus.READY,
    OrderStatus.READY: OrderStatus.DELIVERING,
    OrderStatus.DELIVERING: OrderStatus.DELIVERED,
}


async def advance_once(client: KitchenClient) -> int:
    """Продвигает на шаг все заказы в работе, возвращает число переходов"""
    moved = 0
    for order in store.orders():
        nxt = COOKING_CHAIN.get(order.status)
        if nxt is None:
            continue

        try:
            await client.update_order_status(order.id, nxt)
        except KitchenError as error:
            # Заказ мог быть отменён пользователем между шагами
            logger.warning("заказ №%s не переведён в %s: %s", order.number, nxt.value, error)
            continue

        store.set_status(order.id, nxt)
        moved += 1
        logger.info("заказ №%s переведён в %s", order.number, nxt.value)
    return moved


async def run(client: KitchenClient, step_seconds: float) -> None:
    """Крутит симулятор до отмены задачи"""
    logger.info("симулятор кухни запущен, шаг %.1f с", step_seconds)
    while True:
        await asyncio.sleep(step_seconds)
        try:
            await advance_once(client)
        except Exception:
            logger.exception("шаг симулятора завершился ошибкой")


def start(client: KitchenClient, step_seconds: float) -> asyncio.Task[None]:
    """Запускает симулятор фоновой задачей"""
    return asyncio.create_task(run(client, step_seconds))


async def stop(task: asyncio.Task[None]) -> None:
    """Останавливает симулятор и дожидается завершения"""
    task.cancel()
    with contextlib.suppress(asyncio.CancelledError):
        await task
