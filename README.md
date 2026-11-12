# tenderwatch

A small Go service that pulls Norwegian public procurement notices from [Doffin](https://doffin.no), stores them in PostgreSQL, and answers the questions a supplier's bid team asks every week:

- **Which tenders are open in my category?**
- **Which deadlines are coming up?**
- **Who are the biggest buyers?**

## Why

Doffin publishes every Norwegian public tender, more than 160,000 notices so far. The data is public, but it isn't organised for a supplier:
- **It's built for publishing, not for answering questions.** A bid team wants "construction tenders closing in the next two weeks", not a list of notices in the order they were published.
- **The data is messy.** The same organisation appears with differently formatted numbers, and one notice can list several buyers.
- **CPV codes are hierarchical.** A category search has to match by prefix, not exactly.

tenderwatch is a deliberately small slice of that problem. It ingests notices reliably (re-running it is safe), models them so these questions become cheap SQL queries, and exposes the results as a JSON API.

## Quick start

Requirements: Go 1.27+, Docker, and a free Doffin API key (from the Public API product on the [Doffin developer portal](https://dof-notices-prod-api.developer.azure-api.net/)).

```bash
git clone https://github.com/jurzila/tenderwatch.git
cd tenderwatch
docker compose up -d          # PostgreSQL 17 on localhost:5432
```

Set the environment variables. `DOFFIN_API_KEY` is only needed for `ingest`. To try the API on data that's already ingested, `DATABASE_URL` is enough:

```
DATABASE_URL=postgres://tenderwatch:tenderwatch@localhost:5432/tenderwatch
DOFFIN_API_KEY=<your key>
```

The program reads them from the environment and doesn't load a `.env` file itself. Export them in your shell:

```bash
# bash / zsh: export every variable from .env
set -a; source .env; set +a
```

```powershell
# PowerShell
Get-Content .env | ForEach-Object { if ($_ -match '^\s*([^#=]+)=(.*)$') { Set-Item "env:$($matches[1].Trim())" $matches[2].Trim().Trim('"') } }
```

Then:

```bash
go run . ingest   # fetch the 50 newest notices and save them (safe to re-run)
go run . serve    # JSON API on http://localhost:8080
```

Both commands create the database schema at startup if it doesn't exist yet.

## API

| Endpoint | Answers | Parameters |
|---|---|---|
| `GET /notices?cpv=45` | Notices in a CPV category, matched by prefix (`45` = construction work) | `cpv`: 1–8 digits, required |
| `GET /notices/closing?days=14` | Open tenders whose deadline falls within the next N days | `days`: 1–365, default 14 |
| `GET /buyers/top?limit=10` | Buyers ranked by total estimated contract value | `limit`: 1–100, default 10 |

Example:

```bash
curl "http://localhost:8080/notices/closing?days=10"
```

```json
[{"id":"2026-117319","heading":"114-BYM-2025 - Anskaffelse av entreprenør til utbedring av skatebowl i Oslo Skatehall","deadline":"2026-11-19T12:00:00+01:00","estimatedAmount":null,"currency":null}]
```

Missing values are returned as `null`, never as `0`: "no estimated value given" is not the same as "worth 0 NOK". Invalid parameters get a `400` with a message saying what's wrong. Internal errors get a generic `500`; the details go to the server log, not the client.

## Design choices

- **The standard library, no framework.** The HTTP layer uses `net/http` and Go 1.22's `ServeMux` patterns (`GET /notices`). The only dependency is the Postgres driver, [pgx](https://github.com/jackc/pgx).
- **Ingestion is idempotent.** Every notice is written with `INSERT ... ON CONFLICT DO UPDATE`, keyed on Doffin's own notice id. Running ingestion twice leaves the database in the same state. That matters for anything run on a schedule or retried automatically.
- **One transaction per notice.** A notice's buyers and CPV codes are deleted and reinserted inside the same transaction as the notice itself. An upsert alone can't remove a CPV code that disappeared from the source. The transaction also means a crash never leaves half a notice, and readers never see a notice without its codes.
- **Natural keys from the source.** The keys are Doffin's notice id and the Norwegian organisation number, not generated ids. That's what makes the upsert possible, and it lets "buyer" mean the legal entity, the level a supplier cares about.
- **Lists are tables, not array columns.** `notice_buyers` and `notice_cpv_codes` make "value per buyer" a plain `JOIN` with `GROUP BY`. A `text_pattern_ops` index lets `cpv_code LIKE '45%'` use an index under a non-C collation.
- **Money is `numeric(15,2)`,** so sums are exact.
- **Handlers depend on a small interface (`noticeStore`) defined next to them,** not on the database. Handler tests use a fake store, with no database, and can simulate failures.
- **The schema is embedded in the binary** (`//go:embed`) and applied at startup with `CREATE ... IF NOT EXISTS`. That's simple and works the same locally and in a container. Its limit is that it can only add things, never change or drop them; see "What I'd build next".

## Data quality findings

Working with real Doffin data surfaced issues a production version has to handle:

- **Organisation numbers aren't always clean.** One buyer arrived as `940 101 808`, with spaces, instead of `940101808`. Stored as-is, the same organisation could become two buyers and split its totals. *Not fixed yet:* numbers should be normalised to digits only during ingestion.
- **One notice can list the same organisation twice,** for example two departments that share an organisation number. Because buyers are modelled at the legal-entity level, such a notice currently fails to save. It's logged and skipped, and doesn't corrupt anything. *Fix:* deduplicate buyers per notice before linking them.
- **Joint procurements inflate the buyer totals.** When several buyers share a notice, `/buyers/top` counts the full estimated value for each of them, so totals across buyers add up to more than the actual market. Whether to split the value, and how, is a product decision; for now the API documents it rather than hiding it.
- **Many notices have no estimated value, status or deadline.** These are stored and returned as `null` throughout.

## Tests

```bash
go test ./...                                 # unit and handler tests, no database needed
go test -tags integration ./...               # also runs the database tests (needs DATABASE_URL)
```

- **`search_test.go`:** the Doffin client against an `httptest` server. It covers a 401, an HTML maintenance page, a wrong JSON type, and checks that the API key header is always sent.
- **`server_test.go`:** all three handlers, with a fake store that records what it was called with.
  - **Parameters:** validation errors return 400 before the store is called. The defaults (`days=14`, `limit=10`) actually reach the store.
  - **Errors:** a store failure returns 500, with no internal details leaked.
  - **Empty results** are encoded as `[]`, not `null`.
  - **`intParam`** is tested at and one step past each boundary.
  - **The `/buyers/top` field names** are checked by decoding into a generic map. Decoding into the same struct would hide a typo in a JSON tag, because the typo would appear on both sides.
- **`db_test.go`:** saving the same notice twice gives one row, and a CPV code removed from the source is removed from the database. It runs against the real database and cleans up after itself.

Each error case was checked by temporarily breaking the code it protects and confirming the test fails.

## Coming from Java

This project was a way to learn Go coming from Java and Spring. The differences that stood out:

- **Errors are values.** `net/http` doesn't treat a 401 as an error the way `RestTemplate` does: a response came back, so `err` is `nil` and checking the status is up to you. Being explicit like that is more verbose, but you always see where each failure is handled.
- **Interfaces are satisfied implicitly and defined by the code that uses them.** The handler declares the one method it needs, and the database type fits without `implements`. That made a fake store for tests a five-line struct, with no mocking library.
- **The standard library goes a long way.** Go has web frameworks and routers (Gin, Echo, chi) and ORMs (GORM, ent), but I deliberately used none, to learn what they would abstract. `net/http`, `encoding/json` and the pgx driver covered what Spring Boot, Jackson and JPA usually do for me. There was more code to write, but nothing was hidden, and for a service this size the standard library was enough.
- **Zero values versus `null`.** A JSON `null` decoded into an `int` silently becomes `0`. Telling "missing" apart from "zero" needs a pointer type, and that decision came up again at every layer: JSON, Go and SQL.
- **`defer`** replaces `try/finally`, and sits right next to the resource it cleans up.

## How I used AI tools

I built this with Claude Code as a tutor and reviewer, not as a code generator:
- **I wrote the code myself.** Claude explained Go concepts, usually by contrasting them with Java, set each task, and reviewed what I wrote the way a senior colleague would.
- **What the reviews caught:** ignored errors, a nil pointer that would have panicked on real data, and tests that passed for the wrong reason. One example: a 401 test passed only because an empty body happened to fail JSON decoding.
- **Checking tests by breaking the code on purpose** became a habit through that review process.
- **Where it helped most:** unblocking toolchain problems (CORS in the API portal, gofmt and line endings on Windows, `go mod tidy`), so the time went into concepts instead.
## What I'd build next

- **Fix the data issues above:** normalise organisation numbers, deduplicate buyers per notice, and decide how to attribute value on joint procurements.
- **Full ingestion.** Doffin's search returns at most 1,000 hits per query, so a backfill needs to split the query (for example by publication date), and respect the 10 requests per minute limit of the `download` endpoint.
- **Parse the full eForms XML** from `download/{doffinId}` (lots, award criteria, winners), not just the search summary.
- **Supplier risk:** look up each buyer and winner in the public Brønnøysund register (Enhetsregisteret).
- **Versioned migrations** (goose or golang-migrate, the Go counterparts of the Liquibase changelogs I've used), and **sqlc** to generate typed query code.
- **Run it as a container on Cloud Run,** with ingestion as a scheduled job. With Temporal, the idempotent `saveNotice` would be a safe, retryable activity.
- **Show CPV category names** ("Construction work") next to the codes, from the EU's CPV list.
- **Structured logging** with `log/slog`, for Cloud Logging.

## Project layout

```
main.go             entry point: `ingest` and `serve` subcommands, Doffin search client
db.go               connection, schema, saveNotice (upsert), read queries
server.go           HTTP handlers, parameter validation, routes
schema.sql          tables and indexes (embedded into the binary)
docker-compose.yml  local PostgreSQL
*_test.go           tests (see above)
```
