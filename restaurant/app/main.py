"""Точка входа FastAPI-приложения заведения"""

from fastapi import FastAPI

from app.settings import settings

app = FastAPI(title="Demo Restaurant", version="0.1.0")


@app.get("/health")
async def health() -> dict[str, str]:
    return {"status": "ok"}


def run() -> None:
    import uvicorn

    uvicorn.run("app.main:app", host="0.0.0.0", port=settings.http_port)  # noqa: S104


if __name__ == "__main__":
    run()
