# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

An Olympic Games data-management service built on **polyglot persistence**: a single
Go HTTP API backed by four databases. Postgres is the **source of truth**; Redis,
MongoDB and Neo4j hold *derived views* optimized for specific read use cases. Writes
go to Postgres first and then fan out to the derived stores.

Module path: `github.com/ObiaNzk/bdd-2-JOJO`. Go 1.26.

## Commands

```bash
make up        # full stack (4 DBs + app + web UIs) detached, then opens the 4 UIs in a browser
make down      # tear down AND delete volumes (-v): all data is wiped
make run       # run the app locally (go run ./cmd/server) against dockerized DBs
make console   # interactive admin console (go run ./cmd/console): set up entities + "realize" events
make build     # static binary to bin/server
make logs      # follow app container logs
make tidy      # go mod tidy
./scripts/seed.sh   # seed two editions (Tokyo 2020, Paris 2024); requires the stack up

go test ./...                    # run all tests (none exist yet)
go test ./internal/service -run TestName   # run a single test once they exist
go vet ./...                     # vet
```

To run the app locally against the databases without the app container:
`docker compose up -d postgres mongo redis neo4j && make run`.

## Architecture

Dependency-injected layers, wired in `cmd/server/main.go` (the composition root —
it opens all four DB connections, builds the repositories, injects them into the
service, and injects the service into the HTTP handler):

- `cmd/server/internal/handler` — chi HTTP layer. Declares the `Service` interface it consumes; one route per use case (see `router.go`).
- `internal/service` — business logic. Declares **consumer-side interfaces** (`SQLStore`, `MedalCache`, `ResultStore`, `GraphStore`), orchestrates write fan-out and read composition.
- `internal/repository` — one concrete type per backend (`PostgresRepository`, `RedisRepository`, `MongoRepository`, `Neo4jRepository`) that *implicitly* satisfies the service interfaces.
- `internal/platform/{postgres,mongodb,redis,neo4j}` — thin connection constructors only.
- `internal/model` — plain data carriers + query-result DTOs (no behaviour).
- `internal/config` — env-var config with localhost defaults.

**Key convention:** interfaces are defined by the *consumer* (handler defines what it
needs from service; service defines what it needs from each store), not by the
implementer. When adding a capability, add the method to the consumer interface
*and* the concrete repository — the compiler links them structurally.

### Which database serves what

- **Postgres** — relational source of truth + exact aggregate queries (medals by country/discipline, hosts, exact event popularity).
- **Redis** — leaderboards via sorted sets (`medals:country:{gameID}`, `medals:athlete:{gameID}`) and approximate event popularity via HyperLogLog (`event:{eventID}:countries`).
- **MongoDB** — type-specific event results (`event_results` collection); the `result` payload is intentionally schema-flexible because each `format` records its outcome differently (finishing order for a race, attempts per athlete for a field event, etc.). Persons are embedded as snapshots, and a queryable `records` array flags olympic records — that is how cases 2 and 7 are derived.
- **Neo4j** — full team-centric graph mirroring the relational structure: `Athlete`, `Team`, `Event`, `Discipline`, `Sport`, `OlympicGame`, `Country` and `Medal` nodes, linked by `MEMBER_OF`, `COMPETED_IN`, `WON`, `OF`, `PART_OF`, `IN_GAME` and `REPRESENTS`. The whole team neighbourhood is materialized by `GraphStore.SyncTeamGraph` on every result/medal write; used for graph traversals like "athletes with medals across multiple disciplines".

### Write fan-out (important)

`Service.AwardMedal` / `RegisterResult` write to Postgres, then push to Redis/Mongo/Neo4j
**sequentially and non-atomically**. There is no cross-store transaction or
compensation: if a later step fails, earlier stores are already mutated. Keep this in
mind when adding writes — derived stores can drift from Postgres. `./scripts/seed.sh`
deliberately drives all medals/results/records through the HTTP API (not direct SQL) so
this fan-out actually runs.

## Database schema & lifecycle

- Schema lives in `migrations/0001_schema.sql` and is auto-loaded by the Postgres container **only on a fresh volume** (Docker's `/docker-entrypoint-initdb.d`). To apply schema changes you must drop the volume: `make down` (which uses `-v`) then `make up`.
- `make down` deletes all data in every store. There is no migration tool — editing the SQL file and recreating the volume is the workflow.
- Mongo indexes are created at app startup via `resultRepo.EnsureIndexes`.

## Web UIs (for running queries)

`make up` starts these (Docker compose `tools` profile) and opens them:

| DB        | UI              | URL                   |
|-----------|-----------------|-----------------------|
| Postgres  | Adminer         | http://localhost:8081 |
| MongoDB   | mongo-express   | http://localhost:8082 |
| Redis     | Redis Commander | http://localhost:8083 |
| Neo4j     | Neo4j Browser   | http://localhost:7474 |

Inside the UIs the DB host is the **service name** (`postgres`, `mongo`, `redis`,
`neo4j`), not `localhost`. Default creds: Postgres `app`/`app`/db `app`; Neo4j
`neo4j`/`test12345`. App listens on http://localhost:8080 (`GET /healthz`).
