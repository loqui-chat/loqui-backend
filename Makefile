ifneq (,$(wildcard .env))
include .env
export
endif

.PHONY: up down logs db-up db-down migrate-up migrate-down migrate-status run test tidy genkey

up:
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f api

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

genkey:
	go run ./cmd/genkey
