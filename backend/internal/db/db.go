package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"ocm-backend/internal/logging"
)

const (
	defaultHost            = "localhost"
	defaultPort            = "3306"
	defaultUser            = "ocm"
	defaultPassword        = "ocm"
	defaultName            = "ocm"
	defaultMaxOpenConns    = 25
	defaultMaxIdleConns    = 25
	defaultConnMaxLifetime = 5 * time.Minute
	pingInterval           = 2 * time.Second
)

// Config holds the settings needed to open a MySQL connection.
type Config struct {
	Host            string
	Port            string
	User            string
	Password        string
	Name            string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// ConfigFromEnv builds a Config from DB_* environment variables.
func ConfigFromEnv() Config {
	return Config{
		Host:            envOrDefault("DB_HOST", defaultHost),
		Port:            envOrDefault("DB_PORT", defaultPort),
		User:            envOrDefault("DB_USER", defaultUser),
		Password:        envOrDefault("DB_PASSWORD", defaultPassword),
		Name:            envOrDefault("DB_NAME", defaultName),
		MaxOpenConns:    envIntOrDefault("DB_MAX_OPEN_CONNS", defaultMaxOpenConns),
		MaxIdleConns:    envIntOrDefault("DB_MAX_IDLE_CONNS", defaultMaxIdleConns),
		ConnMaxLifetime: envDurationOrDefault("DB_CONN_MAX_LIFETIME", defaultConnMaxLifetime),
	}
}

// DSN renders a MySQL data source name. utf8mb4 enables full Unicode support
// (including emoji) and parseTime maps MySQL timestamps to time.Time.
// caching_sha2_password over plain TCP needs no extra flag: driver v1.10+
// fetches the server public key automatically (the old
// allowPublicKeyRetrieval parameter was removed and now fails as an unknown
// system variable).
func (c Config) DSN() string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		c.User, c.Password, c.Host, c.Port, c.Name,
	)
}

// New opens a MySQL connection pool, tunes it, and verifies reachability by
// pinging until ctx is cancelled. Callers should pass a context with a timeout
// so startup fails fast when the database is unreachable.
func New(ctx context.Context, cfg Config) (*sql.DB, error) {
	// Refuse to silently fall back to localhost outside development. Every
	// real environment (compose, CI, serverless) sets DB_HOST; a missing value
	// means a misconfigured deployment that would otherwise hang retrying
	// localhost. Dev opts out via APP_ENV=development.
	if os.Getenv("DB_HOST") == "" && appEnv() != "development" {
		return nil, fmt.Errorf("DB_HOST is required in production (APP_ENV=%q); refusing localhost fallback", appEnv())
	}

	d, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	d.SetMaxOpenConns(cfg.MaxOpenConns)
	d.SetMaxIdleConns(cfg.MaxIdleConns)
	d.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if err := pingWithRetry(ctx, d); err != nil {
		_ = d.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	return d, nil
}

// pingWithRetry polls the database until it responds or ctx is cancelled.
func pingWithRetry(ctx context.Context, d *sql.DB) error {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	var lastErr error
	for {
		if err := d.PingContext(ctx); err == nil {
			return nil
		} else {
			lastErr = err
			logging.L.Info("db: not reachable yet, retrying", "interval", pingInterval, "err", err)
		}

		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("%w: %v", err, lastErr)
			}
			return lastErr
		case <-ticker.C:
		}
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// appEnv returns the deployment mode, defaulting to "production" so a
// deployment that forgets to set APP_ENV gets strict fail-fast behavior.
// Development environments opt in with APP_ENV=development.
func appEnv() string {
	return envOrDefault("APP_ENV", "production")
}

func envIntOrDefault(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envDurationOrDefault(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
