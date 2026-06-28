package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
)

func main() {
	port := envOr("PORT", "8080")
	jwtSecret := envOr("JWT_SECRET", "dev-secret-change-in-production")
	dbURL := os.Getenv("DATABASE_URL")

	if jwtSecret == "dev-secret-change-in-production" {
		log.Println("warning: JWT_SECRET is set to the default insecure value")
	}
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	db, err := newStore(dbURL)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.close()

	if err := db.seed(context.Background()); err != nil {
		log.Fatalf("failed to seed database: %v", err)
	}

	a := &app{store: db, jwtSecret: jwtSecret}

	r := chi.NewRouter()

	r.Get("/health", a.health)

	r.Post("/api/v1/auth/token", a.issueToken)

	r.Get("/api/v1/services", a.listServices)
	r.Post("/api/v1/services", a.requireAuth(a.createService))
	r.Get("/api/v1/services/{id}", a.getService)
	r.Put("/api/v1/services/{id}", a.requireAuth(a.updateService))
	r.Delete("/api/v1/services/{id}", a.requireAuth(a.deleteService))
	r.Get("/api/v1/services/{id}/versions", a.listVersions)
	r.Post("/api/v1/services/{id}/versions", a.requireAuth(a.createVersion))
	r.Delete("/api/v1/services/{id}/versions/{vid}", a.requireAuth(a.deleteVersion))

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("server listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Println("server stopped")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
