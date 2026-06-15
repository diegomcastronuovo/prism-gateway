.PHONY: dev test lint build

dev:
	go run ./cmd/gateway -config config.yaml

build:
	go build -o bin/gateway ./cmd/gateway

test:
	go test ./...

lint:
	golangci-lint run ./...

# Docker targets
.PHONY: docker-build docker-up docker-down docker-logs docker-clean docker-restart

docker-build:
	docker compose build

docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f gateway

docker-restart:
	docker compose restart gateway

docker-clean:
	docker compose down -v
	docker system prune -f

# Development with Docker
.PHONY: docker-dev

docker-dev: docker-up
	@echo "Gateway running at http://localhost:8081"
	@echo "Postgres running at localhost:5434"
	@echo "Use 'make docker-logs' to view logs"
	@echo "Use 'make docker-down' to stop services"
