"""Точка входа FastAPI-приложения заведения"""

import asyncio
import logging
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from pathlib import Path

from fastapi import FastAPI

from app import routes, simulator, webhooks
from app.kitchen import KitchenClient
from app.settings import settings
from app.store import store

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
logger = logging.getLogger(__name__)


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    """Подключает заведение к платформе и поднимает симулятор кухни"""
    if getattr(app.state, "kitchen", None) is not None:
        yield
        return

    client = KitchenClient(settings.kitchen_base_url, settings.api_key)
    app.state.kitchen = client

    menu = store.load_menu(Path(settings.menu_path))
    logger.info("меню загружено: категорий %d", len(menu.categories))

    task: asyncio.Task[None] | None = None
    try:
        await client.wait_ready()
        await client.update_profile(webhook_url=f"{settings.public_url}/webhooks/kitchen")
        result = await client.sync_menu(menu)
        logger.info("меню синхронизировано: %s", result)

        task = simulator.start(client, settings.simulation_step_seconds)
    except Exception:
        logger.exception("не удалось подключиться к платформе")

    try:
        yield
    finally:
        if task is not None:
            await simulator.stop(task)
        await client.close()


app = FastAPI(title="Demo Restaurant", version="1.0.0", lifespan=lifespan)
app.include_router(webhooks.router)
app.include_router(routes.router)


@app.get("/health", tags=["service"])
async def health() -> dict[str, str]:
    return {"status": "ok"}


def run() -> None:
    import uvicorn

    uvicorn.run("app.main:app", host="0.0.0.0", port=settings.http_port)  # noqa: S104


if __name__ == "__main__":
    run()
