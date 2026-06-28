package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Architkumar13/services-catalog-api/internal/domain"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Store is the PostgreSQL-backed implementation of domain.ServiceRepository.
type Store struct {
	db *sql.DB
}

// New opens a connection to PostgreSQL at dsn, runs migrations, and returns a Store.
// dsn should be a postgres connection URL, e.g.:
//
//	postgres://user:pass@localhost:5432/catalog?sslmode=disable
func New(dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: open: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("postgres: migrate: %w", err)
	}
	return s, nil
}

// Close releases the underlying database connection pool.
func (s *Store) Close() error {
	return s.db.Close()
}

// Ping verifies the connection is alive.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// DB exposes the underlying *sql.DB for testing and admin use.
func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS services (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS versions (
    id         TEXT PRIMARY KEY,
    service_id TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    tag        TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_versions_service_id ON versions(service_id);
`)
	return err
}

// Seed inserts demo services and versions when the services table is empty.
func (s *Store) Seed(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM services`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	type seedService struct {
		name        string
		description string
		versions    []struct{ tag, status string }
	}

	seeds := []seedService{
		{
			name:        "Auth Service",
			description: "Handles authentication and authorisation for all platform services.",
			versions: []struct{ tag, status string }{
				{"v1.0.0", "deprecated"},
				{"v1.5.0", "deprecated"},
				{"v2.0.0", "active"},
				{"v2.1.0-beta", "beta"},
			},
		},
		{
			name:        "Payment Gateway",
			description: "Processes payments via multiple providers (Stripe, PayPal, Razorpay).",
			versions: []struct{ tag, status string }{
				{"v1.0.0", "deprecated"},
				{"v1.2.0", "active"},
				{"v2.0.0-beta", "beta"},
			},
		},
		{
			name:        "Notification Service",
			description: "Sends email, SMS, and push notifications through a unified API.",
			versions: []struct{ tag, status string }{
				{"v1.0.0", "deprecated"},
				{"v1.1.0", "active"},
			},
		},
		{
			name:        "User Management",
			description: "CRUD operations and profile management for platform users.",
			versions: []struct{ tag, status string }{
				{"v1.0.0", "active"},
				{"v1.1.0-beta", "beta"},
			},
		},
		{
			name:        "API Gateway",
			description: "Edge layer for routing, rate-limiting, and request transformation.",
			versions: []struct{ tag, status string }{
				{"v1.0.0", "deprecated"},
				{"v2.0.0", "active"},
				{"v2.5.0", "active"},
				{"v3.0.0-beta", "beta"},
			},
		},
		{
			name:        "Reporting Service",
			description: "Generates on-demand and scheduled reports in PDF, CSV, and Excel formats.",
			versions: []struct{ tag, status string }{
				{"v1.0.0", "deprecated"},
				{"v1.3.0", "active"},
			},
		},
		{
			name:        "Search Service",
			description: "Full-text and faceted search backed by Elasticsearch.",
			versions: []struct{ tag, status string }{
				{"v1.0.0", "active"},
				{"v2.0.0-beta", "beta"},
			},
		},
		{
			name:        "Billing Service",
			description: "Subscription lifecycle management, invoicing, and revenue recognition.",
			versions: []struct{ tag, status string }{
				{"v1.0.0", "deprecated"},
				{"v1.4.0", "active"},
				{"v2.0.0-beta", "beta"},
			},
		},
	}

	now := time.Now().UTC()
	for i, seed := range seeds {
		svc := &domain.Service{
			ID:          uuid.New().String(),
			Name:        seed.name,
			Description: seed.description,
			CreatedAt:   now.Add(-time.Duration(len(seeds)-i) * 24 * time.Hour),
			UpdatedAt:   now.Add(-time.Duration(len(seeds)-i) * 24 * time.Hour),
		}
		if err := s.CreateService(ctx, svc); err != nil {
			return err
		}
		for j, v := range seed.versions {
			ver := &domain.Version{
				ID:        uuid.New().String(),
				ServiceID: svc.ID,
				Tag:       v.tag,
				Status:    v.status,
				CreatedAt: svc.CreatedAt.Add(time.Duration(j) * 48 * time.Hour),
			}
			if err := s.CreateVersion(ctx, ver); err != nil {
				return err
			}
		}
	}
	return nil
}

// ─── Service methods ──────────────────────────────────────────────────────────

var safeSortColumns = map[string]string{
	"name":       "s.name",
	"created_at": "s.created_at",
	"updated_at": "s.updated_at",
}

// ListServices returns a paginated, optionally filtered and sorted list of services.
// Search uses ILIKE for case-insensitive matching. Placeholders use Postgres $N syntax.
func (s *Store) ListServices(ctx context.Context, filter domain.ListFilter) ([]domain.Service, int, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}

	sortCol, ok := safeSortColumns[filter.SortBy]
	if !ok {
		sortCol = "s.created_at"
	}
	order := "ASC"
	if strings.ToLower(filter.Order) == "desc" {
		order = "DESC"
	}

	args := []any{}
	whereClause := ""
	argIdx := 1

	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		whereClause = fmt.Sprintf("WHERE (s.name ILIKE $%d OR s.description ILIKE $%d)", argIdx, argIdx+1)
		args = append(args, like, like)
		argIdx += 2
	}

	var total int
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM services s %s`, whereClause)
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ListServices count: %w", err)
	}

	offset := (filter.Page - 1) * filter.PageSize
	dataQuery := fmt.Sprintf(`
SELECT
    s.id, s.name, s.description, s.created_at, s.updated_at,
    (SELECT COUNT(*) FROM versions v WHERE v.service_id = s.id) AS version_count
FROM services s
%s
ORDER BY %s %s
LIMIT $%d OFFSET $%d`, whereClause, sortCol, order, argIdx, argIdx+1)

	dataArgs := append(args, filter.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("ListServices query: %w", err)
	}
	defer rows.Close()

	services := make([]domain.Service, 0, filter.PageSize)
	for rows.Next() {
		var svc domain.Service
		if err := rows.Scan(&svc.ID, &svc.Name, &svc.Description, &svc.CreatedAt, &svc.UpdatedAt, &svc.VersionCount); err != nil {
			return nil, 0, fmt.Errorf("ListServices scan: %w", err)
		}
		svc.CreatedAt = svc.CreatedAt.UTC()
		svc.UpdatedAt = svc.UpdatedAt.UTC()
		services = append(services, svc)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("ListServices rows: %w", err)
	}
	return services, total, nil
}

// GetService retrieves a single service by ID with its version count.
func (s *Store) GetService(ctx context.Context, id string) (*domain.Service, error) {
	query := `
SELECT s.id, s.name, s.description, s.created_at, s.updated_at,
       (SELECT COUNT(*) FROM versions v WHERE v.service_id = s.id) AS version_count
FROM services s
WHERE s.id = $1`

	var svc domain.Service
	err := s.db.QueryRowContext(ctx, query, id).
		Scan(&svc.ID, &svc.Name, &svc.Description, &svc.CreatedAt, &svc.UpdatedAt, &svc.VersionCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetService: %w", err)
	}
	svc.CreatedAt = svc.CreatedAt.UTC()
	svc.UpdatedAt = svc.UpdatedAt.UTC()
	return &svc, nil
}

// CreateService inserts a new service row.
func (s *Store) CreateService(ctx context.Context, svc *domain.Service) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO services (id, name, description, created_at, updated_at) VALUES ($1, $2, $3, $4, $5)`,
		svc.ID, svc.Name, svc.Description, svc.CreatedAt.UTC(), svc.UpdatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("CreateService: %w", err)
	}
	return nil
}

// UpdateService updates mutable fields of an existing service.
func (s *Store) UpdateService(ctx context.Context, svc *domain.Service) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE services SET name = $1, description = $2, updated_at = $3 WHERE id = $4`,
		svc.Name, svc.Description, svc.UpdatedAt.UTC(), svc.ID,
	)
	if err != nil {
		return fmt.Errorf("UpdateService: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// DeleteService removes a service and cascades to its versions.
func (s *Store) DeleteService(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM services WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("DeleteService: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ─── Version methods ──────────────────────────────────────────────────────────

// ListVersions returns all versions for the given service ordered by creation time.
func (s *Store) ListVersions(ctx context.Context, serviceID string) ([]domain.Version, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, service_id, tag, status, created_at FROM versions WHERE service_id = $1 ORDER BY created_at ASC`,
		serviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("ListVersions: %w", err)
	}
	defer rows.Close()

	versions := []domain.Version{}
	for rows.Next() {
		var v domain.Version
		if err := rows.Scan(&v.ID, &v.ServiceID, &v.Tag, &v.Status, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("ListVersions scan: %w", err)
		}
		v.CreatedAt = v.CreatedAt.UTC()
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListVersions rows: %w", err)
	}
	return versions, nil
}

// GetVersion fetches a single version by its primary key.
func (s *Store) GetVersion(ctx context.Context, id string) (*domain.Version, error) {
	var v domain.Version
	err := s.db.QueryRowContext(ctx,
		`SELECT id, service_id, tag, status, created_at FROM versions WHERE id = $1`, id,
	).Scan(&v.ID, &v.ServiceID, &v.Tag, &v.Status, &v.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetVersion: %w", err)
	}
	v.CreatedAt = v.CreatedAt.UTC()
	return &v, nil
}

// CreateVersion inserts a new version row.
func (s *Store) CreateVersion(ctx context.Context, v *domain.Version) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO versions (id, service_id, tag, status, created_at) VALUES ($1, $2, $3, $4, $5)`,
		v.ID, v.ServiceID, v.Tag, v.Status, v.CreatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("CreateVersion: %w", err)
	}
	return nil
}

// DeleteVersion removes a single version row.
func (s *Store) DeleteVersion(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM versions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("DeleteVersion: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
