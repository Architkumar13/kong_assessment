package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Architkumar13/services-catalog-api/internal/handler"
	"github.com/Architkumar13/services-catalog-api/internal/repository/sqlite"
	"github.com/Architkumar13/services-catalog-api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testJWTSecret = "test-secret-key"

// newTestRouter spins up a real router backed by an in-memory SQLite store.
// It seeds the store with sample data so tests can rely on existing records.
func newTestRouter(t *testing.T) (http.Handler, *sqlite.Store) {
	t.Helper()

	store, err := sqlite.New(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	require.NoError(t, store.Seed(ctx))

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	catalog := service.New(store)
	h := handler.New(catalog, testJWTSecret, logger)
	router := handler.NewRouter(h)

	return router, store
}

// getAuthToken authenticates with the demo credentials and returns the JWT string.
func getAuthToken(t *testing.T, router http.Handler) string {
	t.Helper()
	body := `{"username":"admin","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "expected 200 from auth/token, got: %s", rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	token, ok := resp["token"].(string)
	require.True(t, ok, "token field missing from auth response")
	return token
}

// ─── Health ───────────────────────────────────────────────────────────────────

func TestHealth(t *testing.T) {
	router, _ := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "ok", resp["status"])
}

// ─── Auth ─────────────────────────────────────────────────────────────────────

func TestIssueToken_ValidCredentials(t *testing.T) {
	router, _ := newTestRouter(t)
	token := getAuthToken(t, router)
	assert.NotEmpty(t, token)
}

func TestIssueToken_InvalidCredentials(t *testing.T) {
	router, _ := newTestRouter(t)
	body := `{"username":"admin","password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ─── Services – list ──────────────────────────────────────────────────────────

func TestListServices_ReturnsPaginatedList(t *testing.T) {
	router, _ := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	data, ok := resp["data"].([]any)
	require.True(t, ok, "data field should be an array")
	assert.NotEmpty(t, data)

	meta, ok := resp["meta"].(map[string]any)
	require.True(t, ok, "meta field should be an object")
	assert.Equal(t, float64(1), meta["page"])
}

func TestListServices_SearchFilter(t *testing.T) {
	router, _ := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services?search=auth", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	data := resp["data"].([]any)
	require.NotEmpty(t, data)
	// All returned services should have "auth" in their name or description.
	for _, item := range data {
		svc := item.(map[string]any)
		name := svc["name"].(string)
		desc := svc["description"].(string)
		hasAuth := contains(name, "auth") || contains(name, "Auth") || contains(desc, "auth") || contains(desc, "Auth")
		assert.True(t, hasAuth, "unexpected service returned by auth search: %s", name)
	}
}

func TestListServices_PaginationParams(t *testing.T) {
	router, _ := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services?page=1&page_size=3", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	data := resp["data"].([]any)
	assert.LessOrEqual(t, len(data), 3)
}

// ─── Services – single ────────────────────────────────────────────────────────

func TestGetService_WithVersionCount(t *testing.T) {
	router, _ := newTestRouter(t)

	// Get the list to find a known service ID.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var listResp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&listResp))
	data := listResp["data"].([]any)
	require.NotEmpty(t, data)

	firstSvc := data[0].(map[string]any)
	id := firstSvc["id"].(string)

	// Now fetch the individual service.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/services/"+id, nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)

	var svcResp map[string]any
	require.NoError(t, json.NewDecoder(rec2.Body).Decode(&svcResp))
	assert.Equal(t, id, svcResp["id"])
	// version_count should be present and >= 0.
	_, hasVersionCount := svcResp["version_count"]
	assert.True(t, hasVersionCount, "version_count should be present")
	// versions array should be embedded.
	versions, hasVersions := svcResp["versions"]
	assert.True(t, hasVersions, "versions array should be embedded")
	assert.IsType(t, []any{}, versions)
}

func TestGetService_NotFound(t *testing.T) {
	router, _ := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/00000000-0000-0000-0000-000000000000", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// ─── Services – mutations (protected) ────────────────────────────────────────

func TestCreateService_WithoutAuth_Returns401(t *testing.T) {
	router, _ := newTestRouter(t)
	body := `{"name":"New Service","description":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/services", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestCreateService_WithValidJWT(t *testing.T) {
	router, _ := newTestRouter(t)
	token := getAuthToken(t, router)

	body := `{"name":"Brand New Service","description":"a test service"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/services", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body.String())

	var svc map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&svc))
	assert.Equal(t, "Brand New Service", svc["name"])
	assert.NotEmpty(t, svc["id"])
}

func TestCreateService_ValidationError(t *testing.T) {
	router, _ := newTestRouter(t)
	token := getAuthToken(t, router)

	body := `{"name":"","description":"missing name"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/services", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestUpdateService_WithValidJWT(t *testing.T) {
	router, _ := newTestRouter(t)
	token := getAuthToken(t, router)

	// Create a service to update.
	createBody := `{"name":"To Update","description":"original"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/services", bytes.NewBufferString(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+token)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	require.Equal(t, http.StatusCreated, createRec.Code)

	var created map[string]any
	require.NoError(t, json.NewDecoder(createRec.Body).Decode(&created))
	id := created["id"].(string)

	// Update it.
	updateBody := `{"name":"Updated Name","description":"updated description"}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/services/"+id, bytes.NewBufferString(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("Authorization", "Bearer "+token)
	updateRec := httptest.NewRecorder()
	router.ServeHTTP(updateRec, updateReq)
	require.Equal(t, http.StatusOK, updateRec.Code, "body: %s", updateRec.Body.String())

	var updated map[string]any
	require.NoError(t, json.NewDecoder(updateRec.Body).Decode(&updated))
	assert.Equal(t, "Updated Name", updated["name"])
}

func TestUpdateService_WithoutAuth_Returns401(t *testing.T) {
	router, _ := newTestRouter(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/services/some-id", bytes.NewBufferString(`{"name":"x","description":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestDeleteService_WithValidJWT(t *testing.T) {
	router, _ := newTestRouter(t)
	token := getAuthToken(t, router)

	// Create a service.
	createBody := `{"name":"To Delete","description":""}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/services", bytes.NewBufferString(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+token)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	require.Equal(t, http.StatusCreated, createRec.Code)

	var created map[string]any
	require.NoError(t, json.NewDecoder(createRec.Body).Decode(&created))
	id := created["id"].(string)

	// Delete it.
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/services/"+id, nil)
	deleteReq.Header.Set("Authorization", "Bearer "+token)
	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, deleteReq)
	assert.Equal(t, http.StatusNoContent, deleteRec.Code)

	// Confirm 404 afterwards.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/services/"+id, nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	assert.Equal(t, http.StatusNotFound, getRec.Code)
}

func TestDeleteService_WithoutAuth_Returns401(t *testing.T) {
	router, _ := newTestRouter(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/services/some-id", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ─── Versions ─────────────────────────────────────────────────────────────────

func TestListVersions_ForExistingService(t *testing.T) {
	router, _ := newTestRouter(t)

	// Get a service ID.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var listResp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&listResp))
	data := listResp["data"].([]any)
	require.NotEmpty(t, data)
	id := data[0].(map[string]any)["id"].(string)

	req2 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/services/%s/versions", id), nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)

	var versions []any
	require.NoError(t, json.NewDecoder(rec2.Body).Decode(&versions))
	assert.NotEmpty(t, versions)
}

func TestCreateVersion_WithValidJWT(t *testing.T) {
	router, _ := newTestRouter(t)
	token := getAuthToken(t, router)

	// Create a service.
	createBody := `{"name":"Versioned Service","description":""}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/services", bytes.NewBufferString(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+token)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	require.Equal(t, http.StatusCreated, createRec.Code)

	var created map[string]any
	require.NoError(t, json.NewDecoder(createRec.Body).Decode(&created))
	id := created["id"].(string)

	// Add a version.
	vBody := `{"tag":"v1.0.0","status":"active"}`
	vReq := httptest.NewRequest(http.MethodPost, "/api/v1/services/"+id+"/versions", bytes.NewBufferString(vBody))
	vReq.Header.Set("Content-Type", "application/json")
	vReq.Header.Set("Authorization", "Bearer "+token)
	vRec := httptest.NewRecorder()
	router.ServeHTTP(vRec, vReq)
	require.Equal(t, http.StatusCreated, vRec.Code, "body: %s", vRec.Body.String())

	var ver map[string]any
	require.NoError(t, json.NewDecoder(vRec.Body).Decode(&ver))
	assert.Equal(t, "v1.0.0", ver["tag"])
	assert.Equal(t, "active", ver["status"])
}

func TestDeleteVersion_WithValidJWT(t *testing.T) {
	router, _ := newTestRouter(t)
	token := getAuthToken(t, router)

	// Create service + version.
	createBody := `{"name":"Temp Service","description":""}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/services", bytes.NewBufferString(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+token)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	require.Equal(t, http.StatusCreated, createRec.Code)

	var svc map[string]any
	require.NoError(t, json.NewDecoder(createRec.Body).Decode(&svc))
	svcID := svc["id"].(string)

	vBody := `{"tag":"v1.0.0","status":"beta"}`
	vReq := httptest.NewRequest(http.MethodPost, "/api/v1/services/"+svcID+"/versions", bytes.NewBufferString(vBody))
	vReq.Header.Set("Content-Type", "application/json")
	vReq.Header.Set("Authorization", "Bearer "+token)
	vRec := httptest.NewRecorder()
	router.ServeHTTP(vRec, vReq)
	require.Equal(t, http.StatusCreated, vRec.Code)

	var ver map[string]any
	require.NoError(t, json.NewDecoder(vRec.Body).Decode(&ver))
	vid := ver["id"].(string)

	// Delete the version.
	delReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/services/%s/versions/%s", svcID, vid), nil)
	delReq.Header.Set("Authorization", "Bearer "+token)
	delRec := httptest.NewRecorder()
	router.ServeHTTP(delRec, delReq)
	assert.Equal(t, http.StatusNoContent, delRec.Code)
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
