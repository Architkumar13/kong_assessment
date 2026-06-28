package domain

import (
	"context"
	"time"
)

// Service represents a catalog entry for a service.
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
	Tag       string    `json:"tag"`    // e.g. "v1.0.0"
	Status    string    `json:"status"` // "active" | "deprecated" | "beta"
	CreatedAt time.Time `json:"created_at"`
}

// ValidVersionStatuses is the set of allowed version status values.
var ValidVersionStatuses = map[string]struct{}{
	"active":     {},
	"deprecated": {},
	"beta":       {},
}

// ListFilter holds query parameters for listing services.
type ListFilter struct {
	Search   string
	SortBy   string // "name" | "created_at" | "updated_at"
	Order    string // "asc" | "desc"
	Page     int
	PageSize int
}

// ServiceRepository defines the persistence contract for the domain.
type ServiceRepository interface {
	ListServices(ctx context.Context, filter ListFilter) ([]Service, int, error)
	GetService(ctx context.Context, id string) (*Service, error)
	CreateService(ctx context.Context, s *Service) error
	UpdateService(ctx context.Context, s *Service) error
	DeleteService(ctx context.Context, id string) error

	ListVersions(ctx context.Context, serviceID string) ([]Version, error)
	GetVersion(ctx context.Context, id string) (*Version, error)
	CreateVersion(ctx context.Context, v *Version) error
	DeleteVersion(ctx context.Context, id string) error
}
