package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	port := envOr("PORT", "8080")
	jwtSecret := envOr("JWT_SECRET", "dev-secret-change-in-production")
	dbPath := envOr("DB_PATH", "services.db")

	if jwtSecret == "dev-secret-change-in-production" {
		log.Println("warning: JWT_SECRET is set to the default insecure value")
	}

	db, err := newStore(dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.close()

	if err := db.seed(context.Background()); err != nil {
		log.Fatalf("failed to seed database: %v", err)
	}

	a := &app{store: db, jwtSecret: jwtSecret}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", a.health)

	mux.HandleFunc("POST /api/v1/auth/token", a.issueToken)

	mux.HandleFunc("GET /api/v1/services", a.listServices)
	mux.HandleFunc("POST /api/v1/services", a.requireAuth(a.createService))
	mux.HandleFunc("GET /api/v1/services/{id}", a.getService)
	mux.HandleFunc("PUT /api/v1/services/{id}", a.requireAuth(a.updateService))
	mux.HandleFunc("DELETE /api/v1/services/{id}", a.requireAuth(a.deleteService))
	mux.HandleFunc("GET /api/v1/services/{id}/versions", a.listVersions)
	mux.HandleFunc("POST /api/v1/services/{id}/versions", a.requireAuth(a.createVersion))
	mux.HandleFunc("DELETE /api/v1/services/{id}/versions/{vid}", a.requireAuth(a.deleteVersion))

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
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
