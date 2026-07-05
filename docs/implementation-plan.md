# Implementation Plan — Garmin Connect Weight Sync (Go)

Step-by-step plan to build the Garmin Connect write path in this Go service.
Each step links back to the protocol analysis in
[garmin-connect-mechanism.md](./garmin-connect-mechanism.md) (`§` = section).

## Goal

Wire the existing HTTP intake (`internal/adapter/http`) to a real Garmin Connect
uploader so that a `BodyComposition` measurement becomes a FIT `File.Weight`
uploaded to Garmin. Preserve the hexagonal layout: `domain` owns ports,
`adapter/garmin` implements them.

## Current State (baseline)

- `domain.BodyComposition` + `Validate()` — [measurement.go](../internal/domain/measurement.go)
- Ports `MeasurementSyncer` / `MeasurementRepository` — [ports.go](../internal/domain/ports.go)
- HTTP handler logs payloads but does **not** sync yet — [handler.go](../internal/adapter/http/handler.go)
- Config has `GarminUsername` / `GarminPassword` (currently commented out) — [config.go](../config/config.go)
- Fiber v3, Go 1.26.

## Target Package Layout

```
internal/
  domain/                 # unchanged ports + BodyComposition
  adapter/
    http/                 # existing intake; wire syncer in
    garmin/
      client.go           # orchestrator: Sync(BodyComposition) -> upload
      auth.go             # SSO/CAS login (§3.2)
      oauth.go            # OAuth1 sign + OAuth2 exchange (§3.2 steps 3-4)
      fit.go              # encode BodyComposition -> FIT File.Weight (§5.1)
      upload.go           # multipart POST to upload-service (§6)
      tokenstore.go       # persist/reuse OAuth1 token pair (§3.4)
      urls.go             # endpoint constants (§9)
config/                   # add profile + token-cache path
```

---

## Step 0 — Dependencies & Scaffolding

Add libraries and empty package.

- FIT encoding: evaluate `github.com/tormoder/fit` — **verify it can *encode*
  `File.Weight` + `WeightScaleMesg`** (its reader is strong, writer weaker). If
  not, fall back to a hand-rolled FIT encoder (Step 4b).
- OAuth1: `github.com/dghubble/oauth1` (or hand-rolled HMAC-SHA1).
- Create `internal/adapter/garmin/` package with the files above (stubs).

**Verify:** `go build ./...` passes with stubs.

Ref: mechanism §8 (port map).

---

## Step 1 — Endpoint Constants & Config

- `urls.go`: encode all endpoints/constants from mechanism **§9** (domain
  `garmin.com`, SSO embed/signin/MFA, OAuth1 preauthorized, OAuth2 exchange,
  upload URL, `User-Agent = com.garmin.android.apps.connectmobile`, CSRF/ticket
  regexes).
- Consumer key/secret (**§2**): hardcode the well-known pair
  (`fc3e99d2-...` / `E08WAR897...`) as constants; no need for the remote-fetch
  indirection.
- `config.go`: uncomment/enable `GarminUsername`/`GarminPassword`; add
  `GarminProfileGender`, `GarminProfileHeightCm`, `GarminProfileAge` (needed for
  the FIT UserProfile, §5.1), and `TokenCachePath`.

**Verify:** config loads from env; constants compile.

Ref: mechanism §2, §9.

---

## Step 2 — OAuth1 Signing + OAuth2 Exchange (`oauth.go`)

The token half of the flow. Independent of the HTML scraping, so build/test first.

- `signOAuth1Header(method, url, consumerKey, consumerSecret, token, tokenSecret)`
  → OAuth1 `Authorization` header (HMAC-SHA1). Two modes:
  - **request token** (no token/secret) — for the preauthorized call.
  - **protected resource** (with token/secret) — for the exchange call.
- `getOAuth1(ticket)` — GET `/oauth-service/oauth/preauthorized?ticket=...` signed
  as request-token → parse url-encoded `oauth_token` + `oauth_token_secret`
  (mechanism **§3.2 Step 3**).
- `getOAuth2(token, secret)` — POST `/oauth-service/oauth/exchange/user/2.0`
  signed as protected-resource, empty body → JSON bearer
  (`access_token`, `expires_in`) (mechanism **§3.2 Step 4**).
- Track `expiresAt = now + expires_in`; helper `isOAuth2Valid()`.

**Verify:** unit test the OAuth1 signature against a known vector (compare with
`dghubble/oauth1` if hand-rolled).

Ref: mechanism §3.2 steps 3–4, OAuth2Token shape.

---

## Step 3 — SSO / CAS Login (`auth.go`)

The HTML scraping half. Produces a service ticket.

- Shared `http.Client` with `cookiejar.New` (mechanism **§3.2 Step 0**).
- `initCookies()` — GET `/sso/embed` with widget query params.
- `getCSRF()` — GET `/sso/signin`, scrape `_csrf` via regex (**Step 1**).
- `sendCredentials(email, pw, csrf)` — POST url-encoded `/sso/signin`
  (`username,password,embed=true,_csrf`), headers `origin/referer/NK:NT`, follow
  redirects, detect:
  - MFA redirect (`/sso/verifyMFA/loginEnterMfaCode`) → return "MFA needed" +
    scraped mfa CSRF.
  - success → scrape service ticket via ticket regex (**§3.2 Step 2**).
- `completeMFA(code, mfaCsrf)` — POST `/sso/verifyMFA/loginEnterMfaCode` → ticket
  (**§3.2 Step 2b**).
- **Anti-bot (§3.3):** random 10–16s sleeps between steps; detect Cloudflare
  (403 `error code: 1020`, 429). Make the sleep configurable / skippable in tests.

**Verify:** integration test against real Garmin (guarded by env flag) — full cold
login yields a non-empty ticket. Expect ~40–60s.

Ref: mechanism §3.2 steps 0–2b, §3.3.

> MFA note: model as two-phase like the reference — first call returns
> `MFACodeRequested`, caller re-invokes with the code. Decide how the HTTP layer
> surfaces this (Step 7).

---

## Step 4 — FIT File Encoding (`fit.go`)

Turn `BodyComposition` into a FIT `File.Weight` byte slice (mechanism **§5.1**).

Messages, in order:
1. **FileId** — `Type=Weight`, `Manufacturer=Garmin`, `GarminProduct=2429`,
   `SerialNumber=1234`, `TimeCreated=now(UTC)`.
2. **UserProfile** — `Gender`, `Age`, `Weight`, `Height` (**meters** = cm/100),
   `ActivityClass=90`, `MessageIndex=0`, `LocalId=0`.
3. **WeightScale** — `Timestamp(UTC)`, `UserProfileIndex=0`, `Weight`,
   plus optional `PercentFat`, `MuscleMass`/lean mass, `Bmi` (from
   `BodyComposition`). Map our fields:
   - `Weight → Weight`
   - `FatPercentage → PercentFat`
   - `LeanBodyMass → ` (FIT has no direct lean-mass on WeightScale; either derive
     muscle mass or omit — decide during impl)
   - `BMI → Bmi`
   - `Timestamp(ms) → seconds UTC`

- **4b fallback:** if `tormoder/fit` can't encode this, hand-roll the encoder —
  FIT is a header + definition/data records + CRC-16; only 3 message types needed.

**Verify:** encode a sample, then decode it back (round-trip) and/or validate with
the FIT SDK / an online FIT decoder. Confirm byte output is a valid `.fit`.

Ref: mechanism §5.1, DTO field list.

> Skip the dead `DeviceInfoMesg` — the reference builds it but never writes it
> (mechanism §5.1 note).

---

## Step 5 — Upload (`upload.go`)

Single write call (mechanism **§6**).

- `uploadFIT(bearer, fitBytes)` — POST `/upload-service/upload/.fit`,
  `Authorization: Bearer <access_token>`, headers `NK:NT`, `origin`, mobile UA.
- Body: `multipart/form-data`, one part `file`, `application/octet-stream`,
  filename `<date>_sync.fit`.
- Accept status **2xx and 409** (409 = duplicate, benign).
- Parse `detailedImportResult` → `uploadId`; treat failure `code==202` as
  already-uploaded (benign).

**Verify:** with a valid bearer (from Steps 2–3), upload a real FIT and confirm the
measurement appears in Garmin Connect.

Ref: mechanism §6.

---

## Step 6 — Token Store & Orchestrator (`tokenstore.go`, `client.go`)

Tie it together + make it fast on repeat runs (mechanism **§3.4, §7**).

- `tokenstore.go`: persist the **OAuth1** `token`+`secret` (long-lived) to
  `TokenCachePath` (JSON file, 0600). Load on startup.
- `client.go` implements `domain.MeasurementSyncer`:
  ```
  Sync(m *BodyComposition):
    1. if cached OAuth1 pair -> getOAuth2 (fast path, §3.4)
    2. if OAuth2 invalid   -> full login (Step 3) -> OAuth1 -> OAuth2, cache OAuth1
    3. fit := encode(m)      (Step 4)
    4. uploadFIT(bearer, fit) (Step 5)
  ```
  Follows the decision flow in mechanism **§7** (see its flowchart).

**Verify:** first run does full login + caches; second run skips SSO (only
OAuth2 exchange + upload, a few seconds).

Ref: mechanism §3.4, §7.

---

## Step 7 — Wire HTTP Intake → Syncer

- Extend `Handler` to hold a `domain.MeasurementSyncer` (constructor injection).
- In `SyncMeasurement`, after parsing, build a `BodyComposition`, `Validate()`,
  call `syncer.Sync(...)`, map result/errors to `MeasurementResponse`.
- Handle the two-phase MFA case: decide UX — e.g. return `409 + "mfa_required"`
  and accept the code on a follow-up endpoint, or require a pre-seeded token cache
  so interactive MFA is only needed once out-of-band.
- Update `main.go` to construct `garmin.NewClient(cfg)` and inject into the router.

**Verify:** end-to-end — POST a measurement to `/api/v1/measurements`, confirm it
lands in Garmin Connect.

Ref: existing [handler.go](../internal/adapter/http/handler.go), mechanism §7.

---

## Step 8 — Hardening (optional / later)

- Idempotency: use `AppleHealthID` + `MeasurementRepository.ExistsByAppleHealthID`
  to skip duplicates before uploading (409 is the safety net).
- OAuth2 refresh via `refresh_token` instead of re-running OAuth1 exchange.
- Cloudflare fallback: if plain TLS gets 1020-blocked, add JA3 impersonation via
  `utls` (mechanism **§4.6**) — only if needed.
- Consider the multi-strategy DI flow (mechanism **§4**) as a more future-proof
  auth if the legacy CAS flow degrades.

---

## Build Order Summary

| Step | Deliverable | Depends on | Mechanism ref |
|---|---|---|---|
| 0 | deps + stubs | — | §8 |
| 1 | urls + config + consumer keys | 0 | §2, §9 |
| 2 | OAuth1 sign + OAuth2 exchange | 1 | §3.2 (3–4) |
| 3 | SSO/CAS login → ticket | 1 | §3.2 (0–2b), §3.3 |
| 4 | FIT encoder | 1 | §5.1 |
| 5 | upload | 2 | §6 |
| 6 | token store + orchestrator | 2,3,4,5 | §3.4, §7 |
| 7 | HTTP wiring | 6 | §7 |
| 8 | hardening | 7 | §4, §4.6 |

**Critical-path unknowns to de-risk early:** (a) FIT `File.Weight` encoding in Go
(Step 4/4b), (b) surviving Cloudflare on the SSO POST (Step 3/§3.3). Prototype
both before committing to the full build.
