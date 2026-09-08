"""API заведения"""

import logging

from fastapi import APIRouter, HTTPException, Request, status

from app.kitchen import KitchenClient, KitchenError
from app.schemas import Menu, MenuItem, MenuItemUpdate, Order, OrderStatus, RejectRequest
from app.store import store
from app.webhooks import accept, safe_reject

logger = logging.getLogger(__name__)
router = APIRouter(tags=["restaurant"])


def kitchen(request: Request) -> KitchenClient:
    """Клиент платформы"""
    client: KitchenClient = request.app.state.kitchen
    return client


@router.get("/menu")
async def get_menu() -> Menu:
    """Меню заведения с текущими остатками"""
    return store.menu


@router.patch("/menu/items/{external_id}")
async def update_menu_item(external_id: str, update: MenuItemUpdate, request: Request) -> MenuItem:
    """Меняет остаток или цену позиции и сообщает об этом платформе"""
    item = store.update_item(external_id, stock_qty=update.stock_qty, price=update.price)
    if item is None:
        raise HTTPException(status.HTTP_404_NOT_FOUND, "Позиция не найдена")

    try:
        await kitchen(request).push_item(item)
    except KitchenError as error:
        logger.error("не удалось обновить позицию на платформе: %s", error)
        raise HTTPException(status.HTTP_502_BAD_GATEWAY, "Платформа недоступна") from error

    return item


@router.get("/orders")
async def list_orders(order_status: OrderStatus | None = None) -> list[Order]:
    """Заказы заведения, при необходимости отфильтрованные по статусу"""
    return store.orders(order_status)


@router.get("/orders/{order_id}")
async def get_order(order_id: str) -> Order:
    """Заказ по идентификатору платформы"""
    order = store.order(order_id)
    if order is None:
        raise HTTPException(status.HTTP_404_NOT_FOUND, "Заказ не найден")
    return order


@router.post("/orders/{order_id}/accept")
async def accept_order(order_id: str, request: Request) -> Order:
    """Принимает заказ вручную, когда выключен автоприём"""
    order = store.order(order_id)
    if order is None:
        raise HTTPException(status.HTTP_404_NOT_FOUND, "Заказ не найден")
    if order.status is not OrderStatus.NEW:
        raise HTTPException(status.HTTP_409_CONFLICT, f"Заказ уже в статусе {order.status.value}")

    await accept(kitchen(request), order)
    return order


@router.post("/orders/{order_id}/reject")
async def reject_order(order_id: str, body: RejectRequest, request: Request) -> Order:
    """Отклоняет заказ вручную"""
    order = store.order(order_id)
    if order is None:
        raise HTTPException(status.HTTP_404_NOT_FOUND, "Заказ не найден")
    if order.status is not OrderStatus.NEW:
        raise HTTPException(status.HTTP_409_CONFLICT, f"Заказ уже в статусе {order.status.value}")

    await safe_reject(kitchen(request), order, body.reason)
    return order


@router.post("/pause")
async def pause(request: Request) -> dict[str, str]:
    """Ставит заведение на паузу"""
    await kitchen(request).set_status("paused")
    store.paused = True
    logger.info("заведение на паузе")
    return {"status": "paused"}


@router.post("/resume")
async def resume(request: Request) -> dict[str, str]:
    """Снимает паузу"""
    await kitchen(request).set_status("active")
    store.paused = False
    logger.info("заведение снова принимает заказы")
    return {"status": "active"}
