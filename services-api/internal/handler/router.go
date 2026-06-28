package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/Architkumar13/services-catalog-api/internal/handler/middleware"
	"github.com/Architkumar13/services-catalog-api/internal/service"
)

// Handlers holds all dependencies needed by the HTTP layer.
type Handlers struct {
	catalog   *service.CatalogService
	jwtSecret string
	logger    *slog.Logger
}

// New constructs a Handlers instance.
func New(catalog *service.CatalogService, jwtSecret string, logger *slog.Logger) *Handlers {
	return &Handlers{
		catalog:   catalog,
		jwtSecret: jwtSecret,
		logger:    logger,
	}
}

// NewRouter wires the Chi router with all routes and middleware.
func NewRouter(h *Handlers) http.Handler {
	r := chi.NewRouter()

	// Global middleware.
	r.Use(chimiddleware.Recoverer)
	r.Use(corsMiddleware)
	r.Use(middleware.Logger(h.logger))

	// Health check (no auth).
	r.Get("/health", h.Health)

	// API v1.
	r.Route("/api/v1", func(r chi.Router) {
		// Auth – public.
		r.Post("/auth/token", h.IssueToken)

		// Services – read endpoints are public; mutating endpoints require JWT.
		r.Route("/services", func(r chi.Router) {
			r.Get("/", h.ListServices)

			r.Group(func(r chi.Router) {
				r.Use(middleware.Auth(h.jwtSecret))
				r.Post("/", h.CreateService)
			})

			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.GetService)

				r.Group(func(r chi.Router) {
					r.Use(middleware.Auth(h.jwtSecret))
					r.Put("/", h.UpdateService)
					r.Delete("/", h.DeleteService)
				})

				// Versions sub-resource.
				r.Route("/versions", func(r chi.Router) {
					r.Get("/", h.ListVersions)

					r.Group(func(r chi.Router) {
						r.Use(middleware.Auth(h.jwtSecret))
						r.Post("/", h.CreateVersion)
						r.Delete("/{vid}", h.DeleteVersion)
					})
				})
			})
		})
	})

	return r
}

// Health handles GET /health.
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// writeJSON serialises v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// corsMiddleware is a simple middleware that allows all origins (suitable for development).
// In production, restrict allowed origins appropriately.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
