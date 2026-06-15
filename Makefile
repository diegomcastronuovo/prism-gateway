.PHONY: help dev build test lint up down logs restart clean

help:
	@echo "PrismGateway — available targets"
	@echo ""
	@echo "  up        Start all services (gateway + postgres + redis + frontend)"
	@echo "  down      Stop all services"
	@echo "  logs      Tail gateway logs"
	@echo "  restart   Restart gateway container"
	@echo "  clean     Stop and remove all volumes (destructive)"
	@echo ""
	@echo "  build     Build gateway binary (backend/)"
	@echo "  test      Run backend tests"
	@echo "  lint      Run golangci-lint"
	@echo "  dev       Run gateway locally (requires postgres + redis running)"
	@echo ""
	@echo "  Note: DB migrations run automatically on gateway startup."

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

dev:
	cd backend && go run ./cmd/gateway -config config.yaml
