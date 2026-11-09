# multi-tour-worker-departure-deadlines

> Worker job 3: departure deadlines (operations-cost)

Part of the **Multi Tour** distributed system — team `multi-tour`, Grupo 2.
Governance and documentation live in [`multi-tour-docs`](https://github.com/code-corhuila/multi-tour-docs).

## What it is

Worker job 3 (ADR-004 D3, D10): every 15 minutes it asks `multi-tour-operations-cost-api`, through its internal route, to look at the departures of all tenants that passed a deadline. It holds no business rule and no database, and exposes only `/health`. What it does by story is in `multi-tour-docs` (`07-api/specs/operations-cost/multi-tour-worker-departure-deadlines`). The job asks `POST /api/v1/operations-cost/internal/departures/pending-regularization?limit=` (limit = `BATCH_SIZE`) and asks again while the batch comes back full, at most five times per run; a failed ask ends the run, counted, and the next run asks again, because the service marks by state. The call carries the id of the run as `Idempotency-Key` only so that the runtime repeats it after a transient failure; the service ignores it (HU-WRK-003, HU-OPS-003 part B). Starting the saga `cancel-departure` for a tenant that is inactive (HU-WRK-003 AC2) waits for that saga.

## The service token

The worker calls the internal routes with `Authorization: Bearer $SERVICE_TOKEN`, a token of role `SERVICE` that `multi-tour-security` issues for it:

```
security issue-service-token worker-departure-deadlines 720h
```

In `develop`, `multi-tour-infra-postgres/scripts/dev-token.sh worker-departure-deadlines` runs that same command. In `qa` and `main` the value is a secret of the environment, set in `env/qa.env` or `env/main.env`: never in a compose file, an image or the repository, and renewed before it expires.

## How to start it

Copy `.env.example` to `.env` and set `SERVICE_TOKEN`. Then run it with Go 1.26, exporting the variables of `.env` first, or build the image and start it from the root composition, which includes `deploy/compose.yml` (the service is `worker-departure-deadlines`; it publishes no port, only `expose 8080` for `/health`):

```
go run ./cmd/worker
docker compose -f deploy/compose.yml up --build
```

| Variable | Default | Meaning |
|---|---|---|
| `SERVICE_TOKEN` | none, required | Service credential, sent as `Authorization: Bearer` |
| `OPERATIONS_COST_BASE_URL` | none, required (e.g. `http://operations-cost-api:8084`) | Internal base URL of `multi-tour-operations-cost-api`: scheme, host and port only |
| `DEADLINES_INTERVAL` | `15m` | Time between two runs |
| `BATCH_SIZE` | `100` | Departures asked per call at most (the `limit` of the ask) |
| `RUN_TIMEOUT` | `30s` (compose: `60s`) | Time limit of one run |
| `HTTP_TIMEOUT` | `5s` | Time limit of each HTTP attempt |
| `HTTP_ATTEMPTS` | `3` | Attempts per call (network, `429` and `5xx` only) |
| `HEALTH_ADDR` | `:8080` | Where `GET /health` listens |

A missing or malformed variable stops the worker at start and the message names the variable, never its value.

## Where the data is

Nowhere: it keeps no database and no state.

## How to test it

```
gofmt -l .
go vet ./...
go test ./...
```

## Branching

Three permanent branches. **None of them accepts a direct commit** — you enter through a child
branch and leave through a Pull Request.

```
develop  <--PR--  feat/... fix/... chore/...
qa       <--PR--  qa/...
main     <--PR--  release/...  hotfix/...
```

Promotion happens **by re-application** (`git cherry-pick -x`), never by merging one permanent
branch into another: `merge develop -> qa` and `merge qa -> main` do not exist in this model.

`main` requires **1 approval from `ariel5253`**. On `develop` and `qa` the team sets its own review
rule.

Full policy: `00-governance/branching-policy.md` in `multi-tour-docs`.
