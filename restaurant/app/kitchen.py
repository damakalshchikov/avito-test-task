"""Клиент партнёрского API Авито.Кухня"""

import asyncio
import logging
from datetime import datetime
from typing import Any

import httpx

from app.schemas import Menu, MenuItem, OrderStatus

logger = logging.getLogger(__name__)


class KitchenClient:
    """Обёртка над Partner API платформы

    Ключ заведения уходит в заголовке X-Api-Key со всеми запросами
    """

    def __init__(self, base_url: str, api_key: str, timeout: float = 10.0) -> None:
        self._client = httpx.AsyncClient(
            base_url=base_url.rstrip("/"),
            headers={"X-Api-Key": api_key},
            timeout=timeout,
        )

    async def close(self) -> None:
        """Закрывает соединения клиента"""
        await self._client.aclose()

    async def wait_ready(self, attempts: int = 60, delay: float = 1.0) -> None:
        """Ждёт готовности платформы"""
        last_error: Exception | None = None
        for attempt in range(1, attempts + 1):
            try:
                response = await self._client.get("/health")
                if response.status_code == httpx.codes.OK:
                    return
                last_error = RuntimeError(f"health вернул {response.status_code}")
            except httpx.HTTPError as error:
                last_error = error
            logger.warning("платформа не готова, попытка %d: %s", attempt, last_error)
            await asyncio.sleep(delay)
        raise RuntimeError(f"платформа недоступна после {attempts} попыток: {last_error}")

    async def update_profile(self, webhook_url: str, status: str = "active") -> dict[str, Any]:
        """Сообщает платформе адрес для webhook-ов и текущий статус"""
        return await self._request(
            "PATCH", "/partner/v1/me", json={"webhook_url": webhook_url, "status": status}
        )

    async def set_status(self, status: str) -> dict[str, Any]:
        """Ставит заведение на паузу или снимает с неё"""
        return await self._request("PATCH", "/partner/v1/me", json={"status": status})

    async def sync_menu(self, menu: Menu) -> dict[str, Any]:
        """Полностью синхронизирует меню с платформой"""
        payload = {
            "categories": [
                {
                    "name": category.name,
                    "sort_order": category.sort_order,
                    "items": [
                        {
                            "external_id": item.external_id,
                            "name": item.name,
                            "description": item.description,
                            "price": item.price,
                            "stock_qty": item.stock_qty,
                            "is_available": item.stock_qty is None or item.stock_qty > 0,
                        }
                        for item in category.items
                    ],
                }
                for category in menu.categories
            ]
        }
        return await self._request("PUT", "/partner/v1/menu", json=payload)

    async def push_item(self, item: MenuItem) -> dict[str, Any]:
        """Отправляет платформе изменившийся остаток или цену позиции"""
        return await self._request(
            "PATCH",
            f"/partner/v1/menu/items/{item.external_id}",
            json={
                "stock_qty": item.stock_qty,
                "price": item.price,
                "is_available": item.stock_qty is None or item.stock_qty > 0,
            },
        )

    async def accept_order(
        self, order_id: str, expected_at: datetime | None = None
    ) -> dict[str, Any]:
        """Принимает заказ в работу"""
        payload: dict[str, Any] = {}
        if expected_at is not None:
            payload["expected_at"] = expected_at.isoformat()
        return await self._request("POST", f"/partner/v1/orders/{order_id}/accept", json=payload)

    async def reject_order(self, order_id: str, reason: str) -> dict[str, Any]:
        """Отклоняет заказ с причиной"""
        return await self._request(
            "POST", f"/partner/v1/orders/{order_id}/reject", json={"reason": reason}
        )

    async def update_order_status(self, order_id: str, status: OrderStatus) -> dict[str, Any]:
        """Продвигает заказ по цепочке готовки и доставки"""
        return await self._request(
            "POST", f"/partner/v1/orders/{order_id}/status", json={"status": status.value}
        )

    async def list_orders(self, status: OrderStatus | None = None) -> list[dict[str, Any]]:
        """Забирает заказы опросом, запасной канал к webhook-ам"""
        params = {"status": status.value} if status is not None else None
        payload = await self._request("GET", "/partner/v1/orders", params=params)
        items = payload.get("items", [])
        return items if isinstance(items, list) else []

    async def _request(
        self,
        method: str,
        url: str,
        json: dict[str, Any] | None = None,
        params: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        response = await self._client.request(method, url, json=json, params=params)
        if response.status_code >= httpx.codes.BAD_REQUEST:
            raise KitchenError(method, url, response.status_code, response.text)
        payload = response.json()
        return payload if isinstance(payload, dict) else {}


class KitchenError(RuntimeError):
    """Платформа ответила ошибкой"""

    def __init__(self, method: str, url: str, status_code: int, body: str) -> None:
        super().__init__(f"{method} {url} вернул {status_code}: {body}")
        self.status_code = status_code
        self.body = body
