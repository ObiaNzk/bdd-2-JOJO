.PHONY: up down logs run build tidy

up:
	docker compose up --build -d

down:
	docker compose down -v

logs:
	docker compose logs -f app

run:
	go run ./cmd/server

build:
	CGO_ENABLED=0 go build -trimpath -o bin/server ./cmd/server

tidy:
	go mod tidy
