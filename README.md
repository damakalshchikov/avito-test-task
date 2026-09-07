# Авито.Кухня

> Исходная формулировка в [backend-trainee-assignment-autumn-2026.md](backend-trainee-assignment-autumn-2026.md), план реализации (prompt для ИИ-агента) в [PLAN.md](PLAN.md)

Платформа доставки еды, состоящая из двух сервисов:

| Сервис | Директория | Стек | Назначение |
|---|---|---|---|
| **kitchen** | `cmd/`, `internal/` | Go, chi, pgx, PostgreSQL | Основной API: клиентский *(web)*, партнёрский *(заведения)*, административный |
| **restaurant** | `restaurant/` | Python, FastAPI | Пример заведения, интегрированного с платформой через API и webhook-и |

## Запуск

```bash
docker compose up --build
```

- kitchen: http://localhost:8080/health
- restaurant: http://localhost:8090/health
- postgres: localhost:5434

## Разработка

```bash
make help        # список целей
make lint        # golangci-lint + ruff + mypy
make test        # go test + pytest
make generate    # кодогенерация сервера из api/openapi.yaml
make diagrams    # рендер PlantUML в SVG
```

*Остальные разделы будут появлятся по мере их реализации*
