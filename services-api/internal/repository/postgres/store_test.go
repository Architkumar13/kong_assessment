package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Architkumar13/services-catalog-api/internal/domain"
	"github.com/Architkumar13/services-catalog-api/internal/repository/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestStore connects to a real Postgres instance.
// Tests are skipped when TEST_DATABASE_URL is not set so CI works without Postgres.
func newTestStore(t *testing.T) *postgres.Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping Postgres integration tests")
	}

	store, err := postgres.New(dsn)
	require.NoError(t, err)

	t.Cleanup(func() {
		// Clean up test data so tests are isolated.
		db := store.DB()
		_, _ = db.Exec(`DELETE FROM versions`)
		_, _ = db.Exec(`DELETE FROM services`)
		store.Close()
	})
	return store
}

func makeService(name, description string) *domain.Service {
	now := time.Now().UTC().Truncate(time.Millisecond)
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
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
}

func TestPG_CreateAndGetService(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	svc := makeService("Auth Service", "Handles auth")
	require.NoError(t, store.CreateService(ctx, svc))

	got, err := store.GetService(ctx, svc.ID)
	require.NoError(t, err)
	assert.Equal(t, svc.ID, got.ID)
	assert.Equal(t, svc.Name, got.Name)
}

func TestPG_GetService_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, err := store.GetService(ctx, uuid.New().String())
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestPG_ListServices_ILIKESearch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, store.CreateService(ctx, makeService("Auth Service", "authentication")))
	require.NoError(t, store.CreateService(ctx, makeService("Payment Gateway", "payments")))

	// ILIKE: uppercase search should still match.
	services, total, err := store.ListServices(ctx, domain.ListFilter{Search: "AUTH"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, services, 1)
	assert.Equal(t, "Auth Service", services[0].Name)
}

func TestPG_ListServices_Pagination(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 15; i++ {
		require.NoError(t, store.CreateService(ctx, makeService("Service"+string(rune('A'+i)), "")))
	}

	page1, total, err := store.ListServices(ctx, domain.ListFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, 15, total)
	assert.Len(t, page1, 10)

	page2, _, err := store.ListServices(ctx, domain.ListFilter{Page: 2, PageSize: 10})
	require.NoError(t, err)
	assert.Len(t, page2, 5)
}

func TestPG_UpdateService(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	svc := makeService("Original", "original")
	require.NoError(t, store.CreateService(ctx, svc))

	svc.Name = "Updated"
	svc.UpdatedAt = time.Now().UTC()
	require.NoError(t, store.UpdateService(ctx, svc))

	got, err := store.GetService(ctx, svc.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", got.Name)
}

func TestPG_DeleteService_CascadesVersions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	svc := makeService("Parent", "")
	require.NoError(t, store.CreateService(ctx, svc))

	ver := makeVersion(svc.ID, "v1.0.0", "active")
	require.NoError(t, store.CreateVersion(ctx, ver))

	require.NoError(t, store.DeleteService(ctx, svc.ID))

	_, err := store.GetVersion(ctx, ver.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestPG_VersionCount(t *testing.T) {
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
