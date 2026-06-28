package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Architkumar13/services-catalog-api/internal/domain"
	"github.com/Architkumar13/services-catalog-api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Mock repository ──────────────────────────────────────────────────────────

// mockRepo is a hand-written mock that satisfies domain.ServiceRepository.
type mockRepo struct {
	services map[string]*domain.Service
	versions map[string]*domain.Version

	// Capture arguments passed to ListServices for assertion.
	lastFilter domain.ListFilter
	// Force errors on specific calls.
	listErr   error
	createErr error
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		services: make(map[string]*domain.Service),
		versions: make(map[string]*domain.Version),
	}
}

func (m *mockRepo) ListServices(ctx context.Context, filter domain.ListFilter) ([]domain.Service, int, error) {
	m.lastFilter = filter
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	out := make([]domain.Service, 0, len(m.services))
	for _, s := range m.services {
		out = append(out, *s)
	}
	return out, len(out), nil
}

func (m *mockRepo) GetService(ctx context.Context, id string) (*domain.Service, error) {
	s, ok := m.services[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *s
	return &cp, nil
}

func (m *mockRepo) CreateService(ctx context.Context, s *domain.Service) error {
	if m.createErr != nil {
		return m.createErr
	}
	cp := *s
	m.services[s.ID] = &cp
	return nil
}

func (m *mockRepo) UpdateService(ctx context.Context, s *domain.Service) error {
	if _, ok := m.services[s.ID]; !ok {
		return domain.ErrNotFound
	}
	cp := *s
	m.services[s.ID] = &cp
	return nil
}

func (m *mockRepo) DeleteService(ctx context.Context, id string) error {
	if _, ok := m.services[id]; !ok {
		return domain.ErrNotFound
	}
	delete(m.services, id)
	return nil
}

func (m *mockRepo) ListVersions(ctx context.Context, serviceID string) ([]domain.Version, error) {
	out := []domain.Version{}
	for _, v := range m.versions {
		if v.ServiceID == serviceID {
			out = append(out, *v)
		}
	}
	return out, nil
}

func (m *mockRepo) GetVersion(ctx context.Context, id string) (*domain.Version, error) {
	v, ok := m.versions[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *v
	return &cp, nil
}

func (m *mockRepo) CreateVersion(ctx context.Context, v *domain.Version) error {
	cp := *v
	m.versions[v.ID] = &cp
	return nil
}

func (m *mockRepo) DeleteVersion(ctx context.Context, id string) error {
	if _, ok := m.versions[id]; !ok {
		return domain.ErrNotFound
	}
	delete(m.versions, id)
	return nil
}

// ─── Service layer tests ──────────────────────────────────────────────────────

func TestCreateService_ValidationErrors(t *testing.T) {
	cs := service.New(newMockRepo())
	ctx := context.Background()

	tests := []struct {
		name        string
		svcName     string
		description string
		wantErrIs   error
	}{
		{"empty name", "", "desc", domain.ErrValidation},
		{"name too long", string(make([]byte, 101)), "desc", domain.ErrValidation},
		{"description too long", "Valid Name", string(make([]byte, 501)), domain.ErrValidation},
		{"whitespace name", "   ", "desc", domain.ErrValidation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cs.CreateService(ctx, tt.svcName, tt.description)
			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.wantErrIs), "expected %v, got %v", tt.wantErrIs, err)
		})
	}
}

func TestCreateService_Success(t *testing.T) {
	repo := newMockRepo()
	cs := service.New(repo)
	ctx := context.Background()

	svc, err := cs.CreateService(ctx, "Auth Service", "handles auth")
	require.NoError(t, err)
	assert.NotEmpty(t, svc.ID)
	assert.Equal(t, "Auth Service", svc.Name)
	assert.False(t, svc.CreatedAt.IsZero())
}

func TestListServices_PassesCorrectFilter(t *testing.T) {
	repo := newMockRepo()
	cs := service.New(repo)
	ctx := context.Background()

	filter := domain.ListFilter{
		Search:   "payment",
		SortBy:   "name",
		Order:    "desc",
		Page:     2,
		PageSize: 10,
	}
	_, _, err := cs.ListServices(ctx, filter)
	require.NoError(t, err)

	assert.Equal(t, "payment", repo.lastFilter.Search)
	assert.Equal(t, "name", repo.lastFilter.SortBy)
	assert.Equal(t, "desc", repo.lastFilter.Order)
	assert.Equal(t, 2, repo.lastFilter.Page)
	assert.Equal(t, 10, repo.lastFilter.PageSize)
}

func TestListServices_DefaultsApplied(t *testing.T) {
	repo := newMockRepo()
	cs := service.New(repo)
	ctx := context.Background()

	_, _, err := cs.ListServices(ctx, domain.ListFilter{})
	require.NoError(t, err)

	assert.Equal(t, 1, repo.lastFilter.Page)
	assert.Equal(t, 20, repo.lastFilter.PageSize)
	assert.Equal(t, "created_at", repo.lastFilter.SortBy)
	assert.Equal(t, "asc", repo.lastFilter.Order)
}

func TestListServices_InvalidSortByDefaultsToCreatedAt(t *testing.T) {
	repo := newMockRepo()
	cs := service.New(repo)
	ctx := context.Background()

	_, _, err := cs.ListServices(ctx, domain.ListFilter{SortBy: "'; DROP TABLE services; --"})
	require.NoError(t, err)
	assert.Equal(t, "created_at", repo.lastFilter.SortBy)
}

func TestGetService_ReturnsErrNotFound(t *testing.T) {
	cs := service.New(newMockRepo())
	ctx := context.Background()

	_, err := cs.GetService(ctx, "does-not-exist")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestGetService_EmptyID(t *testing.T) {
	cs := service.New(newMockRepo())
	ctx := context.Background()
	_, err := cs.GetService(ctx, "")
	assert.ErrorIs(t, err, domain.ErrValidation)
}

func TestUpdateService_NotFound(t *testing.T) {
	cs := service.New(newMockRepo())
	ctx := context.Background()
	_, err := cs.UpdateService(ctx, "ghost-id", "Name", "desc")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDeleteService_NotFound(t *testing.T) {
	cs := service.New(newMockRepo())
	ctx := context.Background()
	err := cs.DeleteService(ctx, "ghost-id")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestCreateVersion_ValidationErrors(t *testing.T) {
	repo := newMockRepo()
	cs := service.New(repo)
	ctx := context.Background()

	// Seed a service.
	svc, err := cs.CreateService(ctx, "MyService", "")
	require.NoError(t, err)

	tests := []struct {
		name   string
		tag    string
		status string
	}{
		{"empty tag", "", "active"},
		{"bad tag format", "1.0.0", "active"},   // missing v prefix
		{"bad tag format 2", "v1", "active"},     // no minor
		{"invalid status", "v1.0.0", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cs.CreateVersion(ctx, svc.ID, tt.tag, tt.status)
			assert.ErrorIs(t, err, domain.ErrValidation)
		})
	}
}

func TestCreateVersion_Success(t *testing.T) {
	repo := newMockRepo()
	cs := service.New(repo)
	ctx := context.Background()

	svc, err := cs.CreateService(ctx, "MyService", "")
	require.NoError(t, err)

	ver, err := cs.CreateVersion(ctx, svc.ID, "v1.0.0", "active")
	require.NoError(t, err)
	assert.NotEmpty(t, ver.ID)
	assert.Equal(t, "v1.0.0", ver.Tag)
	assert.Equal(t, "active", ver.Status)
}

func TestCreateVersion_DefaultsStatusToActive(t *testing.T) {
	repo := newMockRepo()
	cs := service.New(repo)
	ctx := context.Background()

	svc, err := cs.CreateService(ctx, "MyService", "")
	require.NoError(t, err)

	ver, err := cs.CreateVersion(ctx, svc.ID, "v1.0.0", "")
	require.NoError(t, err)
	assert.Equal(t, "active", ver.Status)
}

func TestListVersions_ServiceNotFound(t *testing.T) {
	cs := service.New(newMockRepo())
	ctx := context.Background()
	_, err := cs.ListVersions(ctx, "ghost-service-id")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}
