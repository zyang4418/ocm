package importer

import (
	"context"
	"database/sql"
	"fmt"

	"ocm-backend/internal/user"
)

// Column names for the admin_classes import (header-mapped, order-independent).
const (
	ColAdminGrade = "grade"
	ColAdminName  = "name"
	ColAdminNote  = "note"
)

// AdminClassesImporter imports admin classes, upserting each row by the
// (grade, name) unique key. Re-importing updates the note.
type AdminClassesImporter struct {
	db *sql.DB
}

func NewAdminClassesImporter(db *sql.DB) *AdminClassesImporter {
	return &AdminClassesImporter{db: db}
}

func (i *AdminClassesImporter) Analyze(ctx context.Context, payload string) (Result, error) {
	return analyzeAdminClasses(ctx, i.db, payload)
}

func (i *AdminClassesImporter) Commit(ctx context.Context, payload string) (Result, error) {
	return commitAdminClasses(ctx, i.db, payload)
}

type adminClassRow struct {
	user.AdminClassInput
	rowNum int
}

func (r adminClassRow) toPreviewMap() map[string]any {
	return map[string]any{
		"grade": r.Grade,
		"name":  r.Name,
		"note":  r.Note,
	}
}

func parseAdminClasses(payload string) (clean []adminClassRow, errs []RowError, dataRows int, err error) {
	headers, rows, headerErr := parseWorkbook(payload)
	if headerErr != nil {
		return nil, []RowError{{Row: 1, Error: headerErr.Error()}}, 1, headerErr
	}
	if rerr, ok := requireColumns(headers, ColAdminName); !ok {
		return nil, []RowError{rerr}, 1, fmt.Errorf("%s", rerr.Error)
	}

	for i, rec := range rows {
		rowNum := i + 2
		dataRows++
		in := user.AdminClassInput{
			Grade: rec[ColAdminGrade],
			Name:  rec[ColAdminName],
			Note:  rec[ColAdminNote],
		}
		if msg, ok := user.NormalizeAdminClass(&in); !ok {
			errs = append(errs, RowError{Row: rowNum, Error: msg})
			continue
		}
		clean = append(clean, adminClassRow{AdminClassInput: in, rowNum: rowNum})
	}
	return clean, errs, dataRows, nil
}

func analyzeAdminClasses(ctx context.Context, db *sql.DB, payload string) (Result, error) {
	clean, errs, dataRows, err := parseAdminClasses(payload)
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

func commitAdminClasses(ctx context.Context, db *sql.DB, payload string) (Result, error) {
	clean, errs, dataRows, err := parseAdminClasses(payload)
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

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO admin_classes (grade, name, note)
VALUES (?, ?, ?)
ON DUPLICATE KEY UPDATE note=VALUES(note)`)
	if err != nil {
		res.FailedRows = dataRows
		res.Errors = append(res.Errors, RowError{Row: 0, Error: "导入中断：" + err.Error()})
		return res, fmt.Errorf("prepare upsert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, c := range clean {
		if _, err := stmt.ExecContext(ctx, c.Grade, c.Name, c.Note); err != nil {
			res.FailedRows = dataRows
			res.Errors = append(res.Errors, RowError{Row: c.rowNum, Error: "导入中断：" + err.Error()})
			return res, fmt.Errorf("upsert admin class: %w", err)
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
