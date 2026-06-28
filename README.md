# Services Catalog API

A production-ready RESTful API in Go for a Service Catalog — the backend for a dashboard showing services, their descriptions, and available versions.

---

## Quick Start

### Prerequisites
- Go 1.22+
- No external database required (SQLite is embedded)

### Run

```bash
# From the services-api/ directory
make run
# or
go run ./cmd/server
```

The server starts on port `8080` by default. The database is created in-memory and seeded with sample data on startup.

### Environment Variables

| Variable     | Default                          | Description                        |
|--------------|----------------------------------|------------------------------------|
| `PORT`       | `8080`                           | HTTP listen port                   |
| `JWT_SECRET` | `dev-secret-change-in-production`| Secret key for signing JWT tokens  |
| `DB_PATH`    | `:memory:`                       | SQLite path (use a file path to persist across restarts) |

> **Note:** The default `DB_PATH` of `:memory:` seeds sample data on every startup. Set `DB_PATH=./catalog.db` to persist data.

---

## Make Commands

```bash
make build    # Compile to bin/server
make run      # Run the server
make test     # Run all tests with -race flag
make lint     # Run go vet
```

---

## API Reference

### Authentication

The API uses JWT bearer tokens. To get a token:

```bash
curl -X POST http://localhost:8080/api/v1/auth/token \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"secret"}'
```

> **Demo credentials:** `admin` / `secret` — for production, replace with a real auth service.

Use the returned token in subsequent requests:
```
Authorization: Bearer <token>
```

Read operations are public. Write operations (POST, PUT, DELETE) require a valid JWT.

---

### Endpoints

#### Health

```bash
GET /health
```

#### List Services

```bash
GET /api/v1/services
```

Query parameters:

| Param       | Type   | Default      | Description                                      |
|-------------|--------|--------------|--------------------------------------------------|
| `search`    | string | —            | Filter by name or description (case-insensitive) |
| `sort`      | string | `created_at` | Sort field: `name`, `created_at`, `updated_at`   |
| `order`     | string | `asc`        | Sort direction: `asc` or `desc`                  |
| `page`      | int    | `1`          | Page number (1-indexed)                          |
| `page_size` | int    | `20`         | Items per page (max 100)                         |

```bash
# Search and paginate
curl "http://localhost:8080/api/v1/services?search=payment&sort=name&order=asc&page=1&page_size=5"
```

Response:
```json
{
  "data": [
    {
      "id": "uuid",
      "name": "Payment Gateway",
      "description": "...",
      "version_count": 3,
      "created_at": "...",
      "updated_at": "..."
    }
  ],
  "meta": {
    "page": 1,
    "page_size": 5,
    "total": 1,
    "total_pages": 1
  }
}
```

#### Get Service

```bash
GET /api/v1/services/{id}
```

Returns the service with its full `versions` array embedded.

```bash
curl http://localhost:8080/api/v1/services/<id>
```

#### Create Service (protected)

```bash
POST /api/v1/services
Authorization: Bearer <token>
Content-Type: application/json

{"name": "My Service", "description": "Does something useful"}
```

#### Update Service (protected)

```bash
PUT /api/v1/services/{id}
Authorization: Bearer <token>
Content-Type: application/json

{"name": "Renamed Service", "description": "Updated description"}
```

#### Delete Service (protected)

```bash
DELETE /api/v1/services/{id}
Authorization: Bearer <token>
```

Deletes the service and all its versions (cascade).

#### List Versions

```bash
GET /api/v1/services/{id}/versions
```

#### Create Version (protected)

```bash
POST /api/v1/services/{id}/versions
Authorization: Bearer <token>
Content-Type: application/json

{"tag": "v1.2.0", "status": "active"}
```

`status` must be one of: `active`, `deprecated`, `beta`.

#### Delete Version (protected)

```bash
DELETE /api/v1/services/{id}/versions/{vid}
Authorization: Bearer <token>
```

---

## Architecture

### Layer Structure

```
cmd/server/          # Entry point — wires dependencies, starts HTTP server
internal/
  domain/            # Models (Service, Version), repository interface, domain errors
  repository/sqlite/ # SQLite implementation of the repository interface
  service/           # Business logic — validation, orchestration
  handler/           # HTTP handlers, request/response types, router setup
    middleware/      # JWT auth, request logger
pkg/apierror/        # Shared JSON error response helper
```

The three internal layers follow a strict dependency direction:

```
handler → service → repository
                  ↑
              domain (shared models + interface)
```

No layer imports anything above it. The `domain` package is the only shared package — it defines the `ServiceRepository` interface, which lets the service layer remain completely independent of SQLite.

### Persistence Choice: SQLite

SQLite was chosen because:
- A service catalog is read-heavy with modest write volume — SQLite handles this well
- Zero operational overhead: no separate process, no connection pooling config, no migrations server
- `modernc.org/sqlite` is pure Go (no CGO), so the binary cross-compiles cleanly
- The repository interface makes swapping to PostgreSQL a one-file change

The schema uses a simple two-table design (`services`, `versions`) with a foreign key and cascade delete. `version_count` per service is computed with a subquery in the list query to avoid N+1 fetches.

### Repository Pattern

`domain.ServiceRepository` is an interface. The SQLite store implements it. The service layer depends only on the interface — tests use a hand-written mock, and a future PostgreSQL implementation would require zero changes to the service or handler layers.

---

## Testing

```bash
make test
```

Tests are organized by layer:

| Package | Type | What it tests |
|---|---|---|
| `repository/sqlite` | Unit (in-memory SQLite) | All DB queries: CRUD, search, sort, pagination, cascade delete |
| `service` | Unit (mock repo) | Validation logic, default values, error propagation |
| `handler` | Integration (httptest) | Full HTTP stack: routing, auth middleware, request parsing, response shapes |

The integration tests spin up a real chi router with a real in-memory SQLite store — no mocking at the HTTP layer. This catches wiring bugs that pure unit tests miss.

---

## Trade-offs & Shortcuts

- **Hardcoded credentials:** The `admin`/`secret` user is a demo stub. In production this would call a real identity provider or user service.
- **No persistent DB by default:** `:memory:` keeps the demo zero-config. Set `DB_PATH` to a file path for persistence.
- **No migrations tool:** Schema is created inline in `store.go`'s `migrate()` function. For production, `golang-migrate` or `goose` would manage incremental migrations.
- **Single JWT secret:** Symmetric signing (HS256) is fine for a single-service setup. A multi-service system would use asymmetric keys (RS256) with a JWKS endpoint.
- **No rate limiting or tracing:** Kept out of scope to stay within the time budget.

---

## Areas for Improvement

1. **Observability:** Add structured logging (already using `log/slog`), distributed tracing (OpenTelemetry), and Prometheus metrics.
2. **Real auth:** Replace the demo credential check with an OAuth2/OIDC integration or a proper users table with bcrypt-hashed passwords.
3. **Migrations:** Use `golang-migrate` or `goose` for versioned, reversible schema migrations.
4. **OpenAPI spec:** Generate from code annotations (e.g. `swaggo/swag`) so the API is self-documenting.
5. **PostgreSQL support:** The repository interface is already in place — add a `repository/postgres` package and wire it via config.
6. **Rate limiting:** Add `chi/middleware`'s throttle or a token-bucket middleware on write endpoints.
7. **Pagination cursor:** Offset-based pagination degrades at large offsets; cursor-based (keyset) pagination would scale better.
