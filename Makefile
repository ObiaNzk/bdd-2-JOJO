.PHONY: install up down logs run console build tidy

# Browser opener (macOS: open, Linux: xdg-open)
OPEN := $(shell command -v open >/dev/null 2>&1 && echo open || echo xdg-open)

# One web UI per database, for running queries
UIS := http://localhost:8081 http://localhost:8082 http://localhost:8083 http://localhost:7474

# Pull the external Docker images (4 DBs + 3 web UIs) and build the app image,
# so the first `make up` doesn't have to download everything.
install:
	@echo "==> Pulling Docker images (databases + web UIs)"
	docker compose --profile tools pull postgres mongo redis neo4j adminer mongo-express redis-commander
	@echo "==> Building the app image"
	docker compose build app
	@echo "==> Done. Levantá todo con: make up"

up:
	docker compose --profile tools up --build -d
	@echo "Waiting for web UIs to be ready..."
	@sleep 5
	@for url in $(UIS); do $(OPEN) $$url; done

down:
	docker compose down -v

logs:
	docker compose logs -f app

run:
	go run ./cmd/server

console:
	go run ./cmd/console

build:
	CGO_ENABLED=0 go build -trimpath -o bin/server ./cmd/server

tidy:
	go mod tidy
