"""Хранилище заведения в памяти. Упрощение MVP: здесь её роль роль играет словарь"""

import json
from pathlib import Path

from app.schemas import Menu, MenuItem, Order, OrderStatus


class Store:
    """Меню, заказы и обработанные события заведения"""

    def __init__(self) -> None:
        self._menu: Menu = Menu(categories=[])
        self._items: dict[str, MenuItem] = {}
        self._orders: dict[str, Order] = {}
        self._handled_events: set[str] = set()
        self.paused: bool = False

    def load_menu(self, path: Path) -> Menu:
        """Читает меню из файла и строит индекс позиций по external_id"""
        raw = json.loads(path.read_text(encoding="utf-8"))
        self._menu = Menu.model_validate(raw)
        self._items = {
            item.external_id: item for category in self._menu.categories for item in category.items
        }
        return self._menu

    @property
    def menu(self) -> Menu:
        """Текущее меню"""
        return self._menu

    def item(self, external_id: str) -> MenuItem | None:
        """Позиция меню по идентификатору заведения"""
        return self._items.get(external_id)

    def items(self) -> list[MenuItem]:
        """Все позиции меню"""
        return list(self._items.values())

    def update_item(
        self,
        external_id: str,
        stock_qty: int | None = None,
        price: int | None = None,
    ) -> MenuItem | None:
        """Меняет остаток или цену позиции"""
        item = self._items.get(external_id)
        if item is None:
            return None
        if stock_qty is not None:
            item.stock_qty = stock_qty
        if price is not None:
            item.price = price
        return item

    def reserve(self, order: Order) -> None:
        """Списывает остатки принятого заказа со склада заведения"""
        for line in order.items:
            item = self._items.get(line.external_id)
            if item is not None and item.stock_qty is not None:
                item.stock_qty = max(0, item.stock_qty - line.quantity)

    def release(self, order: Order) -> None:
        """Возвращает остатки отменённого заказа"""
        for line in order.items:
            item = self._items.get(line.external_id)
            if item is not None and item.stock_qty is not None:
                item.stock_qty += line.quantity

    def unavailable_items(self, order: Order) -> list[str]:
        """Позиции заказа, которые заведение не может приготовить"""
        missing: list[str] = []
        for line in order.items:
            item = self._items.get(line.external_id)
            if item is None or not item.available(line.quantity):
                missing.append(line.external_id)
        return missing

    def save_order(self, order: Order) -> None:
        """Сохраняет или обновляет заказ"""
        self._orders[order.id] = order

    def order(self, order_id: str) -> Order | None:
        """Заказ по идентификатору платформы"""
        return self._orders.get(order_id)

    def orders(self, status: OrderStatus | None = None) -> list[Order]:
        """Заказы заведения, при необходимости отфильтрованные по статусу"""
        found = list(self._orders.values())
        if status is not None:
            found = [order for order in found if order.status is status]
        return sorted(found, key=lambda order: order.number)

    def set_status(self, order_id: str, status: OrderStatus) -> Order | None:
        """Обновляет статус сохранённого заказа"""
        order = self._orders.get(order_id)
        if order is None:
            return None
        order.status = status
        return order

    def mark_handled(self, event_id: str) -> bool:
        """Помечает событие обработанным. Возвращает False, если событие уже приходило"""
        if event_id in self._handled_events:
            return False
        self._handled_events.add(event_id)
        return True


store = Store()
