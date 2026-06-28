package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Architkumar13/services-catalog-api/internal/domain"
	"github.com/Architkumar13/services-catalog-api/internal/handler"
	"github.com/Architkumar13/services-catalog-api/internal/repository/postgres"
	"github.com/Architkumar13/services-catalog-api/internal/repository/sqlite"
	"github.com/Architkumar13/services-catalog-api/internal/service"
)

// seedableStore is satisfied by both the SQLite and Postgres stores.
type seedableStore interface {
	domain.ServiceRepository
	Seed(ctx context.Context) error
	Close() error
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	port := envOrDefault("PORT", "8080")
	jwtSecret := envOrDefault("JWT_SECRET", "dev-secret-change-in-production")

	if jwtSecret == "dev-secret-change-in-production" {
		logger.Warn("JWT_SECRET is using the default insecure value — set JWT_SECRET in production")
	}

	// Select persistence backend: Postgres if DATABASE_URL is set, otherwise SQLite.
	var store seedableStore
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		logger.Info("using PostgreSQL backend")
		pg, err := postgres.New(dbURL)
		if err != nil {
			logger.Error("failed to connect to PostgreSQL", slog.String("error", err.Error()))
			os.Exit(1)
		}
		store = pg
	} else {
		dsn := envOrDefault("DATABASE_DSN", "services.db")
		logger.Info("using SQLite backend", slog.String("dsn", dsn))
		sq, err := sqlite.New(dsn)
		if err != nil {
			logger.Error("failed to open SQLite database", slog.String("error", err.Error()))
			os.Exit(1)
		}
		store = sq
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.Seed(ctx); err != nil {
		logger.Error("failed to seed database", slog.String("error", err.Error()))
		os.Exit(1)
	}

	catalog := service.New(store)
	h := handler.New(catalog, jwtSecret, logger)
	router := handler.NewRouter(h)

	addr := fmt.Sprintf(":%s", port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server starting", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		logger.Info("received shutdown signal", slog.String("signal", sig.String()))
	case err := <-serverErr:
		logger.Error("server error", slog.String("error", err.Error()))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("server stopped gracefully")
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
