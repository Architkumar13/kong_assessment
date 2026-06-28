package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Architkumar13/services-catalog-api/internal/domain"
	"github.com/Architkumar13/services-catalog-api/pkg/apierror"
)

// ListVersions handles GET /api/v1/services/{id}/versions.
func (h *Handlers) ListVersions(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")

	versions, err := h.catalog.ListVersions(r.Context(), serviceID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			apierror.NotFound(w, "service not found")
			return
		}
		apierror.InternalServerError(w, "failed to list versions")
		return
	}
	if versions == nil {
		versions = []domain.Version{}
	}

	writeJSON(w, http.StatusOK, versions)
}

// createVersionRequest is the JSON body for POST /api/v1/services/{id}/versions.
type createVersionRequest struct {
	Tag    string `json:"tag"`
	Status string `json:"status"`
}

// CreateVersion handles POST /api/v1/services/{id}/versions.
func (h *Handlers) CreateVersion(w http.ResponseWriter, r *http.Request) {
	serviceID := chi.URLParam(r, "id")

	var req createVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.BadRequest(w, "invalid JSON body")
		return
	}

	ver, err := h.catalog.CreateVersion(r.Context(), serviceID, req.Tag, req.Status)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			apierror.NotFound(w, "service not found")
			return
		}
		if errors.Is(err, domain.ErrValidation) {
			apierror.UnprocessableEntity(w, err.Error())
			return
		}
		apierror.InternalServerError(w, "failed to create version")
		return
	}

	writeJSON(w, http.StatusCreated, ver)
}

// DeleteVersion handles DELETE /api/v1/services/{id}/versions/{vid}.
func (h *Handlers) DeleteVersion(w http.ResponseWriter, r *http.Request) {
	vid := chi.URLParam(r, "vid")

	if err := h.catalog.DeleteVersion(r.Context(), vid); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			apierror.NotFound(w, "version not found")
			return
		}
		apierror.InternalServerError(w, "failed to delete version")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
