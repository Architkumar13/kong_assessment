package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Architkumar13/services-catalog-api/internal/domain"
	"github.com/Architkumar13/services-catalog-api/pkg/apierror"
)

// listMeta is the pagination envelope metadata returned with list responses.
type listMeta struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// listResponse is the standard paginated list envelope.
type listResponse[T any] struct {
	Data []T      `json:"data"`
	Meta listMeta `json:"meta"`
}

// serviceDetailResponse augments Service with its full versions list.
type serviceDetailResponse struct {
	domain.Service
	Versions []domain.Version `json:"versions"`
}

// ListServices handles GET /api/v1/services.
func (h *Handlers) ListServices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))

	filter := domain.ListFilter{
		Search:   q.Get("search"),
		SortBy:   q.Get("sort"),
		Order:    q.Get("order"),
		Page:     page,
		PageSize: pageSize,
	}

	services, total, err := h.catalog.ListServices(r.Context(), filter)
	if err != nil {
		apierror.InternalServerError(w, "failed to list services")
		return
	}

	if services == nil {
		services = []domain.Service{}
	}

	// Re-read the filter through the service layer defaults.
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}

	totalPages := 0
	if filter.PageSize > 0 {
		totalPages = (total + filter.PageSize - 1) / filter.PageSize
	}

	writeJSON(w, http.StatusOK, listResponse[domain.Service]{
		Data: services,
		Meta: listMeta{
			Page:       filter.Page,
			PageSize:   filter.PageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// GetService handles GET /api/v1/services/{id}.
// The response embeds the full versions list alongside the service.
func (h *Handlers) GetService(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	svc, err := h.catalog.GetService(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			apierror.NotFound(w, "service not found")
			return
		}
		apierror.InternalServerError(w, "failed to get service")
		return
	}

	versions, err := h.catalog.ListVersions(r.Context(), id)
	if err != nil {
		apierror.InternalServerError(w, "failed to get versions")
		return
	}
	if versions == nil {
		versions = []domain.Version{}
	}

	writeJSON(w, http.StatusOK, serviceDetailResponse{
		Service:  *svc,
		Versions: versions,
	})
}

// createServiceRequest is the JSON body for POST /api/v1/services.
type createServiceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CreateService handles POST /api/v1/services.
func (h *Handlers) CreateService(w http.ResponseWriter, r *http.Request) {
	var req createServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.BadRequest(w, "invalid JSON body")
		return
	}

	svc, err := h.catalog.CreateService(r.Context(), req.Name, req.Description)
	if err != nil {
		if errors.Is(err, domain.ErrValidation) {
			apierror.UnprocessableEntity(w, err.Error())
			return
		}
		apierror.InternalServerError(w, "failed to create service")
		return
	}

	writeJSON(w, http.StatusCreated, svc)
}

// updateServiceRequest is the JSON body for PUT /api/v1/services/{id}.
type updateServiceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// UpdateService handles PUT /api/v1/services/{id}.
func (h *Handlers) UpdateService(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req updateServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.BadRequest(w, "invalid JSON body")
		return
	}

	svc, err := h.catalog.UpdateService(r.Context(), id, req.Name, req.Description)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			apierror.NotFound(w, "service not found")
			return
		}
		if errors.Is(err, domain.ErrValidation) {
			apierror.UnprocessableEntity(w, err.Error())
			return
		}
		apierror.InternalServerError(w, "failed to update service")
		return
	}

	writeJSON(w, http.StatusOK, svc)
}

// DeleteService handles DELETE /api/v1/services/{id}.
func (h *Handlers) DeleteService(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.catalog.DeleteService(r.Context(), id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			apierror.NotFound(w, "service not found")
			return
		}
		apierror.InternalServerError(w, "failed to delete service")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
