package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Architkumar13/services-catalog-api/internal/domain"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// Store is the SQLite-backed implementation of domain.ServiceRepository.
type Store struct {
	db *sql.DB
}

// New opens (or creates) a SQLite database at dsn, runs migrations, and optionally seeds data.
func New(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	// SQLite performs best with a single connection for writes.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("sqlite: migrate: %w", err)
	}
	return s, nil
}

// Close releases the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// migrate creates tables and indexes.
func (s *Store) migrate() error {
	_, err := s.db.Exec(`
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;

CREATE TABLE IF NOT EXISTS services (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS versions (
    id         TEXT PRIMARY KEY,
    service_id TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    tag        TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL
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
		return nil // already seeded
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

// safeSortColumns maps allowed sort keys to SQL column names.
var safeSortColumns = map[string]string{
	"name":       "s.name",
	"created_at": "s.created_at",
	"updated_at": "s.updated_at",
}

// ListServices returns a paginated, optionally filtered and sorted list of services
// together with their version counts and the total number of matching rows.
func (s *Store) ListServices(ctx context.Context, filter domain.ListFilter) ([]domain.Service, int, error) {
	// Defaults.
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
	order := "asc"
	if strings.ToLower(filter.Order) == "desc" {
		order = "desc"
	}

	args := []any{}
	whereClause := ""
	if filter.Search != "" {
		whereClause = "WHERE (s.name LIKE ? OR s.description LIKE ?)"
		like := "%" + filter.Search + "%"
		args = append(args, like, like)
	}

	// Total count.
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM services s %s`, whereClause)
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ListServices count: %w", err)
	}

	// Paginated rows with version_count via subquery.
	offset := (filter.Page - 1) * filter.PageSize
	dataQuery := fmt.Sprintf(`
SELECT
    s.id, s.name, s.description, s.created_at, s.updated_at,
    (SELECT COUNT(*) FROM versions v WHERE v.service_id = s.id) AS version_count
FROM services s
%s
ORDER BY %s %s
LIMIT ? OFFSET ?`, whereClause, sortCol, order)

	dataArgs := append(args, filter.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("ListServices query: %w", err)
	}
	defer rows.Close()

	services := make([]domain.Service, 0, filter.PageSize)
	for rows.Next() {
		var svc domain.Service
		var createdAt, updatedAt string
		if err := rows.Scan(&svc.ID, &svc.Name, &svc.Description, &createdAt, &updatedAt, &svc.VersionCount); err != nil {
			return nil, 0, fmt.Errorf("ListServices scan: %w", err)
		}
		svc.CreatedAt, _ = parseTime(createdAt)
		svc.UpdatedAt, _ = parseTime(updatedAt)
		services = append(services, svc)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("ListServices rows: %w", err)
	}
	return services, total, nil
}

// GetService retrieves a single service by ID and embeds its full version list.
func (s *Store) GetService(ctx context.Context, id string) (*domain.Service, error) {
	query := `
SELECT s.id, s.name, s.description, s.created_at, s.updated_at,
       (SELECT COUNT(*) FROM versions v WHERE v.service_id = s.id) AS version_count
FROM services s
WHERE s.id = ?`
	row := s.db.QueryRowContext(ctx, query, id)

	var svc domain.Service
	var createdAt, updatedAt string
	err := row.Scan(&svc.ID, &svc.Name, &svc.Description, &createdAt, &updatedAt, &svc.VersionCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetService: %w", err)
	}
	svc.CreatedAt, _ = parseTime(createdAt)
	svc.UpdatedAt, _ = parseTime(updatedAt)
	return &svc, nil
}

// CreateService inserts a new service row.
func (s *Store) CreateService(ctx context.Context, svc *domain.Service) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO services (id, name, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		svc.ID, svc.Name, svc.Description,
		svc.CreatedAt.UTC().Format(time.RFC3339Nano),
		svc.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("CreateService: %w", err)
	}
	return nil
}

// UpdateService updates mutable fields of an existing service.
func (s *Store) UpdateService(ctx context.Context, svc *domain.Service) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE services SET name = ?, description = ?, updated_at = ? WHERE id = ?`,
		svc.Name, svc.Description,
		svc.UpdatedAt.UTC().Format(time.RFC3339Nano),
		svc.ID,
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
	res, err := s.db.ExecContext(ctx, `DELETE FROM services WHERE id = ?`, id)
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

// ListVersions returns all versions for the given service.
func (s *Store) ListVersions(ctx context.Context, serviceID string) ([]domain.Version, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, service_id, tag, status, created_at FROM versions WHERE service_id = ? ORDER BY created_at ASC`,
		serviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("ListVersions: %w", err)
	}
	defer rows.Close()

	versions := []domain.Version{}
	for rows.Next() {
		var v domain.Version
		var createdAt string
		if err := rows.Scan(&v.ID, &v.ServiceID, &v.Tag, &v.Status, &createdAt); err != nil {
			return nil, fmt.Errorf("ListVersions scan: %w", err)
		}
		v.CreatedAt, _ = parseTime(createdAt)
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListVersions rows: %w", err)
	}
	return versions, nil
}

// GetVersion fetches a single version by its primary key.
func (s *Store) GetVersion(ctx context.Context, id string) (*domain.Version, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, service_id, tag, status, created_at FROM versions WHERE id = ?`, id)
	var v domain.Version
	var createdAt string
	err := row.Scan(&v.ID, &v.ServiceID, &v.Tag, &v.Status, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetVersion: %w", err)
	}
	v.CreatedAt, _ = parseTime(createdAt)
	return &v, nil
}

// CreateVersion inserts a new version row.
func (s *Store) CreateVersion(ctx context.Context, v *domain.Version) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO versions (id, service_id, tag, status, created_at) VALUES (?, ?, ?, ?, ?)`,
		v.ID, v.ServiceID, v.Tag, v.Status,
		v.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("CreateVersion: %w", err)
	}
	return nil
}

// DeleteVersion removes a single version row.
func (s *Store) DeleteVersion(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM versions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("DeleteVersion: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// parseTime attempts to parse the various time string formats SQLite may return.
func parseTime(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time: %q", s)
}
