package importer

import (
	"context"
	"database/sql"
	"fmt"

	"ocm-backend/internal/schedule"
)

// Column names for the schedule_regimes import (header-mapped, order-independent).
// Flattened: one row per period; rows sharing regime_name form one regime whose
// period set is replaced on commit.
const (
	ColRegimeName        = "regime_name"
	ColEffectiveMonth    = "effective_month"
	ColEffectiveDay      = "effective_day"
	ColRegimePeriodIndex = "period_index"
	ColStartTime         = "start_time"
	ColEndTime           = "end_time"
)

// RegimesImporter imports bell-time regimes and their periods. Each row names a
// regime plus one period; rows are grouped by regime_name. The regime is
// upserted by name and its periods fully replaced (delete + insert), matching
// the replace-periods semantics of the CRUD handler.
type RegimesImporter struct {
	db *sql.DB
}

func NewRegimesImporter(db *sql.DB) *RegimesImporter {
	return &RegimesImporter{db: db}
}

func (i *RegimesImporter) Analyze(ctx context.Context, payload string) (Result, error) {
	return analyzeRegimes(ctx, i.db, payload)
}

func (i *RegimesImporter) Commit(ctx context.Context, payload string) (Result, error) {
	return commitRegimes(ctx, i.db, payload)
}

type regimeInsert struct {
	name    string
	month   int
	day     int
	periods []schedule.PeriodInput
	rowNum  int
}

func (r regimeInsert) toPreviewMap() map[string]any {
	periods := make([]map[string]any, 0, len(r.periods))
	for _, p := range r.periods {
		periods = append(periods, map[string]any{
			"periodIndex": p.PeriodIndex,
			"startTime":   p.StartTime,
			"endTime":     p.EndTime,
		})
	}
	return map[string]any{
		"name":           r.name,
		"effectiveMonth": r.month,
		"effectiveDay":   r.day,
		"periods":        periods,
	}
}

// parseRegimes parses the workbook, groups rows by regime_name, and validates
// each regime and its period set using the schedule package's normalizers. No
// writes. A group whose month/day disagree across its rows is rejected (the
// regime fields must be consistent within a group).
func parseRegimes(payload string) (clean []regimeInsert, errs []RowError, dataRows int, err error) {
	headers, rows, headerErr := parseWorkbook(payload)
	if headerErr != nil {
		return nil, []RowError{{Row: 1, Error: headerErr.Error()}}, 1, headerErr
	}
	if rerr, ok := requireColumns(headers,
		ColRegimeName, ColEffectiveMonth, ColEffectiveDay, ColRegimePeriodIndex, ColStartTime, ColEndTime); !ok {
		return nil, []RowError{rerr}, 1, fmt.Errorf("%s", rerr.Error)
	}

	type group struct {
		name       string
		month, day int
		haveMonth  bool
		periods    []schedule.PeriodInput
		rowNum     int
	}
	groups := map[string]*group{}
	var order []string

	for i, rec := range rows {
		rowNum := i + 2
		dataRows++
		name := rec[ColRegimeName]
		if name == "" {
			errs = append(errs, RowError{Row: rowNum, Error: "regime_name 为空"})
			continue
		}
		month := atoiOr(rec[ColEffectiveMonth], 0)
		day := atoiOr(rec[ColEffectiveDay], 0)
		idx := atoiOr(rec[ColRegimePeriodIndex], 0)
		start := rec[ColStartTime]
		end := rec[ColEndTime]

		g, ok := groups[name]
		if !ok {
			g = &group{name: name, month: month, day: day, haveMonth: true, rowNum: rowNum}
			groups[name] = g
			order = append(order, name)
		} else if g.haveMonth && (month != g.month || day != g.day) {
			// Inconsistent regime fields within the same group.
			errs = append(errs, RowError{Row: rowNum, Error: "同一作息制度「" + name + "」的 effective_month/effective_day 不一致"})
			continue
		}
		g.periods = append(g.periods, schedule.PeriodInput{
			PeriodIndex: idx, StartTime: start, EndTime: end,
		})
	}

	for _, name := range order {
		g := groups[name]
		ri := regimeInsert{name: g.name, month: g.month, day: g.day, periods: g.periods, rowNum: g.rowNum}
		if msg, ok := schedule.NormalizeRegime(&schedule.RegimeInput{
			Name: ri.name, EffectiveMonth: ri.month, EffectiveDay: ri.day,
		}); !ok {
			errs = append(errs, RowError{Row: g.rowNum, Error: msg})
			continue
		}
		if msg, ok := schedule.NormalizePeriods(ri.periods); !ok {
			errs = append(errs, RowError{Row: g.rowNum, Error: msg})
			continue
		}
		clean = append(clean, ri)
	}
	return clean, errs, dataRows, nil
}

func analyzeRegimes(ctx context.Context, db *sql.DB, payload string) (Result, error) {
	clean, errs, dataRows, err := parseRegimes(payload)
	if err != nil {
		return Result{TotalRows: dataRows, FailedRows: dataRows, Errors: errs}, err
	}
	rows := make([]map[string]any, 0, len(clean))
	for _, c := range clean {
		rows = append(rows, c.toPreviewMap())
	}
	return Result{
		TotalRows:     dataRows,
		SucceededRows: len(clean),
		FailedRows:    dataRows - len(clean),
		Errors:        errs,
		Rows:          rows,
	}, nil
}

func commitRegimes(ctx context.Context, db *sql.DB, payload string) (Result, error) {
	clean, errs, dataRows, err := parseRegimes(payload)
	if err != nil {
		return Result{TotalRows: dataRows, FailedRows: dataRows, Errors: errs}, err
	}

	res := Result{TotalRows: dataRows, Errors: errs}
	if len(clean) == 0 {
		res.FailedRows = dataRows
		return res, nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		res.FailedRows = dataRows
		res.Errors = append(res.Errors, RowError{Row: 0, Error: "导入中断：" + err.Error()})
		return res, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Upsert the regime and resolve its id in one statement: ON DUPLICATE KEY
	// UPDATE id=LAST_INSERT_ID(id) makes LastInsertId return the existing id on
	// update, so the subsequent period replacement targets the right regime.
	upsertStmt, err := tx.PrepareContext(ctx, `INSERT INTO schedule_regimes (name, effective_month, effective_day)
VALUES (?, ?, ?)
ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id), effective_month=VALUES(effective_month), effective_day=VALUES(effective_day)`)
	if err != nil {
		res.FailedRows = dataRows
		res.Errors = append(res.Errors, RowError{Row: 0, Error: "导入中断：" + err.Error()})
		return res, fmt.Errorf("prepare regime upsert: %w", err)
	}
	defer func() { _ = upsertStmt.Close() }()

	delStmt, err := tx.PrepareContext(ctx, `DELETE FROM schedule_periods WHERE regime_id = ?`)
	if err != nil {
		res.FailedRows = dataRows
		res.Errors = append(res.Errors, RowError{Row: 0, Error: "导入中断：" + err.Error()})
		return res, fmt.Errorf("prepare period delete: %w", err)
	}
	defer func() { _ = delStmt.Close() }()

	insStmt, err := tx.PrepareContext(ctx, `INSERT INTO schedule_periods (regime_id, period_index, start_time, end_time) VALUES (?, ?, ?, ?)`)
	if err != nil {
		res.FailedRows = dataRows
		res.Errors = append(res.Errors, RowError{Row: 0, Error: "导入中断：" + err.Error()})
		return res, fmt.Errorf("prepare period insert: %w", err)
	}
	defer func() { _ = insStmt.Close() }()

	for _, r := range clean {
		r2, err := upsertStmt.ExecContext(ctx, r.name, r.month, r.day)
		if err != nil {
			res.FailedRows = dataRows
			res.Errors = append(res.Errors, RowError{Row: r.rowNum, Error: "导入中断：" + err.Error()})
			return res, fmt.Errorf("upsert regime: %w", err)
		}
		regimeID, err := r2.LastInsertId()
		if err != nil {
			res.FailedRows = dataRows
			res.Errors = append(res.Errors, RowError{Row: r.rowNum, Error: "导入中断：" + err.Error()})
			return res, fmt.Errorf("regime last insert id: %w", err)
		}
		if _, err := delStmt.ExecContext(ctx, regimeID); err != nil {
			res.FailedRows = dataRows
			res.Errors = append(res.Errors, RowError{Row: r.rowNum, Error: "导入中断：" + err.Error()})
			return res, fmt.Errorf("delete periods: %w", err)
		}
		for _, p := range r.periods {
			if _, err := insStmt.ExecContext(ctx, regimeID, p.PeriodIndex, p.StartTime, p.EndTime); err != nil {
				res.FailedRows = dataRows
				res.Errors = append(res.Errors, RowError{Row: r.rowNum, Error: "导入中断：" + err.Error()})
				return res, fmt.Errorf("insert period: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		res.FailedRows = dataRows
		res.Errors = append(res.Errors, RowError{Row: 0, Error: "导入中断：" + err.Error()})
		return res, fmt.Errorf("commit: %w", err)
	}
	res.SucceededRows = len(clean)
	res.FailedRows = dataRows - res.SucceededRows
	return res, nil
}
