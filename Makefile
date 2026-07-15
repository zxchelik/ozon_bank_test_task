GOOSE_VERSION ?= v3.27.1
export GOOSE_VERSION

.DEFAULT_GOAL := help

.PHONY: help run build fmt test test-race vet check docker-build postgres-up postgres-down db-up migrate-up migrate-down migrate-status clean

help:
	@printf '%s\n' \
		'make run             Run with in-memory storage' \
		'make build           Build bin/shortener' \
		'make fmt             Format Go files' \
		'make test            Run unit tests' \
		'make test-race       Run race-sensitive tests' \
		'make vet             Run go vet' \
		'make check           Run all checks' \
		'make docker-build    Build the application image' \
		'make postgres-up     Start PostgreSQL, migrations and app' \
		'make postgres-down   Stop Compose services' \
		'make db-up           Start only PostgreSQL' \
		'make migrate-up      Apply Goose migrations' \
		'make migrate-down    Roll back one Goose migration' \
		'make migrate-status  Show Goose migration status' \
		'make clean           Remove local build artifacts'

run:
	go run ./cmd/shortener

build:
	mkdir -p bin
	go build -o bin/shortener ./cmd/shortener

fmt:
	gofmt -w cmd internal

test:
	go test ./...

test-race:
	go test -race ./internal/storage/memory ./internal/service ./internal/handler

vet:
	go vet ./...

check: test test-race vet

docker-build:
	docker build -t ozon-shortener .

postgres-up:
	docker compose up --build app

postgres-down:
	docker compose down

db-up:
	docker compose up -d db

migrate-up:
	docker compose run --build --rm migrate up

migrate-down:
	docker compose run --build --rm migrate down

migrate-status:
	docker compose run --build --rm migrate status

clean:
	rm -rf bin
