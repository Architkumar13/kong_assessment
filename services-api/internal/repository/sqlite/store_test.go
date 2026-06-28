package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/Architkumar13/services-catalog-api/internal/domain"
	"github.com/Architkumar13/services-catalog-api/internal/repository/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.New(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	return store
}

func makeService(name, description string) *domain.Service {
	now := time.Now().UTC().Truncate(time.Second)
	return &domain.Service{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func makeVersion(serviceID, tag, status string) *domain.Version {
	return &domain.Version{
		ID:        uuid.New().String(),
		ServiceID: serviceID,
		Tag:       tag,
		Status:    status,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
}

// ─── Service tests ────────────────────────────────────────────────────────────

func TestCreateAndGetService(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	svc := makeService("Auth Service", "Handles auth")
	require.NoError(t, store.CreateService(ctx, svc))

	got, err := store.GetService(ctx, svc.ID)
	require.NoError(t, err)
	assert.Equal(t, svc.ID, got.ID)
	assert.Equal(t, svc.Name, got.Name)
	assert.Equal(t, svc.Description, got.Description)
}

func TestGetService_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, err := store.GetService(ctx, uuid.New().String())
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestListServices_Search(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	svc1 := makeService("Auth Service", "authentication")
	svc2 := makeService("Payment Gateway", "payments")
	require.NoError(t, store.CreateService(ctx, svc1))
	require.NoError(t, store.CreateService(ctx, svc2))

	services, total, err := store.ListServices(ctx, domain.ListFilter{Search: "auth"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, services, 1)
	assert.Equal(t, "Auth Service", services[0].Name)
}

func TestListServices_SearchDescription(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	svc1 := makeService("Notifier", "sends email notifications")
	svc2 := makeService("Logger", "records audit logs")
	require.NoError(t, store.CreateService(ctx, svc1))
	require.NoError(t, store.CreateService(ctx, svc2))

	services, total, err := store.ListServices(ctx, domain.ListFilter{Search: "email"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, services, 1)
	assert.Equal(t, "Notifier", services[0].Name)
}

func TestListServices_SortByName(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	names := []string{"Zebra Service", "Alpha Service", "Mango Service"}
	for _, n := range names {
		require.NoError(t, store.CreateService(ctx, makeService(n, "")))
	}

	services, _, err := store.ListServices(ctx, domain.ListFilter{SortBy: "name", Order: "asc"})
	require.NoError(t, err)
	require.Len(t, services, 3)
	assert.Equal(t, "Alpha Service", services[0].Name)
	assert.Equal(t, "Mango Service", services[1].Name)
	assert.Equal(t, "Zebra Service", services[2].Name)
}

func TestListServices_SortByNameDesc(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	names := []string{"Zebra Service", "Alpha Service"}
	for _, n := range names {
		require.NoError(t, store.CreateService(ctx, makeService(n, "")))
	}

	services, _, err := store.ListServices(ctx, domain.ListFilter{SortBy: "name", Order: "desc"})
	require.NoError(t, err)
	require.Len(t, services, 2)
	assert.Equal(t, "Zebra Service", services[0].Name)
}

func TestListServices_Pagination(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 15; i++ {
		require.NoError(t, store.CreateService(ctx, makeService("Service"+string(rune('A'+i)), "")))
	}

	// Page 1
	page1, total, err := store.ListServices(ctx, domain.ListFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, 15, total)
	assert.Len(t, page1, 10)

	// Page 2
	page2, total2, err := store.ListServices(ctx, domain.ListFilter{Page: 2, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, 15, total2)
	assert.Len(t, page2, 5)
}

func TestUpdateService(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	svc := makeService("Original", "original desc")
	require.NoError(t, store.CreateService(ctx, svc))

	svc.Name = "Updated"
	svc.Description = "updated desc"
	svc.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	require.NoError(t, store.UpdateService(ctx, svc))

	got, err := store.GetService(ctx, svc.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", got.Name)
	assert.Equal(t, "updated desc", got.Description)
}

func TestUpdateService_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	svc := makeService("Ghost", "")
	svc.ID = uuid.New().String() // not inserted
	err := store.UpdateService(ctx, svc)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDeleteService(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	svc := makeService("ToDelete", "")
	require.NoError(t, store.CreateService(ctx, svc))
	require.NoError(t, store.DeleteService(ctx, svc.ID))

	_, err := store.GetService(ctx, svc.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDeleteService_CascadesVersions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	svc := makeService("Parent", "")
	require.NoError(t, store.CreateService(ctx, svc))

	ver := makeVersion(svc.ID, "v1.0.0", "active")
	require.NoError(t, store.CreateVersion(ctx, ver))

	require.NoError(t, store.DeleteService(ctx, svc.ID))

	// The version should be gone due to ON DELETE CASCADE.
	_, err := store.GetVersion(ctx, ver.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDeleteService_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	err := store.DeleteService(ctx, uuid.New().String())
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// ─── Version tests ────────────────────────────────────────────────────────────

func TestCreateAndListVersions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	svc := makeService("MyService", "")
	require.NoError(t, store.CreateService(ctx, svc))

	v1 := makeVersion(svc.ID, "v1.0.0", "deprecated")
	v2 := makeVersion(svc.ID, "v2.0.0", "active")
	require.NoError(t, store.CreateVersion(ctx, v1))
	require.NoError(t, store.CreateVersion(ctx, v2))

	versions, err := store.ListVersions(ctx, svc.ID)
	require.NoError(t, err)
	assert.Len(t, versions, 2)
}

func TestListVersions_EmptyForUnknownService(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	versions, err := store.ListVersions(ctx, uuid.New().String())
	require.NoError(t, err)
	assert.Empty(t, versions)
}

func TestDeleteVersion(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	svc := makeService("Service", "")
	require.NoError(t, store.CreateService(ctx, svc))

	ver := makeVersion(svc.ID, "v1.0.0", "active")
	require.NoError(t, store.CreateVersion(ctx, ver))

	require.NoError(t, store.DeleteVersion(ctx, ver.ID))

	_, err := store.GetVersion(ctx, ver.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDeleteVersion_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	err := store.DeleteVersion(ctx, uuid.New().String())
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestVersionCount_InListServices(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	svc := makeService("Counted", "")
	require.NoError(t, store.CreateService(ctx, svc))
	require.NoError(t, store.CreateVersion(ctx, makeVersion(svc.ID, "v1.0.0", "active")))
	require.NoError(t, store.CreateVersion(ctx, makeVersion(svc.ID, "v2.0.0", "beta")))

	services, _, err := store.ListServices(ctx, domain.ListFilter{})
	require.NoError(t, err)
	require.Len(t, services, 1)
	assert.Equal(t, 2, services[0].VersionCount)
}
