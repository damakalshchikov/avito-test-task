"""Схемы обмена с платформой Авито.Кухня"""

from datetime import datetime
from enum import StrEnum

from pydantic import BaseModel, Field


class OrderStatus(StrEnum):
    """Статусы заказа на платформе"""

    NEW = "new"
    ACCEPTED = "accepted"
    COOKING = "cooking"
    READY = "ready"
    DELIVERING = "delivering"
    DELIVERED = "delivered"
    REJECTED = "rejected"
    CANCELLED = "cancelled"


class WebhookEventType(StrEnum):
    """События, которые платформа присылает заведению"""

    ORDER_CREATED = "order.created"
    ORDER_CANCELLED = "order.cancelled"


class MenuItem(BaseModel):
    """Позиция меню в учётной системе заведения"""

    external_id: str
    name: str
    description: str = ""
    price: int
    stock_qty: int | None = None
    image_url: str | None = None

    def available(self, quantity: int) -> bool:
        """Можно ли отдать указанное количество прямо сейчас"""
        if self.stock_qty is None:
            return True
        return self.stock_qty >= quantity


class MenuCategory(BaseModel):
    """Категория меню"""

    name: str
    sort_order: int = 0
    items: list[MenuItem]


class Menu(BaseModel):
    """Меню заведения, загружаемое из data/menu.json"""

    categories: list[MenuCategory]


class OrderItem(BaseModel):
    """Позиция заказа, пришедшего с платформы"""

    menu_item_id: str
    external_id: str
    name: str
    price: int
    quantity: int


class OrderCustomer(BaseModel):
    """Контакты заказчика"""

    name: str
    phone: str = ""


class Order(BaseModel):
    """Заказ в том виде, в котором его видит заведение"""

    id: str
    number: int
    status: OrderStatus
    items: list[OrderItem]
    delivery_address: str
    comment: str = ""
    customer: OrderCustomer | None = None
    subtotal: int
    delivery_fee: int
    total: int
    created_at: datetime | None = None
    updated_at: datetime | None = None


class WebhookEvent(BaseModel):
    """Тело webhook-а от платформы"""

    event_id: str
    type: WebhookEventType
    occurred_at: datetime
    order: Order


class MenuItemUpdate(BaseModel):
    """Изменение позиции меню через собственный API заведения"""

    stock_qty: int | None = Field(default=None, ge=0)
    price: int | None = Field(default=None, ge=0)
    is_available: bool | None = None


class RejectRequest(BaseModel):
    """Причина отказа при ручной обработке заказа"""

    reason: str = Field(min_length=1, max_length=500)
