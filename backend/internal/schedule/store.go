package schedule

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
)

var (
	ErrRegimeNotFound = errors.New("regime not found")
	ErrNameTaken      = errors.New("regime name already taken")
)

// Store manages bell-time regime and period records.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Migrate creates the regime and period tables. It is idempotent.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schedule_regimes (
    id              BIGINT AUTO_INCREMENT PRIMARY KEY,
    name            VARCHAR(32) NOT NULL UNIQUE,
    effective_month TINYINT NOT NULL,
    effective_day   TINYINT NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return fmt.Errorf("create schedule_regimes table: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schedule_periods (
    id           BIGINT AUTO_INCREMENT PRIMARY KEY,
    regime_id    BIGINT NOT NULL,
    period_index INT NOT NULL,
    start_time   TIME NOT NULL,
    end_time     TIME NOT NULL,
    UNIQUE (regime_id, period_index)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return fmt.Errorf("create schedule_periods table: %w", err)
	}
	return nil
}

func (s *Store) ListRegimes(ctx context.Context) ([]Regime, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, effective_month, effective_day, created_at FROM schedule_regimes ORDER BY effective_month, effective_day, id`)
	if err != nil {
		return nil, fmt.Errorf("list regimes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var regimes []Regime
	for rows.Next() {
		var r Regime
		if err := rows.Scan(&r.ID, &r.Name, &r.EffectiveMonth, &r.EffectiveDay, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan regime: %w", err)
		}
		regimes = append(regimes, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list regimes rows: %w", err)
	}
	for i := range regimes {
		p, err := s.loadPeriods(ctx, regimes[i].ID)
		if err != nil {
			return nil, err
		}
		regimes[i].Periods = p
	}
	return regimes, nil
}

func (s *Store) GetRegime(ctx context.Context, id int64) (Regime, error) {
	var r Regime
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, effective_month, effective_day, created_at FROM schedule_regimes WHERE id = ?`, id,
	).Scan(&r.ID, &r.Name, &r.EffectiveMonth, &r.EffectiveDay, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Regime{}, ErrRegimeNotFound
	}
	if err != nil {
		return Regime{}, fmt.Errorf("get regime: %w", err)
	}
	p, err := s.loadPeriods(ctx, id)
	if err != nil {
		return Regime{}, err
	}
	r.Periods = p
	return r, nil
}

func (s *Store) CreateRegime(ctx context.Context, in RegimeInput) (Regime, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO schedule_regimes (name, effective_month, effective_day) VALUES (?, ?, ?)`,
		in.Name, in.EffectiveMonth, in.EffectiveDay,
	)
	if err != nil {
		if isDuplicateEntry(err) {
			return Regime{}, ErrNameTaken
		}
		return Regime{}, fmt.Errorf("create regime: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Regime{}, fmt.Errorf("create regime last insert id: %w", err)
	}
	return s.GetRegime(ctx, id)
}

func (s *Store) UpdateRegime(ctx context.Context, id int64, in RegimeInput) (Regime, error) {
	_, err := s.db.ExecContext(ctx,
		`UPDATE schedule_regimes SET name = ?, effective_month = ?, effective_day = ? WHERE id = ?`,
		in.Name, in.EffectiveMonth, in.EffectiveDay, id,
	)
	if err != nil {
		if isDuplicateEntry(err) {
			return Regime{}, ErrNameTaken
		}
		return Regime{}, fmt.Errorf("update regime: %w", err)
	}
	return s.GetRegime(ctx, id)
}

func (s *Store) DeleteRegime(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM schedule_periods WHERE regime_id = ?`, id); err != nil {
		return fmt.Errorf("delete periods: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM schedule_regimes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete regime: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete regime rows affected: %w", err)
	}
	if n == 0 {
		return ErrRegimeNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// ReplacePeriods deletes all periods for a regime and inserts the given set in
// a single transaction. The number of periods defines how many class periods a
// day has under this regime.
func (s *Store) ReplacePeriods(ctx context.Context, regimeID int64, periods []PeriodInput) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM schedule_periods WHERE regime_id = ?`, regimeID); err != nil {
		return fmt.Errorf("delete periods: %w", err)
	}
	for _, p := range periods {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schedule_periods (regime_id, period_index, start_time, end_time) VALUES (?, ?, ?, ?)`,
			regimeID, p.PeriodIndex, p.StartTime, p.EndTime,
		); err != nil {
			return fmt.Errorf("insert period: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *Store) loadPeriods(ctx context.Context, regimeID int64) ([]Period, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, regime_id, period_index, start_time, end_time FROM schedule_periods WHERE regime_id = ? ORDER BY period_index`,
		regimeID,
	)
	if err != nil {
		return nil, fmt.Errorf("list periods: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var periods []Period
	for rows.Next() {
		var p Period
		if err := rows.Scan(&p.ID, &p.RegimeID, &p.PeriodIndex, &p.StartTime, &p.EndTime); err != nil {
			return nil, fmt.Errorf("scan period: %w", err)
		}
		p.StartTime = trimSeconds(p.StartTime)
		p.EndTime = trimSeconds(p.EndTime)
		periods = append(periods, p)
	}
	return periods, rows.Err()
}

// ActiveFor returns the regime whose effective date is the most recent on or
// before the given date in the annual cycle. If the date falls before every
// regime's start date, the latest-starting regime wraps around from the
// previous year. It returns false only when regimes is empty.
func ActiveFor(regimes []Regime, date time.Time) (Regime, bool) {
	if len(regimes) == 0 {
		return Regime{}, false
	}
	todayMD := int(date.Month())*100 + date.Day()
	var active Regime
	activeMD := -1
	found := false
	var latest Regime
	latestMD := -1
	for _, r := range regimes {
		md := r.EffectiveMonth*100 + r.EffectiveDay
		if md > latestMD {
			latestMD = md
			latest = r
		}
		if md <= todayMD && md > activeMD {
			activeMD = md
			active = r
			found = true
		}
	}
	if found {
		return active, true
	}
	return latest, true
}

// PeriodIndexSet returns the set of valid period indices for a regime. It
// centralizes the "which periods are valid" rule shared by course, booking, and
// importer validation, so the three cannot drift to accept different period
// sets when the rule changes.
func PeriodIndexSet(r Regime) map[int]bool {
	set := make(map[int]bool, len(r.Periods))
	for _, p := range r.Periods {
		set[p.PeriodIndex] = true
	}
	return set
}

// trimSeconds drops the ":SS" suffix MySQL returns for TIME columns so callers
// see "HH:MM".
func trimSeconds(s string) string {
	if len(s) >= 8 && s[2] == ':' {
		return s[:5]
	}
	return s
}

func isDuplicateEntry(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
