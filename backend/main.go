package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ocm-backend/internal/ai"
	"ocm-backend/internal/attendance"
	"ocm-backend/internal/auth"
	"ocm-backend/internal/authz"
	"ocm-backend/internal/booking"
	"ocm-backend/internal/classroom"
	"ocm-backend/internal/course"
	"ocm-backend/internal/db"
	"ocm-backend/internal/httpx"
	"ocm-backend/internal/iam"
	"ocm-backend/internal/importer"
	"ocm-backend/internal/logging"
	"ocm-backend/internal/mail"
	"ocm-backend/internal/middleware"
	"ocm-backend/internal/observation"
	"ocm-backend/internal/schedule"
	"ocm-backend/internal/storage"
	"ocm-backend/internal/systemlog"
	"ocm-backend/internal/user"
)

func main() {
	// Structured terminal logging for developers and operators (business audit
	// records live in system_logs, see internal/systemlog). Must run before
	// anything logs.
	logging.Init()

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

	// Handler stack order matters: AccessLog must stay OUTSIDE Recover so a
	// recovered panic still gets its final 500 recorded in the access line
	// (inverted, the panic unwinds past AccessLog first and the line shows
	// status 0). Recover logs the panic itself; AccessLog logs the completed
	// request — each event exactly once.
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: middleware.AccessLog(httpx.Recover(mux)),
	}

	go func() {
		logging.L.Info("ocm-backend listening", "port", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logging.L.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	database, err := openDB(ctx)
	if err != nil {
		logging.L.Error("database", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := database.Close(); err != nil {
			logging.L.Error("database close", "err", err)
		}
	}()

	tokenService := auth.NewTokenService()
	wxService := auth.NewWxService()

	authStore := auth.NewStore(database)
	if err := authStore.Migrate(ctx); err != nil {
		logging.L.Error("auth migration", "err", err)
		os.Exit(1)
	}
	// iam.Migrate must run after auth.Migrate (users table exists) and before
	// any route serves traffic: it migrates the legacy users.role column into
	// user_roles grants, then drops the column.
	iamStore := iam.NewStore(database)
	if err := iamStore.Migrate(ctx); err != nil {
		logging.L.Error("iam migration", "err", err)
		os.Exit(1)
	}
	// systemlog depends on nothing; migrate it before auth routes register
	// because the auth handler writes explicit audit rows (login events).
	systemlogStore := systemlog.NewStore(database)
	if err := systemlogStore.Migrate(ctx); err != nil {
		logging.L.Error("systemlog migration", "err", err)
		os.Exit(1)
	}
	// mail/storage settings have no dependencies; migrate them here so the
	// settings routes below can serve immediately.
	mailStore := mail.NewStore(database)
	if err := mailStore.Migrate(ctx); err != nil {
		logging.L.Error("mail migration", "err", err)
		os.Exit(1)
	}
	storageStore := storage.NewStore(database)
	if err := storageStore.Migrate(ctx); err != nil {
		logging.L.Error("storage migration", "err", err)
		os.Exit(1)
	}
	auth.NewHandler(authStore, tokenService, wxService, iamStore, systemlogStore).RegisterRoutes(mux)

	userStore := user.NewStore(database)
	if err := userStore.Migrate(ctx); err != nil {
		logging.L.Error("user org migration", "err", err)
		os.Exit(1)
	}
	// The audit middleware sits inside the auth pipeline — after LoadSubject
	// (Subject available, zero extra queries) and before RequirePermission
	// (403 rejections recorded too). Auth routes are not inside this chain and
	// record login/bind events explicitly instead.
	authenticate := func(next http.Handler) http.Handler {
		return auth.Middleware(tokenService)(user.LoadSubject(userStore, iamStore)(
			systemlog.Audit(systemlogStore)(next)))
	}
	user.NewHandler(userStore, iamStore).RegisterRoutes(mux, authenticate)
	iam.NewHandler(iamStore).RegisterRoutes(mux, authenticate)

	classroomStore := classroom.NewStore(database)
	if err := classroomStore.Migrate(ctx); err != nil {
		logging.L.Error("classroom migration", "err", err)
		os.Exit(1)
	}
	classroom.NewHandler(classroomStore).RegisterRoutes(mux, authenticate)

	// 教室报修. Repair tickets live in the classroom package (not a separate
	// module): they reference a classroom and are managed alongside it. The
	// ticket CRUD/assign/confirm is generic; images is reserved for a future
	// object-storage integration.
	repairStore := classroom.NewRepairStore(database)
	if err := repairStore.Migrate(ctx); err != nil {
		logging.L.Error("repair migration", "err", err)
		os.Exit(1)
	}
	classroom.NewRepairHandler(repairStore).RegisterRoutes(mux, authenticate)

	scheduleStore := schedule.NewStore(database)
	if err := scheduleStore.Migrate(ctx); err != nil {
		logging.L.Error("schedule migration", "err", err)
		os.Exit(1)
	}
	schedule.NewHandler(scheduleStore).RegisterRoutes(mux, authenticate)

	courseStore := course.NewStore(database)
	if err := courseStore.Migrate(ctx); err != nil {
		logging.L.Error("course migration", "err", err)
		os.Exit(1)
	}
	course.NewHandler(courseStore, classroomStore, scheduleStore).RegisterRoutes(mux, authenticate)

	bookingStore := booking.NewStore(database)
	if err := bookingStore.Migrate(ctx); err != nil {
		logging.L.Error("booking migration", "err", err)
		os.Exit(1)
	}
	booking.NewHandler(bookingStore, classroomStore, scheduleStore).RegisterRoutes(mux, authenticate)

	attendanceStore := attendance.NewStore(database)
	if err := attendanceStore.Migrate(ctx); err != nil {
		logging.L.Error("attendance migration", "err", err)
		os.Exit(1)
	}
	attendance.NewHandler(attendanceStore).RegisterRoutes(mux, authenticate)

	// 听课评课. The record CRUD/submit lives here in the open-source layer; the
	// school-specific document backend (form templates + .docx fillers) is
	// injected as a Renderer by a customization layer. Shipping nil keeps the
	// module fully functional for CRUD/submit and disables the templates/export
	// endpoints until a deployment plugs its own backend in.
	observationStore := observation.NewStore(database)
	if err := observationStore.Migrate(ctx); err != nil {
		logging.L.Error("observation migration", "err", err)
		os.Exit(1)
	}
	observation.NewHandler(observationStore, nil).RegisterRoutes(mux, authenticate)

	// AI assistant: settings (admin-only) + streaming chat. Its tools query
	// classrooms/schedule/course/booking, so it wires after all of them.
	aiStore := ai.NewStore(database)
	if err := aiStore.Migrate(ctx); err != nil {
		logging.L.Error("ai migration", "err", err)
		os.Exit(1)
	}
	ai.NewHandler(aiStore, classroomStore, scheduleStore, courseStore, bookingStore).RegisterRoutes(mux, authenticate)

	importerStore := importer.NewStore(database)
	if err := importerStore.Migrate(ctx); err != nil {
		logging.L.Error("importer migration", "err", err)
		os.Exit(1)
	}
	// Register every business-table importer with the permission that gates its
	// manual manage action, so importing classrooms needs classroom:manage,
	// sessions/catalog/offerings/regimes need course:manage, etc. Bookings are a
	// privileged restore (can recreate approved bookings) so they require
	// booking:approve (admin only).
	registry := importer.NewRegistry()
	registry.Register(importer.JobTypeSessions, authz.CourseManage,
		importer.NewSessionsImporter(database, classroomStore, courseStore, scheduleStore))
	registry.Register(importer.JobTypeClassrooms, authz.ClassroomManage,
		importer.NewClassroomsImporter(database))
	registry.Register(importer.JobTypeAdminClasses, authz.AdminClassManage,
		importer.NewAdminClassesImporter(database))
	registry.Register(importer.JobTypeTeachingClasses, authz.TeachingClassManage,
		importer.NewTeachingClassesImporter(database))
	registry.Register(importer.JobTypeCatalog, authz.CourseManage,
		importer.NewCatalogImporter(database))
	registry.Register(importer.JobTypeOfferings, authz.CourseManage,
		importer.NewOfferingsImporter(database, courseStore, userStore))
	registry.Register(importer.JobTypeRegimes, authz.CourseManage,
		importer.NewRegimesImporter(database))
	registry.Register(importer.JobTypeBookings, authz.BookingApprove,
		importer.NewBookingsImporter(database, classroomStore, scheduleStore))
	importerHandler := importer.NewHandler(importerStore, registry, scheduleStore, systemlogStore)
	importerHandler.RecoverStale(ctx)
	importerHandler.RegisterRoutes(mux, authenticate)

	systemlog.NewHandler(systemlogStore).RegisterRoutes(mux, authenticate)
	// Admin-only system settings (admin-gated inside the handlers).
	mail.NewHandler(mailStore).RegisterRoutes(mux, authenticate)
	storage.NewHandler(storageStore).RegisterRoutes(mux, authenticate)

	// Retention cleanup: purge once now and then daily. Recording itself is
	// never disabled by settings — retention only controls deletion.
	go systemlogStore.RunRetentionLoop(ctx, 24*time.Hour)

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
		logging.L.Error("graceful shutdown error", "err", err)
	}
	logging.L.Info("server stopped")
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
	logging.L.Info("database connected", "host", cfg.Host, "port", cfg.Port, "name", cfg.Name)
	return d, nil
}
