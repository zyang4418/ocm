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
	"ocm-backend/internal/booking"
	"ocm-backend/internal/classroom"
	"ocm-backend/internal/course"
	"ocm-backend/internal/db"
	"ocm-backend/internal/schedule"
	"ocm-backend/internal/user"
)

func main() {
	// Container platforms often surface only stdout; route logs there so a
	// crash-looping or startup-blocked process stays diagnosable.
	log.SetOutput(os.Stdout)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()

	// Liveness probe - the process is up. Registered before the database is
	// opened so the port binds immediately: liveness reflects process health,
	// not DB availability, and a slow/blocked DB start no longer causes the
	// platform to restart us before 8080 is bound.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
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

	database, err := openDB(ctx)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("database close: %v", err)
		}
	}()

	tokenService := auth.NewTokenService()

	authStore := auth.NewStore(database)
	if err := authStore.Migrate(ctx); err != nil {
		log.Fatalf("auth migration: %v", err)
	}
	auth.NewHandler(authStore, tokenService).RegisterRoutes(mux)

	userStore := user.NewStore(database)
	authenticate := func(next http.Handler) http.Handler {
		return auth.Middleware(tokenService)(user.LoadSubject(userStore)(next))
	}
	user.NewHandler(userStore).RegisterRoutes(mux, authenticate)

	classroomStore := classroom.NewStore(database)
	if err := classroomStore.Migrate(ctx); err != nil {
		log.Fatalf("classroom migration: %v", err)
	}
	classroom.NewHandler(classroomStore).RegisterRoutes(mux, authenticate)
	scheduleStore := schedule.NewStore(database)
	if err := scheduleStore.Migrate(ctx); err != nil {
		log.Fatalf("schedule migration: %v", err)
	}
	schedule.NewHandler(scheduleStore).RegisterRoutes(mux, authenticate)

	courseStore := course.NewStore(database)
	if err := courseStore.Migrate(ctx); err != nil {
		log.Fatalf("course migration: %v", err)
	}
	course.NewHandler(courseStore, scheduleStore).RegisterRoutes(mux, authenticate)

	bookingStore := booking.NewStore(database)
	if err := bookingStore.Migrate(ctx); err != nil {
		log.Fatalf("booking migration: %v", err)
	}
	booking.NewHandler(bookingStore, classroomStore, scheduleStore).RegisterRoutes(mux, authenticate)

	// Readiness probe - the process can serve requests (database reachable).
	// Registered after the DB is connected so it only reports ready once the
	// app can actually serve auth traffic; until then it 404s (not ready).
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
