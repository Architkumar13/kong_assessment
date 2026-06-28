package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// ─── unit tests (no database required) ───────────────────────────────────────

func TestValidateService(t *testing.T) {
	cases := []struct {
		name, desc string
		wantErr    bool
	}{
		{"", "", true},
		{"Valid Service", "", false},
		{"Valid Service", "A description", false},
		{strings.Repeat("a", 101), "", true},
		{"Valid", strings.Repeat("b", 501), true},
		{"  ", "", true}, // whitespace-only name
	}
	for _, c := range cases {
		err := validateService(c.name, c.desc)
		if (err != nil) != c.wantErr {
			t.Errorf("validateService(%q, %q): got err=%v, wantErr=%v", c.name, c.desc, err, c.wantErr)
		}
	}
}

func TestValidateVersion(t *testing.T) {
	cases := []struct {
		tag, status string
		wantErr     bool
	}{
		{"", "active", true},
		{"1.0.0", "active", true},   // missing v prefix
		{"v1", "active", true},      // too short
		{"v1.0.0", "active", false},
		{"v1.0", "active", false},
		{"v2.1.0-beta", "beta", false},
		{"v3.0.0-rc.1", "active", false},
		{"v1.0.0", "invalid", true}, // bad status
		{"v1.0.0", "", false},       // empty status is allowed (defaults to active)
	}
	for _, c := range cases {
		err := validateVersion(c.tag, c.status)
		if (err != nil) != c.wantErr {
			t.Errorf("validateVersion(%q, %q): got err=%v, wantErr=%v", c.tag, c.status, err, c.wantErr)
		}
	}
}

func TestRequireAuth(t *testing.T) {
	a := &app{jwtSecret: "test-secret"}

	// No Authorization header → 401.
	req := httptest.NewRequest("POST", "/", strings.NewReader(""))
	w := httptest.NewRecorder()
	a.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("no header: got %d, want 401", w.Code)
	}

	// Garbage token → 401.
	req = httptest.NewRequest("POST", "/", strings.NewReader(""))
	req.Header.Set("Authorization", "Bearer not-a-jwt")
	w = httptest.NewRecorder()
	a.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("bad token: got %d, want 401", w.Code)
	}
}

// ─── integration tests (require TEST_DATABASE_URL) ────────────────────────────

// newTestApp opens a real database and returns a cleanup function that wipes all rows.
func newTestApp(t *testing.T) (*app, func()) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	db, err := newStore(dsn)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	cleanup := func() {
		db.db.Exec("DELETE FROM versions")
		db.db.Exec("DELETE FROM services")
		db.close()
	}
	return &app{store: db, jwtSecret: "test-secret"}, cleanup
}

// authToken obtains a JWT from the issueToken handler.
func authToken(t *testing.T, a *app) string {
	t.Helper()
	body := `{"username":"admin","password":"secret"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.issueToken(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("issueToken: got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	return resp["token"].(string)
}

func TestHealth(t *testing.T) {
	a, cleanup := newTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	a.health(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}
}

func TestIssueToken(t *testing.T) {
	a, cleanup := newTestApp(t)
	defer cleanup()

	// Wrong credentials → 401.
	body := `{"username":"admin","password":"wrong"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.issueToken(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("wrong creds: got %d, want 401", w.Code)
	}

	// Correct credentials → 200 with token.
	token := authToken(t, a)
	if token == "" {
		t.Error("expected non-empty token")
	}
}

func TestListServicesEmpty(t *testing.T) {
	a, cleanup := newTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/v1/services", nil)
	w := httptest.NewRecorder()
	a.listServices(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	data := resp["data"].([]any)
	if len(data) != 0 {
		t.Errorf("expected empty list, got %d items", len(data))
	}
}

func TestServiceCRUD(t *testing.T) {
	a, cleanup := newTestApp(t)
	defer cleanup()
	token := authToken(t, a)

	// Create.
	body := `{"name":"Payment Service","description":"Handles payments"}`
	req := httptest.NewRequest("POST", "/api/v1/services", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	a.createService(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d — %s", w.Code, w.Body.String())
	}
	var svc map[string]any
	json.NewDecoder(w.Body).Decode(&svc)
	id := svc["id"].(string)

	// Get.
	req = httptest.NewRequest("GET", "/api/v1/services/"+id, nil)
	req.SetPathValue("id", id)
	w = httptest.NewRecorder()
	a.getService(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get: got %d", w.Code)
	}

	// Update.
	body = `{"name":"Payment Service v2","description":"Updated"}`
	req = httptest.NewRequest("PUT", "/api/v1/services/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.SetPathValue("id", id)
	w = httptest.NewRecorder()
	a.updateService(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update: got %d", w.Code)
	}

	// Delete.
	req = httptest.NewRequest("DELETE", "/api/v1/services/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.SetPathValue("id", id)
	w = httptest.NewRecorder()
	a.deleteService(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: got %d", w.Code)
	}

	// Get after delete → 404.
	req = httptest.NewRequest("GET", "/api/v1/services/"+id, nil)
	req.SetPathValue("id", id)
	w = httptest.NewRecorder()
	a.getService(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get after delete: got %d, want 404", w.Code)
	}
}

func TestCreateServiceValidation(t *testing.T) {
	a, cleanup := newTestApp(t)
	defer cleanup()
	token := authToken(t, a)

	cases := []struct {
		body       string
		wantStatus int
	}{
		{`{"name":"","description":""}`, http.StatusUnprocessableEntity},
		{`{"name":"` + strings.Repeat("a", 101) + `"}`, http.StatusUnprocessableEntity},
		{`not json`, http.StatusBadRequest},
	}

	for _, c := range cases {
		req := httptest.NewRequest("POST", "/api/v1/services", strings.NewReader(c.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		a.createService(w, req)
		if w.Code != c.wantStatus {
			t.Errorf("body=%q: got %d, want %d", c.body, w.Code, c.wantStatus)
		}
	}
}

func TestVersionCRUD(t *testing.T) {
	a, cleanup := newTestApp(t)
	defer cleanup()
	token := authToken(t, a)

	// Create a parent service first.
	req := httptest.NewRequest("POST", "/api/v1/services", strings.NewReader(`{"name":"Auth","description":""}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	a.createService(w, req)
	var svc map[string]any
	json.NewDecoder(w.Body).Decode(&svc)
	svcID := svc["id"].(string)

	// Create version.
	req = httptest.NewRequest("POST", "/api/v1/services/"+svcID+"/versions", strings.NewReader(`{"tag":"v1.0.0","status":"active"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.SetPathValue("id", svcID)
	w = httptest.NewRecorder()
	a.createVersion(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create version: got %d — %s", w.Code, w.Body.String())
	}
	var ver map[string]any
	json.NewDecoder(w.Body).Decode(&ver)
	verID := ver["id"].(string)

	// List versions.
	req = httptest.NewRequest("GET", "/api/v1/services/"+svcID+"/versions", nil)
	req.SetPathValue("id", svcID)
	w = httptest.NewRecorder()
	a.listVersions(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list versions: got %d", w.Code)
	}
	var versions []any
	json.NewDecoder(w.Body).Decode(&versions)
	if len(versions) != 1 {
		t.Errorf("expected 1 version, got %d", len(versions))
	}

	// Delete version.
	req = httptest.NewRequest("DELETE", "/api/v1/services/"+svcID+"/versions/"+verID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.SetPathValue("vid", verID)
	w = httptest.NewRecorder()
	a.deleteVersion(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete version: got %d", w.Code)
	}
}

func TestSearchAndPagination(t *testing.T) {
	a, cleanup := newTestApp(t)
	defer cleanup()
	token := authToken(t, a)

	// Insert two services.
	for _, name := range []string{"Alpha Service", "Beta Service"} {
		body := `{"name":"` + name + `","description":"desc"}`
		req := httptest.NewRequest("POST", "/api/v1/services", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		a.createService(w, req)
	}

	// Search for "Alpha".
	req := httptest.NewRequest("GET", "/api/v1/services?search=Alpha", nil)
	w := httptest.NewRecorder()
	a.listServices(w, req)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	data := resp["data"].([]any)
	if len(data) != 1 {
		t.Errorf("search=Alpha: expected 1 result, got %d", len(data))
	}

	// page_size=1 should return 1 item with total=2.
	req = httptest.NewRequest("GET", "/api/v1/services?page_size=1", nil)
	w = httptest.NewRecorder()
	a.listServices(w, req)
	json.NewDecoder(w.Body).Decode(&resp)
	data = resp["data"].([]any)
	if len(data) != 1 {
		t.Errorf("page_size=1: expected 1 item, got %d", len(data))
	}
	meta := resp["meta"].(map[string]any)
	if meta["total"].(float64) != 2 {
		t.Errorf("expected total=2, got %v", meta["total"])
	}
}
