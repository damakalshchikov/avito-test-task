"""Конфигурация сервиса заведения из переменных окружения с префиксом RESTAURANT_"""

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_prefix="RESTAURANT_")

    http_port: int = 8090
    kitchen_base_url: str = "http://localhost:8080"
    api_key: str = "demo-restaurant-key"
    public_url: str = "http://localhost:8090"
    auto_accept: bool = True
    simulation_step_seconds: float = 5.0
    menu_path: str = "data/menu.json"


settings = Settings()
