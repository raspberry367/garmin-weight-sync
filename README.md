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

## Endpoints (planned)

- `POST /api/v1/measurements` — accept body-composition JSON from the Shortcut, sync to
  Garmin. Auth via a shared secret / API key header (the phone is the only client).
- `GET /health` — liveness.

## Notes / open questions

- **Garmin integration:** no official public API for body composition. Common approaches:
  reverse-engineered `garth`/Garmin Connect login, or upload a **FIT file** with weight
  scale data. Decide auth strategy (username/password session vs. token) before building
  `adapter/garmin`.
- **Auth for the API itself:** since the client is a personal Shortcut, a static API key
  in a request header is sufficient. Keep the key in env/config.
- Consider idempotency / dedup on `timestamp` so re-runs of the Shortcut don't create
  duplicate Garmin entries.
