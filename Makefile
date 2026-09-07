GOLANGCI_LINT_VERSION ?= v2.13.2
GOLANGCI_LINT ?= $(shell command -v golangci-lint 2>/dev/null)
ifeq ($(GOLANGCI_LINT),)
GOLANGCI_LINT = go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
endif

PY_VENV := restaurant/.venv
PY := $(PY_VENV)/bin/python

.PHONY: help build run test lint lint-go lint-py fmt generate diagrams up down logs smoke py-venv

help: # Список целей
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

build: # Собрать бинарник kitchen
	go build -o bin/kitchen ./cmd/kitchen

run: # Запустить kitchen
	go run ./cmd/kitchen

test: # Unit-тесты
	go test ./... -race -count=1

lint: lint-go lint-py # Все линтеры

lint-go: # golangci-lint
	$(GOLANGCI_LINT) run ./...

fmt: # Форматирование
	gofmt -w ./cmd ./internal
	$(GOLANGCI_LINT) fmt ./...

generate: # Кодогенерация сервера из OpenAPI
	go tool oapi-codegen -config oapi-codegen.yaml api/openapi.yaml

diagrams: # Рендер PlantUML в SVG
	docker run --rm -v "$(CURDIR)/docs/diagrams:/work" -w /work plantuml/plantuml:latest -tsvg -o . "*.puml"

up: # Поднять весь стек
	docker compose up --build -d

down: # Остановить стек и удалить данные
	docker compose down -v

logs: # Логи стека
	docker compose logs -f kitchen restaurant

