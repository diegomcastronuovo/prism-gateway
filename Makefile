.PHONY: help dev build test lint migrate up down logs restart clean

help:
	@echo "PrismGateway — available targets"
	@echo ""
	@echo "  up        Start all services (gateway + postgres + redis)"
	@echo "  down      Stop all services"
	@echo "  logs      Tail gateway logs"
	@echo "  restart   Restart gateway container"
	@echo "  clean     Stop and remove volumes"
	@echo ""
	@echo "  build     Build gateway binary (backend/)"
	@echo "  test      Run backend tests"
	@echo "  lint      Run golangci-lint"
	@echo "  migrate   Run DB migrations"
	@echo "  dev       Run gateway locally (requires postgres + redis)"

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f gateway

restart:
	docker compose restart gateway

clean:
	docker compose down -v

build:
	cd backend && go build -o bin/gateway ./cmd/gateway

test:
	cd backend && go test ./...

lint:
	cd backend && golangci-lint run ./...

migrate:
	cd backend && go run ./cmd/migrate
