ifneq (,$(wildcard .env))
include .env
export
endif

.PHONY: db-up db-down migrate-up migrate-down migrate-status run test tidy

db-up:
	docker compose up -d postgres

db-down:
	docker compose down

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

migrate-status:
	go run ./cmd/migrate status

run:
	go run ./cmd/server

test:
	go test ./...

tidy:
	go mod tidy
