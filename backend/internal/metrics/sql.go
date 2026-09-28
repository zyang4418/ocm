package metrics

import (
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"
)

// ObserveSQLDB registers gauges for a *sql.DB pool: open/idle/in-use
// connections, the configured maximum and the cumulative time requests spent
// waiting for a connection. Values are read at scrape time via GaugeFunc —
// no sampling goroutine, no state — so registration happens once at startup
// and costs nothing until someone actually scrapes.
func ObserveSQLDB(db *sql.DB) {
	prometheus.MustRegister(
		prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{Name: "ocm_db_connections_open", Help: "Currently open connections in the pool."},
			func() float64 { return float64(db.Stats().OpenConnections) },
		),
		prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{Name: "ocm_db_connections_idle", Help: "Currently idle connections in the pool."},
			func() float64 { return float64(db.Stats().Idle) },
		),
		prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{Name: "ocm_db_connections_in_use", Help: "Connections currently in use by queries."},
			func() float64 { return float64(db.Stats().InUse) },
		),
		prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{Name: "ocm_db_max_open_connections", Help: "Configured pool maximum (0 means unlimited)."},
			func() float64 { return float64(db.Stats().MaxOpenConnections) },
		),
		prometheus.NewCounterFunc(
			prometheus.CounterOpts{Name: "ocm_db_wait_count_total", Help: "Total number of connections waited for since startup."},
			func() float64 { return float64(db.Stats().WaitCount) },
		),
		prometheus.NewCounterFunc(
			prometheus.CounterOpts{Name: "ocm_db_wait_duration_seconds_total", Help: "Total time spent waiting for a connection since startup."},
			func() float64 { return db.Stats().WaitDuration.Seconds() },
		),
	)
}
