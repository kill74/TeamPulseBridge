SHELL := /bin/sh

.PHONY: help verify lint test race run up down logs tidy fmt docs-build docs-serve

help:
	@echo "Targets:"
	@echo "  make fmt      - Format all Go code"
	@echo "  make lint     - Run golangci-lint"
	@echo "  make test     - Run unit tests"
	@echo "  make race     - Run race detector tests"
	@echo "  make verify   - fmt + lint + test + race"
	@echo "  make run      - Run ingestion gateway"
	@echo "  make up       - Start local docker stack"
	@echo "  make down     - Stop local docker stack"
	@echo "  make logs     - Tail docker compose logs"
	@echo "  make tidy     - Run go mod tidy"
	@echo "  make docs-build - Build docs site"
	@echo "  make docs-serve - Serve docs site locally"

fmt:
	cd services/ingestion-gateway && gofmt -w ./cmd ./internal

lint:
	cd services/ingestion-gateway && golangci-lint run ./...

test:
	cd services/ingestion-gateway && go test ./...

race:
	cd services/ingestion-gateway && go test -race ./...

verify: fmt lint test race

run:
	cd services/ingestion-gateway && go run ./cmd/server

up:
	docker compose up -d --build

down:
	docker compose down -v

logs:
	docker compose logs -f --tail=100

tidy:
	cd services/ingestion-gateway && go mod tidy

docs-build:
	cd site-docs && pip install -r requirements.txt && mkdocs build --strict

docs-serve:
	cd site-docs && pip install -r requirements.txt && mkdocs serve
