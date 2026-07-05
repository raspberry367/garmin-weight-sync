# garmin-weight-sync

## Purpose

HTTP API that receives body-composition data from an **Apple Shortcut** (running on the
user's iPhone) and syncs it to **Garmin Connect**. The Shortcut reads metrics from Apple
Health and POSTs them to this service.

Payload fields sent by the phone:
- `bmi` — Body Mass Index
- `fat_percentage` — body fat %
- `lean_body_mass` — lean body mass (kg)
- `weight` — body weight %

## Tech stack

- **Language:** Go (1.26)
- **Web framework:** [Fiber](https://github.com/gofiber/fiber) v3
- **Architecture:** Clean Architecture (dependency rule points inward: domain has no
  outward imports; infra/transport depend on domain)

## Layout (clean architecture)

```
cmd/
  server/
    main.go              # wiring: build deps, start Fiber, DB connections
internal/
  domain/                # entities + repository/service INTERFACES. Zero framework imports.
    measurement.go       # BodyComposition entity + validation
    ports.go             # interfaces: MeasurementSyncer, MeasurementRepository
  usecase/               # application logic. Depends only on domain.
    sync_measurement.go  # orchestrates validate -> persist/sync
  adapter/
    http/                # Fiber handlers, DTOs, request mapping. Depends on usecase.
      handler.go
      dto.go
      router.go
    db/                  # MySQL repository implementation
      mysql.go
    garmin/              # Garmin Connect client implementing domain port
      client.go
config/
  config.go              # env-based config (port, DB config, garmin creds)
```

Dependency direction: `adapter -> usecase -> domain`. Domain imports nothing from the
outer layers. Wire concrete implementations in `cmd/server/main.go` only.

## Conventions

- Handlers are thin: decode DTO -> map to domain input -> call usecase -> map result to
  HTTP response. No business logic in handlers.
- Usecases take **interfaces** (ports), never concrete types. Enables test doubles.
- Domain entities own their own validation (`Validate() error`).
- Errors: domain returns typed/sentinel errors; adapter layer maps them to HTTP status.
- Config via env vars, loaded once at startup. No global mutable state.
- Units are explicit in field names/comments (kg, %) to avoid Apple Health / Garmin
  unit mismatches.

## Deploying locally with Docker

1. Copy `.env.example` to `.env` and fill in `GARMIN_USERNAME`, `GARMIN_PASSWORD`, and
   `API_KEY` (the Shortcut must send this value in the `X-API-Key` header — the intake
   endpoint rejects any request without a matching header). Set `TELEGRAM_BOT_TOKEN` /
   `TELEGRAM_CHAT_ID` too if you want sync alerts.
2. `make up` — builds the app image and starts it alongside a MySQL 8.4 container
   (`docker-compose.yml`). The app waits for MySQL's healthcheck before starting and
   runs its own schema migrations on boot.
3. `make garmin-login` — runs the separate `cmd/garmin-login` binary
   (`docker compose exec -it app /app/garmin-login`), a one-off **interactive** CLI that
   logs in to Garmin Connect, prompts you on the terminal for an MFA code if Garmin asks
   for one, and caches the resulting OAuth1 token pair to `./data/garmin_token.json`.
   Do this once before relying on auto-sync — the unattended server can reuse a cached
   token but can't complete an MFA prompt itself (no human to answer it). Without a
   seeded token, the first sync attempt hits `ErrMFARequired` and alerts (if Telegram is
   configured) instead of completing. Re-run it any time the cached token gets rejected.
4. Point the Apple Shortcut at `http://<host>:3000/api/v1/measurements` with the
   `X-API-Key` header set to your configured `API_KEY`.

**Is a separate cron needed? No.** The server has a built-in sync loop
(`runGarminSyncLoop` in `cmd/server/main.go`) that runs immediately on startup and then
every `SYNC_INTERVAL_MINUTES` (default 60) for as long as the container is running — no
host crontab or external scheduler required. `cmd/garmin-login` (step 3) is a separate,
manually-run binary, not part of that loop — it exists only to seed/refresh the token
cache when MFA is involved.

```
make down            # stop containers
make logs            # tail container logs
make mysql-shell      # open interactive MySQL shell inside the DB container
make clean            # stop and delete volumes (wipes the DB)
```

## Commands & Docker Orchestration

Run commands locally:
```
go run ./cmd/server        # run locally
go build ./...             # build all
go test ./...              # run tests
go vet ./...               # vet
```

Or manage services in Docker using **Makefile** shortcuts:
```
make up                    # start app & mysql containers in background
make down                  # stop containers
make build                 # rebuild images
make logs                  # view container logs in real time
make mysql-shell           # open interactive MySQL terminal inside DB container
make clean                 # clean stop and purge volumes
```

## Endpoints

- `POST /api/v1/measurements` — accepts body-composition JSON from the Shortcut, saves
  it, and lets the background Garmin sync loop pick it up. Requires an `X-API-Key`
  header matching `API_KEY`.
- `GET /health` — liveness, no auth required.

## Notes / open questions

- **Garmin integration:** no official public API for body composition. Common approaches:
  reverse-engineered `garth`/Garmin Connect login, or upload a **FIT file** with weight
  scale data. Decide auth strategy (username/password session vs. token) before building
  `adapter/garmin`.
- **Auth for the API itself:** since the client is a personal Shortcut, a static API key
  in a request header is sufficient. Keep the key in env/config.
- Consider idempotency / dedup on `timestamp` so re-runs of the Shortcut don't create
  duplicate Garmin entries.
