# MOEX Pulse — realtime edge→cloud signal layer (Yandex Cloud)

Russian translation: [20260927-moex-pulse.ru.md](20260927-moex-pulse.ru.md). This file is canonical.

## Overview

A cloud/distribution layer on top of the existing `moex-trader`: it serves, in realtime over WebSocket,
a stream of "news → sentiment → ticker" plus BUY/SELL/HOLD trading signals, along with an archive and dashboards.

**The problem it solves.** The main goal is to learn Yandex Cloud in depth and pass the
*Certified Engineer Associate* certification (6 domains: infrastructure, data, DevOps,
serverless, security, billing). We build a live highload product that the author uses
personally (they trade on MOEX), which is more effective than dry study of the services.

**How it integrates.** Heavy inference stays on the home GPU box `192.168.88.193`
(Ollama sentiment + the ML ensemble, all already in `moex-trader`). The cloud is the plane for intake,
storage, realtime distribution and observability. A new edge publisher (a command in this same repo)
reads the output of the existing internal packages and pushes ready events to the cloud.

**Key decision:** the project lives inside `moex-trader`, not in a separate repository:
the ISS client (`internal/ingestion`), the bus (`internal/bus`), the signal ensemble
(`internal/orchestrator`, `internal/model`) and news sentiment (`cmd/newsscore` and
related) are already here. The cloud layer reuses them directly, with no cross-repo contracts.

**Scale (by the author's decision):** this plan = a thin serverless MVP (Functions + API Gateway
+ YDB + Object Storage). The highload layer (Kafka, ClickHouse, k8s + autoscaling, load tests) is
a separate next plan, see Post-Completion.

## Context (from discovery)

- **Reusable packages (donors, the same Go module):**
  - `internal/ingestion` — MOEX ISS API client.
  - `internal/orchestrator` + `internal/model` — building the BUY/SELL/HOLD signal.
  - `cmd/newsscore` / `cmd/newsfetch` / news `internal/*` — sentiment per ticker.
  - `internal/bus` — Redis Streams pattern (useful in Phase 2).
  - `internal/metrics` — Prometheus metrics.
  - `internal/config` — configuration.
  - `deploy/*.plist` — launchd pattern for running the edge publisher on `.193`.
- **Infrastructure template (infra only, not domain code):**
  `/Users/olegsidorkin/dev/system/fincontext-mcp/infra/*.tf` (Functions+API Gateway+YDB+
  Lockbox), `src/sigv4.js` (SigV4 signer for the YMQ/YDB API), `src/lockbox.js` (Lockbox pattern).
- **Repo conventions:** plans in `docs/plans/YYYYMMDD-*.md`, finished ones move to
  `docs/plans/completed/`; ralphex/revmux scaffolding is present.
- **New project directories:** `cmd/pulseedge/`, `cloud/functions/{ingest,quotes,ws}/`,
  `cloud/infra/`, `cloud/web/`, `internal/pulse/{event,sigv4,transport}/`.

## Development Approach

- **Testing approach:** Regular (implementation first, then tests, as in the rest of `moex-trader`).
- Complete each task fully before the next; small focused changes.
- **CRITICAL: every task with code MUST include new/updated tests** (success +
  error/edge scenarios), tests are separate checklist items, not bundled with the implementation.
- **CRITICAL: all tests pass before the next task starts.**
- Terraform tasks use `terraform fmt -check` + `terraform validate` instead of unit tests.
- **CRITICAL: update this plan when the scope changes.**
- No comments in code (the author's global rule). The language of new code is Go.

## Testing Strategy

- **Unit tests:** mandatory for every code task (Go `testing`, table-driven).
- **Infra "tests":** `terraform fmt -check` + `terraform validate` for `cloud/infra/`.
- **E2E:** the project has no UI e2e framework; the end-to-end check box→cloud→browser is performed
  manually against the YC sandbox and is moved to Post-Completion (requires cloud credentials).
- All tests must pass before moving on to the next task.

## Progress Tracking

- Mark completed items `[x]` immediately.
- New tasks get the prefix ➕. Blockers get the prefix ⚠️.
- Keep the plan in sync with the actual work; edit the plan when the scope deviates.

## What Goes Where

- **Implementation Steps** (`[ ]`): everything the agent does in this repo — Go code, tests,
  Terraform files (`validate`), a static client, documentation.
- **Post-Completion** (no checkboxes): a real `terraform apply` in YC, Functions deployment,
  end-to-end E2E against the cloud, Phase 2 (highload) as the next plan.

## Implementation Steps

### Task 1: Event domain model (internal/pulse/event)
- [ ] create the package `internal/pulse/event` with the `Event` struct (ticker, sentiment score/label,
      signal BUY/SELL/HOLD, source, produced_at, staleness), source constants
- [ ] implement JSON serialization/deserialization and validation (required fields, ranges)
- [ ] implement a deterministic event `ID()` (sha256 of the tuple ticker+produced_at+source)
- [ ] write serialization and validation tests (success)
- [ ] write error tests (empty ticker, broken label, future timestamp)
- [ ] run the tests — they must pass before Task 2

### Task 2: Request signer for the cloud (internal/pulse/sigv4)
- [ ] create the package `internal/pulse/sigv4`, port the signing logic from
      `fincontext-mcp/src/sigv4.js` to Go (canonical request, string-to-sign, signing key)
- [ ] implement `Sign(req, credentials, region, service)` for HTTP requests
- [ ] write tests against known-answer vectors (the signature matches the reference)
- [ ] write error tests (missing credentials, empty payload)
- [ ] run the tests — they must pass before Task 3

### Task 3: HTTP transport to the cloud (internal/pulse/transport)
- [ ] create the package `internal/pulse/transport` — a client that sends an `Event` to the cloud
      ingest endpoint (API Gateway), with the signature from Task 2 and a key from the config
- [ ] add retries with backoff and timeouts; an interface for mocking in tests
- [ ] write tests of successful sending (mock server) and of retries
- [ ] write error tests (5xx, timeout, signature failure)
- [ ] run the tests — they must pass before Task 4

### Task 4: Edge publisher (cmd/pulseedge)
- [ ] create the command `cmd/pulseedge` that reads the signal from `internal/orchestrator`/`internal/model`
      and the sentiment from the news pipeline (`cmd/newsscore`/related internal packages)
- [ ] assemble the `Event` and send it through `internal/pulse/transport`; config through `internal/config`
- [ ] add Prometheus metrics through `internal/metrics` (publish count/latency/errors)
- [ ] add a launchd plist to `deploy/` modelled on the existing ones (run on `.193`)
- [ ] write tests of assembling the event from signal+sentiment (success + missing data)
- [ ] run the tests — they must pass before Task 5

### Task 5: Cloud Function ingest (cloud/functions/ingest)
- [ ] create the Go function `cloud/functions/ingest`: signature check, `Event` validation
- [ ] write the latest per-ticker state to YDB; the raw event archive to Object Storage
- [ ] storage interfaces (YDB/S3) for mocking in tests
- [ ] write handler tests (valid event → write; invalid signature → 401)
- [ ] write error tests (broken payload, storage unavailable)
- [ ] run the tests — they must pass before Task 6

### Task 6: Cloud Function quotes-poller (cloud/functions/quotes)
- [ ] create the Go function `cloud/functions/quotes` for a timer trigger: pulls MOEX ISS quotes
      through `internal/ingestion`, writes freshness to YDB
- [ ] handling of ISS delay/empty response, idempotent writes
- [ ] write tests of ISS response parsing and writing (success)
- [ ] write error tests (empty/broken ISS response, YDB error)
- [ ] run the tests — they must pass before Task 7

### Task 7: Cloud Function WebSocket distribution (cloud/functions/ws)
- [ ] create the Go function `cloud/functions/ws` for the API Gateway WebSocket:
      connect/message/disconnect, connection registry in YDB
- [ ] broadcast a new event to connected clients (fanout through the registry)
- [ ] write connection lifecycle tests (connect→register, disconnect→cleanup)
- [ ] write broadcast tests (the event is sent to active connections, dead ones are skipped)
- [ ] run the tests — they must pass before Task 8

### Task 8: MVP Terraform infrastructure (cloud/infra)
- [ ] describe a least-privilege service account + IAM roles, VPC
- [ ] describe YDB serverless, an Object Storage bucket (archive + static files), a KMS encryption key
- [ ] describe API Gateway (HTTP ingest + WebSocket), three Cloud Functions, the quotes timer trigger
- [ ] describe Lockbox (edge publisher key, source tokens) and a Monitoring dashboard,
      Container Registry (a stub for Phase 2); take the template from `fincontext-mcp/infra`
- [ ] run `terraform fmt -check` and `terraform validate` — no errors before Task 9

### Task 9: Static web client (cloud/web)
- [ ] create a static page: WebSocket subscription, a "news→sentiment→ticker" feed,
      current signals per ticker
- [ ] add an explicit disclaimer "not investment advice"
- [ ] design per the global rules (SaaS direction, tokens, WCAG AA, contrast, ≥14px)
- [ ] add a smoke test of rendering/parsing a WS message (if there is a JS runner) or
      document the manual check in the README (fails until deployment — note it)
- [ ] run the available tests — they must pass before Task 10

### Task 10: Verify acceptance criteria
- [ ] check that all requirements from the Overview are implemented (event→ingest→YDB→WS→client)
- [ ] run the full unit test suite of the repo
- [ ] run `terraform validate` for `cloud/infra`
- [ ] run the linter — fix all findings
- [ ] check test coverage against the project standard

*Manual E2E against the cloud and deployment are in Post-Completion (not automated by the agent).*

### Task 11: [Final] Documentation
- [ ] add `cloud/README.md` (architecture, deployment, edge publisher config)
- [ ] update the root README/repo docs: a section about MOEX Pulse and running on `.193`

## Technical Details

- **Event model:** `{ticker, sentiment:{score,label}, signal:BUY|SELL|HOLD, source,
  produced_at, staleness_sec}`; money/score — fixed precision; the ID is deterministic.
- **MVP transport:** HTTPS to API Gateway → ingest Function; authorization by SigV4 signature +
  a key from Lockbox. (Phase 2: replace with YMQ/Kafka.)
- **Storage:** YDB serverless — latest per-ticker state + the WS connection registry;
  Object Storage — the raw event archive (later a source for ClickHouse).
- **MOEX data:** free ISS (`iss.moex.com`, ~15 min delay for shares) — enough
  for a self+friends MVP; paid realtime is a Phase 2 config point.
- **News:** we reuse what is already set up in the repo's news pipeline.

## Post-Completion

*Requires manual intervention or external systems — no checkboxes, informational.*

**Manual check / deployment:**
- `terraform apply` in a test Yandex Cloud folder, deploy the three Functions and API Gateway.
- Run `cmd/pulseedge` on `.193` (launchd) against the cloud sandbox.
- End-to-end E2E: an event on `.193` → written to YDB → realtime update in the browser.
- Check the Monitoring dashboard (ingest latency/errors) and billing.

**Next plan — Phase 2 (highload, a separate `docs/plans/`):**
- Managed Kafka as the event backbone (edge → topic → consumers), or Data Streams + YMQ.
- ClickHouse (Managed) for history + DataLens dashboards.
- Managed Kubernetes: WS fanout as a stateful service, cross-pod fanout through Managed Redis,
  Application Load Balancer + Certificate Manager, instance groups/HPA + autoscaling,
  images from Container Registry.
- Data Transfer (YDB↔ClickHouse replication).
- Load testing (k6 / Yandex Load Testing): thousands of WS connections + high-frequency ingest,
  checking autoscaling and alerts.
- Goal: cover all 6 certification domains in practice (keep a checklist of services).
