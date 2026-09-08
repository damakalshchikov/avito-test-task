"""Приём webhook-ов от платформы"""

import hashlib
import hmac
import logging

from fastapi import APIRouter, Header, Request, Response, status

from app.kitchen import KitchenClient, KitchenError
from app.schemas import Order, OrderStatus, WebhookEvent, WebhookEventType
from app.settings import settings
from app.store import store

logger = logging.getLogger(__name__)
router = APIRouter(tags=["webhooks"])

SIGNATURE_HEADER = "X-Kitchen-Signature"


def verify_signature(secret: str, body: bytes, signature: str) -> bool:
    """Проверяет подпись тела запроса секретом заведения"""
    expected = "sha256=" + hmac.new(secret.encode(), body, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, signature)


@router.post("/webhooks/kitchen", status_code=status.HTTP_204_NO_CONTENT)
async def receive(
    request: Request,
    x_kitchen_signature: str = Header(default=""),
) -> Response:
    """Принимает событие заказа от платформы"""
    body = await request.body()

    if not verify_signature(settings.webhook_secret, body, x_kitchen_signature):
        logger.warning("отклонён webhook с неверной подписью")
        return Response(status_code=status.HTTP_401_UNAUTHORIZED)

    event = WebhookEvent.model_validate_json(body)

    if not store.mark_handled(event.event_id):
        logger.info("событие %s уже обработано", event.event_id)
        return Response(status_code=status.HTTP_204_NO_CONTENT)

    logger.info("получено событие %s по заказу №%s", event.type.value, event.order.number)
    client: KitchenClient = request.app.state.kitchen

    if event.type is WebhookEventType.ORDER_CREATED:
        await handle_created(client, event.order)
    else:
        await handle_cancelled(event.order)

    return Response(status_code=status.HTTP_204_NO_CONTENT)


async def handle_created(client: KitchenClient, order: Order) -> None:
    """Решает, брать ли заказ в работу"""
    store.save_order(order)

    if store.paused:
        await safe_reject(client, order, "Заведение временно не принимает заказы")
        return

    missing = store.unavailable_items(order)
    if missing:
        await safe_reject(client, order, f"Нет в наличии: {', '.join(missing)}")
        return

    if not settings.auto_accept:
        logger.info("заказ №%s ждёт решения оператора", order.number)
        return

    await accept(client, order)


async def accept(client: KitchenClient, order: Order) -> None:
    """Принимает заказ и списывает остатки со склада заведения"""
    try:
        await client.accept_order(order.id)
    except KitchenError as error:
        logger.error("не удалось принять заказ №%s: %s", order.number, error)
        return

    store.reserve(order)
    store.set_status(order.id, OrderStatus.ACCEPTED)
    logger.info("заказ №%s принят в работу", order.number)


async def safe_reject(client: KitchenClient, order: Order, reason: str) -> None:
    """Отклоняет заказ"""
    try:
        await client.reject_order(order.id, reason)
    except KitchenError as error:
        logger.error("не удалось отклонить заказ №%s: %s", order.number, error)
        return

    store.set_status(order.id, OrderStatus.REJECTED)
    logger.info("заказ №%s отклонён: %s", order.number, reason)


async def handle_cancelled(order: Order) -> None:
    """Снимает отменённый заказ с кухни"""
    known = store.order(order.id)
    if known is not None and known.status is OrderStatus.ACCEPTED:
        store.release(known)

    store.save_order(order)
    store.set_status(order.id, OrderStatus.CANCELLED)
    logger.info("заказ №%s отменён пользователем, снят с кухни", order.number)
