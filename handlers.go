package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

type app struct {
	store     *store
	jwtSecret string
}

// ─── validation ───────────────────────────────────────────────────────────────

// tagRegex allows tags like v1.0.0, v2.1.0-beta, v3.0.0-rc.1.
var tagRegex = regexp.MustCompile(`^v\d+\.\d+(\.\d+)?(-[a-zA-Z0-9.]+)?$`)

var validStatuses = map[string]bool{
	"active":     true,
	"deprecated": true,
	"beta":       true,
}

func validateService(name, desc string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name is required")
	}
	if len(name) > 100 {
		return errors.New("name must be 100 characters or fewer")
	}
	if len(strings.TrimSpace(desc)) > 500 {
		return errors.New("description must be 500 characters or fewer")
	}
	return nil
}

func validateVersion(tag, status string) error {
	if tag == "" {
		return errors.New("tag is required")
	}
	if !tagRegex.MatchString(tag) {
		return errors.New("tag must follow vMAJOR.MINOR[.PATCH][-pre] format (e.g. v1.0.0)")
	}
	if status != "" && !validStatuses[status] {
		return errors.New("status must be one of: active, deprecated, beta")
	}
	return nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// requireAuth wraps a handler and rejects requests without a valid JWT.
func (a *app) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing or invalid authorization header")
			return
		}
		tokenStr := strings.TrimPrefix(header, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(a.jwtSecret), nil
		})
		if err != nil || !token.Valid {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		next(w, r)
	}
}

// ─── health ───────────────────────────────────────────────────────────────────

func (a *app) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ─── auth ─────────────────────────────────────────────────────────────────────

func (a *app) issueToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Username != "admin" || req.Password != "secret" {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	expiresAt := time.Now().UTC().Add(24 * time.Hour)
	claims := jwt.MapClaims{
		"sub": req.Username,
		"exp": expiresAt.Unix(),
		"iat": time.Now().UTC().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(a.jwtSecret))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to sign token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      signed,
		"expires_at": expiresAt,
	})
}

// ─── services ─────────────────────────────────────────────────────────────────

func (a *app) listServices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	} else if pageSize > 100 {
		pageSize = 100
	}

	services, total, err := a.store.listServices(r.Context(), q.Get("search"), q.Get("sort"), q.Get("order"), page, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list services")
		return
	}
	if services == nil {
		services = []Service{}
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": services,
		"meta": map[string]int{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

func (a *app) getService(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	svc, err := a.store.getService(r.Context(), id)
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get service")
		return
	}

	versions, err := a.store.listVersions(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get versions")
		return
	}
	if versions == nil {
		versions = []Version{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":            svc.ID,
		"name":          svc.Name,
		"description":   svc.Description,
		"version_count": svc.VersionCount,
		"created_at":    svc.CreatedAt,
		"updated_at":    svc.UpdatedAt,
		"versions":      versions,
	})
}

func (a *app) createService(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)

	if err := validateService(req.Name, req.Description); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	svc, err := a.store.createService(r.Context(), req.Name, req.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create service")
		return
	}
	writeJSON(w, http.StatusCreated, svc)
}

func (a *app) updateService(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)

	if err := validateService(req.Name, req.Description); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	svc, err := a.store.updateService(r.Context(), id, req.Name, req.Description)
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update service")
		return
	}
	writeJSON(w, http.StatusOK, svc)
}

func (a *app) deleteService(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := a.store.deleteService(r.Context(), id); errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "service not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete service")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── versions ─────────────────────────────────────────────────────────────────

func (a *app) listVersions(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")
	if _, err := a.store.getService(r.Context(), serviceID); errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "service not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get service")
		return
	}

	versions, err := a.store.listVersions(r.Context(), serviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list versions")
		return
	}
	if versions == nil {
		versions = []Version{}
	}
	writeJSON(w, http.StatusOK, versions)
}

func (a *app) createVersion(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")

	var req struct {
		Tag    string `json:"tag"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Status == "" {
		req.Status = "active"
	}
	if err := validateVersion(req.Tag, req.Status); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	if _, err := a.store.getService(r.Context(), serviceID); errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "service not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get service")
		return
	}

	ver, err := a.store.createVersion(r.Context(), serviceID, req.Tag, req.Status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create version")
		return
	}
	writeJSON(w, http.StatusCreated, ver)
}

func (a *app) deleteVersion(w http.ResponseWriter, r *http.Request) {
	vid := chi.URLParam(r, "vid")
	if err := a.store.deleteVersion(r.Context(), vid); errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "version not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete version")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
