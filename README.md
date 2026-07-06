# Loqui Backend

Welcome to the backend of Loqui a free, open chat platform (Discord-like).
Currently written in Go with Postgres being the DB.

(might change as development goes on)

## Stack

- Go (REST to send, WebSocket gateway to receive)
- Postgres via `pgx`
- Snowflake int64 IDs (time-sortable; pagination keys off them)

## Layout

```md
cmd/server - server entrypoint
cmd/migrate - migration CLI (up / down / status)
internal/config - env-based configuration
internal/logging - slog logger
internal/snowflake - 64-bit ID generator (fully tested)
internal/db - pool + hand-rolled transactional migrator
internal/db/migrations - \*.sql migration files
```

## Identity model

A user is `username#discriminator`:

- username: case-insensitive unique, case preserved for display, no `#`
- discriminator: 4 chars base62 (`A-Za-z0-9`), **case-sensitive**, unique only
  _within_ a username

So `user#0000` and `otheruser#0000` both exist, and `User#aBcD` and `User#AbCd`
are distinct accounts. Email is optional and, when set, unique case-insensitively

## Quickstart

Requires Go 1.26+ and Docker.

## Nix users

A flake provides the whole toolchain (Go 1.26, `psql`/`pg_isready`, linters, delve)
and drops you into fish:

```sh
nix develop        # enter the dev shell (fish)
# or, with direnv:
direnv allow       # auto-loads the shell on cd
```

The Docker daemon is expected from your system config
(`virtualisation.docker.enable = true`); the flake only comes with the compose CLI

```sh
cp .env.example .env
make db-up                                    # start postgres
go mod tidy                                   # resolve dependencies
mkdir -p secrets                              # create folder for jwt
go run ./cmd/genkey > secrets/jwt_ed25519.pem # create secret in dir
make migrate-up                               # create db tables
make run                                      # start the server
curl localhost:8080/healthz                   # -> ok
```

`make migrate-status` shows applies vs pending. `make migrate-down` rolls back
latest-migration.

## Migrations

Plain `.sql` files named `NNNN_name.sql`, split into up/down sections

```sql
-- +migrate up
create table ...;
-- +migrate down
drop table ...;
```

Each migration runs in its own transaction with its bookkeeping insert, so a
failure leaves no partial states

## Next on the TODO

Channels: create and list, giving messages a so called home
