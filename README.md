# Services Catalog API

A REST API in Go for a service catalog dashboard — lets users view, search, and manage services and their versions.

---

## Quick Start

```bash
# Start Postgres and the API together
docker compose up

# Or run locally against an existing Postgres instance
export DATABASE_URL=postgres://catalog:catalog@localhost:5432/catalog?sslmode=disable
go run .
```

The server starts on port `8080` by default and seeds sample data on first run.

### Environment Variables

| Variable       | Default                          | Description                    |
|----------------|----------------------------------|--------------------------------|
| `PORT`         | `8080`                           | HTTP listen port               |
| `JWT_SECRET`   | `dev-secret-change-in-production`| Secret for signing JWT tokens  |
| `DATABASE_URL` | *(required)*                     | Postgres connection string     |

---

## Make Commands

```bash
make build      # compile to bin/server
make run        # go run .
make test-unit  # unit tests — no database needed
make test       # full test suite (set TEST_DATABASE_URL first)
make lint       # go vet
```

---

## API Reference

### Authentication

Write operations require a JWT bearer token.

```bash
# Get a token (demo credentials: admin / secret)
curl -X POST http://localhost:8080/api/v1/auth/token \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"secret"}'

# Use the token
Authorization: Bearer <token>
```

### Endpoints

| Method | Path                                  | Auth | Description                 |
|--------|---------------------------------------|------|-----------------------------|
| GET    | /health                               |      | Health check                |
| POST   | /api/v1/auth/token                    |      | Issue JWT token             |
| GET    | /api/v1/services                      |      | List services               |
| POST   | /api/v1/services                      | ✓    | Create a service            |
| GET    | /api/v1/services/{id}                 |      | Get service + versions      |
| PUT    | /api/v1/services/{id}                 | ✓    | Update a service            |
| DELETE | /api/v1/services/{id}                 | ✓    | Delete service + versions   |
| GET    | /api/v1/services/{id}/versions        |      | List versions               |
| POST   | /api/v1/services/{id}/versions        | ✓    | Add a version               |
| DELETE | /api/v1/services/{id}/versions/{vid}  | ✓    | Delete a version            |

### List Services — query parameters

| Param       | Default      | Description                                      |
|-------------|--------------|--------------------------------------------------|
| `search`    | —            | Filter by name or description (case-insensitive) |
| `sort`      | `created_at` | Sort field: `name`, `created_at`, `updated_at`   |
| `order`     | `asc`        | `asc` or `desc`                                  |
| `page`      | `1`          | Page number (1-indexed)                          |
| `page_size` | `20`         | Items per page (max 100)                         |

```bash
curl "http://localhost:8080/api/v1/services?search=payment&sort=name&order=asc"
```

### Version status values

`active` | `deprecated` | `beta`

---

## Design Considerations

### Flat structure

The project lives in three files at the repo root — `main.go`, `store.go`, `handlers.go` — all in `package main`. There are no sub-packages or abstraction layers. Each file has a single clear responsibility:

- **`main.go`** — wires dependencies, registers routes, graceful shutdown
- **`store.go`** — data models and all database access
- **`handlers.go`** — HTTP handlers, JWT middleware, input validation

This keeps every piece of logic immediately findable without navigating a deep directory tree.

### Routing

Uses Go 1.22's enhanced `net/http.ServeMux` which supports method-scoped routes (`"GET /path/{id}"`) and `r.PathValue()` for URL parameters — no third-party router needed.

### Persistence — PostgreSQL

- **Relational fit** — services and versions have a clear parent-child relationship; a foreign key with `ON DELETE CASCADE` keeps cleanup automatic
- **ILIKE** — case-insensitive search is a single SQL clause with no application-side filtering
- **TIMESTAMPTZ** — the database owns time zones, so no string-to-time parsing is needed in Go
- **Concurrent access** — unlike SQLite, Postgres handles concurrent writes cleanly

### Authentication

JWT (HS256) guards all write operations. The demo uses hardcoded `admin`/`secret` credentials — in production this would verify against a users table or delegate to an identity provider.

---

## Testing

Tests are split into two layers:

**Unit tests** — validate pure logic, no database needed:
- `TestValidateService` — name/description constraints
- `TestValidateVersion` — tag format and status values
- `TestRequireAuth` — JWT middleware with missing/invalid tokens

**Integration tests** — full HTTP stack via `net/http/httptest` against a real Postgres instance:
- `TestServiceCRUD` — create / get / update / delete roundtrip
- `TestVersionCRUD` — version lifecycle
- `TestSearchAndPagination` — search filtering and page_size
- `TestCreateServiceValidation` — 422/400 error cases

```bash
# Unit tests only
make test-unit

# Full suite (needs a Postgres instance)
export TEST_DATABASE_URL=postgres://user:pass@localhost:5432/catalog_test?sslmode=disable
make test
```

---

## Trade-offs

- **Hardcoded credentials** — `admin`/`secret` is a demo stub; production would use bcrypt + a users table or OAuth2.
- **No migrations tool** — schema is created inline in `migrate()`. For production, `golang-migrate` or `goose` would manage versioned, reversible migrations.
- **Offset-based pagination** — simple to implement but degrades at large offsets; cursor-based (keyset) pagination scales better.
- **Single JWT secret** — HS256 is fine for one service; a multi-service setup would use RS256 with a JWKS endpoint.
- **No rate limiting** — a token-bucket middleware on write endpoints would be a straightforward addition.
