package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ocm-backend/internal/auth"
	"ocm-backend/internal/db"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := openDB(ctx)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("database close: %v", err)
		}
	}()

	mux := http.NewServeMux()

	authStore := auth.NewStore(database)
	if err := authStore.Migrate(ctx); err != nil {
		log.Fatalf("auth migration: %v", err)
	}
	auth.NewHandler(authStore, auth.NewTokenService()).RegisterRoutes(mux)

	// Liveness probe - the process is up.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Readiness probe - the process can serve requests (database reachable).
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := database.PingContext(ctx); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// API root - placeholder for future routes.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"service":"ocm-backend","status":"running"}`))
	})

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		log.Printf("ocm-backend listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	stop() // restore default signal handling for a second interrupt to force-exit.

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown error: %v", err)
	}
	log.Print("server stopped")
}

func openDB(ctx context.Context) (*sql.DB, error) {
	cfg := db.ConfigFromEnv()

	// Give MySQL up to 60s to become reachable on startup.
	connectCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	d, err := db.New(connectCtx, cfg)
	if err != nil {
		return nil, err
	}
	log.Printf("database connected: %s:%s/%s", cfg.Host, cfg.Port, cfg.Name)
	return d, nil
}
