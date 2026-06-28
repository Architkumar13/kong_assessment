package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Architkumar13/services-catalog-api/internal/domain"
	"github.com/google/uuid"
)

// tagRegex allows semantic version tags like v1.0.0, v1.2.3-beta, v2.0.0-rc.1, etc.
var tagRegex = regexp.MustCompile(`^v\d+\.\d+(\.\d+)?(-[a-zA-Z0-9.]+)?$`)

// CatalogService implements business logic for the services catalog.
type CatalogService struct {
	repo domain.ServiceRepository
}

// New constructs a CatalogService backed by the given repository.
func New(repo domain.ServiceRepository) *CatalogService {
	return &CatalogService{repo: repo}
}

// ─── Service operations ───────────────────────────────────────────────────────

// ListServices validates the filter, applies defaults, and delegates to the repository.
func (cs *CatalogService) ListServices(ctx context.Context, filter domain.ListFilter) ([]domain.Service, int, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}

	// Sanitise sort column to known values.
	allowed := map[string]bool{"name": true, "created_at": true, "updated_at": true}
	if !allowed[filter.SortBy] {
		filter.SortBy = "created_at"
	}

	lo := strings.ToLower(filter.Order)
	if lo != "asc" && lo != "desc" {
		filter.Order = "asc"
	} else {
		filter.Order = lo
	}

	return cs.repo.ListServices(ctx, filter)
}

// GetService retrieves a single service by ID.
func (cs *CatalogService) GetService(ctx context.Context, id string) (*domain.Service, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: id is required", domain.ErrValidation)
	}
	return cs.repo.GetService(ctx, id)
}

// CreateService validates input, assigns a UUID, and persists the service.
func (cs *CatalogService) CreateService(ctx context.Context, name, description string) (*domain.Service, error) {
	if err := validateServiceFields(name, description); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	svc := &domain.Service{
		ID:          uuid.New().String(),
		Name:        strings.TrimSpace(name),
		Description: strings.TrimSpace(description),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := cs.repo.CreateService(ctx, svc); err != nil {
		return nil, err
	}
	return svc, nil
}

// UpdateService validates input and applies changes to the existing service.
func (cs *CatalogService) UpdateService(ctx context.Context, id, name, description string) (*domain.Service, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: id is required", domain.ErrValidation)
	}
	if err := validateServiceFields(name, description); err != nil {
		return nil, err
	}
	existing, err := cs.repo.GetService(ctx, id)
	if err != nil {
		return nil, err
	}
	existing.Name = strings.TrimSpace(name)
	existing.Description = strings.TrimSpace(description)
	existing.UpdatedAt = time.Now().UTC()
	if err := cs.repo.UpdateService(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// DeleteService removes a service and all its versions.
func (cs *CatalogService) DeleteService(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("%w: id is required", domain.ErrValidation)
	}
	return cs.repo.DeleteService(ctx, id)
}

// ─── Version operations ───────────────────────────────────────────────────────

// ListVersions returns all versions for a given service.
func (cs *CatalogService) ListVersions(ctx context.Context, serviceID string) ([]domain.Version, error) {
	if serviceID == "" {
		return nil, fmt.Errorf("%w: serviceID is required", domain.ErrValidation)
	}
	// Ensure parent service exists.
	if _, err := cs.repo.GetService(ctx, serviceID); err != nil {
		return nil, err
	}
	return cs.repo.ListVersions(ctx, serviceID)
}

// CreateVersion validates input and creates a new version for a service.
func (cs *CatalogService) CreateVersion(ctx context.Context, serviceID, tag, status string) (*domain.Version, error) {
	if serviceID == "" {
		return nil, fmt.Errorf("%w: serviceID is required", domain.ErrValidation)
	}
	if tag == "" {
		return nil, fmt.Errorf("%w: tag is required", domain.ErrValidation)
	}
	if !tagRegex.MatchString(tag) {
		return nil, fmt.Errorf("%w: tag must match vMAJOR.MINOR[.PATCH][-pre] format (e.g. v1.0.0)", domain.ErrValidation)
	}
	if status == "" {
		status = "active"
	}
	if _, ok := domain.ValidVersionStatuses[status]; !ok {
		return nil, fmt.Errorf("%w: status must be one of: active, deprecated, beta", domain.ErrValidation)
	}
	// Ensure parent service exists.
	if _, err := cs.repo.GetService(ctx, serviceID); err != nil {
		return nil, err
	}
	ver := &domain.Version{
		ID:        uuid.New().String(),
		ServiceID: serviceID,
		Tag:       tag,
		Status:    status,
		CreatedAt: time.Now().UTC(),
	}
	if err := cs.repo.CreateVersion(ctx, ver); err != nil {
		return nil, err
	}
	return ver, nil
}

// DeleteVersion removes a single version.
func (cs *CatalogService) DeleteVersion(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("%w: id is required", domain.ErrValidation)
	}
	return cs.repo.DeleteVersion(ctx, id)
}

// ─── Validation helpers ───────────────────────────────────────────────────────

func validateServiceFields(name, description string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: name is required", domain.ErrValidation)
	}
	if len(name) > 100 {
		return fmt.Errorf("%w: name must be 100 characters or fewer", domain.ErrValidation)
	}
	if len(strings.TrimSpace(description)) > 500 {
		return fmt.Errorf("%w: description must be 500 characters or fewer", domain.ErrValidation)
	}
	return nil
}
