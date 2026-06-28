package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Service is a catalog entry.
type Service struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	VersionCount int       `json:"version_count,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Version is a release of a Service.
type Version struct {
	ID        string    `json:"id"`
	ServiceID string    `json:"service_id"`
	Tag       string    `json:"tag"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

var errNotFound = errors.New("not found")

type store struct {
	db *sql.DB
}

func newStore(dsn string) (*store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	s := &store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *store) close() error { return s.db.Close() }

func (s *store) migrate() error {
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

// ─── services ────────────────────────────────────────────────────────────────

// allowedSort maps URL sort param values to safe SQL column names.
var allowedSort = map[string]string{
	"name":       "name",
	"created_at": "created_at",
	"updated_at": "updated_at",
}

func (s *store) listServices(ctx context.Context, search, sortBy, order string, page, pageSize int) ([]Service, int, error) {
	col, ok := allowedSort[sortBy]
	if !ok {
		col = "created_at"
	}
	if strings.ToLower(order) != "desc" {
		order = "asc"
	}

	// Build optional WHERE clause. $1 can be reused twice in the ILIKE expression.
	var args []any
	where := ""
	if search != "" {
		where = "WHERE name ILIKE $1 OR description ILIKE $1"
		args = append(args, "%"+search+"%")
	}

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM services "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// LIMIT and OFFSET placeholders follow any search arg.
	n := len(args)
	offset := (page - 1) * pageSize
	query := fmt.Sprintf(`
SELECT id, name, description, created_at, updated_at,
       (SELECT COUNT(*) FROM versions WHERE service_id = services.id) AS version_count
FROM services %s ORDER BY %s %s LIMIT $%d OFFSET $%d`, where, col, order, n+1, n+2)

	rows, err := s.db.QueryContext(ctx, query, append(args, pageSize, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var services []Service
	for rows.Next() {
		var svc Service
		if err := rows.Scan(&svc.ID, &svc.Name, &svc.Description, &svc.CreatedAt, &svc.UpdatedAt, &svc.VersionCount); err != nil {
			return nil, 0, err
		}
		services = append(services, svc)
	}
	return services, total, rows.Err()
}

func (s *store) getService(ctx context.Context, id string) (*Service, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, description, created_at, updated_at,
       (SELECT COUNT(*) FROM versions WHERE service_id = services.id)
FROM services WHERE id = $1`, id)

	var svc Service
	err := row.Scan(&svc.ID, &svc.Name, &svc.Description, &svc.CreatedAt, &svc.UpdatedAt, &svc.VersionCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound
	}
	return &svc, err
}

func (s *store) createService(ctx context.Context, name, description string) (*Service, error) {
	now := time.Now().UTC()
	svc := &Service{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO services (id, name, description, created_at, updated_at) VALUES ($1, $2, $3, $4, $5)",
		svc.ID, svc.Name, svc.Description, svc.CreatedAt, svc.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return svc, nil
}

func (s *store) updateService(ctx context.Context, id, name, description string) (*Service, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		"UPDATE services SET name = $1, description = $2, updated_at = $3 WHERE id = $4",
		name, description, now, id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, errNotFound
	}
	return s.getService(ctx, id)
}

func (s *store) deleteService(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM services WHERE id = $1", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound
	}
	return nil
}

// ─── versions ────────────────────────────────────────────────────────────────

func (s *store) listVersions(ctx context.Context, serviceID string) ([]Version, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, service_id, tag, status, created_at FROM versions WHERE service_id = $1 ORDER BY created_at ASC",
		serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []Version
	for rows.Next() {
		var v Version
		if err := rows.Scan(&v.ID, &v.ServiceID, &v.Tag, &v.Status, &v.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

func (s *store) createVersion(ctx context.Context, serviceID, tag, status string) (*Version, error) {
	v := &Version{
		ID:        uuid.New().String(),
		ServiceID: serviceID,
		Tag:       tag,
		Status:    status,
		CreatedAt: time.Now().UTC(),
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO versions (id, service_id, tag, status, created_at) VALUES ($1, $2, $3, $4, $5)",
		v.ID, v.ServiceID, v.Tag, v.Status, v.CreatedAt)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (s *store) deleteVersion(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM versions WHERE id = $1", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound
	}
	return nil
}

// ─── seed ─────────────────────────────────────────────────────────────────────

func (s *store) seed(ctx context.Context) error {
	var count int
	s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM services").Scan(&count)
	if count > 0 {
		return nil
	}

	type entry struct {
		name, desc string
		versions   [][2]string // {tag, status}
	}
	seeds := []entry{
		{"Auth Service", "Handles authentication and authorisation for all platform services.",
			[][2]string{{"v1.0.0", "deprecated"}, {"v1.5.0", "deprecated"}, {"v2.0.0", "active"}, {"v2.1.0-beta", "beta"}}},
		{"Payment Gateway", "Processes payments via multiple providers (Stripe, PayPal, Razorpay).",
			[][2]string{{"v1.0.0", "deprecated"}, {"v1.2.0", "active"}, {"v2.0.0-beta", "beta"}}},
		{"Notification Service", "Sends email, SMS, and push notifications through a unified API.",
			[][2]string{{"v1.0.0", "deprecated"}, {"v1.1.0", "active"}}},
		{"User Management", "CRUD operations and profile management for platform users.",
			[][2]string{{"v1.0.0", "active"}, {"v1.1.0-beta", "beta"}}},
		{"API Gateway", "Edge layer for routing, rate-limiting, and request transformation.",
			[][2]string{{"v1.0.0", "deprecated"}, {"v2.0.0", "active"}, {"v2.5.0", "active"}, {"v3.0.0-beta", "beta"}}},
		{"Reporting Service", "Generates on-demand and scheduled reports in PDF, CSV, and Excel formats.",
			[][2]string{{"v1.0.0", "deprecated"}, {"v1.3.0", "active"}}},
		{"Search Service", "Full-text and faceted search backed by Elasticsearch.",
			[][2]string{{"v1.0.0", "active"}, {"v2.0.0-beta", "beta"}}},
		{"Billing Service", "Subscription lifecycle management, invoicing, and revenue recognition.",
			[][2]string{{"v1.0.0", "deprecated"}, {"v1.4.0", "active"}, {"v2.0.0-beta", "beta"}}},
	}

	now := time.Now().UTC()
	for i, e := range seeds {
		svcTime := now.Add(-time.Duration(len(seeds)-i) * 24 * time.Hour)
		id := uuid.New().String()
		_, err := s.db.ExecContext(ctx,
			"INSERT INTO services (id, name, description, created_at, updated_at) VALUES ($1, $2, $3, $4, $5)",
			id, e.name, e.desc, svcTime, svcTime)
		if err != nil {
			return err
		}
		for j, v := range e.versions {
			vTime := svcTime.Add(time.Duration(j) * 48 * time.Hour)
			_, err := s.db.ExecContext(ctx,
				"INSERT INTO versions (id, service_id, tag, status, created_at) VALUES ($1, $2, $3, $4, $5)",
				uuid.New().String(), id, v[0], v[1], vTime)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
