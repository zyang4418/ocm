package importer

import (
	"context"
	"database/sql"
	"fmt"

	"ocm-backend/internal/classroom"
)

// Column names for the classrooms import (header-mapped, order-independent).
const (
	ColClassroomName        = "name"
	ColClassroomBuilding    = "building"
	ColClassroomCapacity    = "capacity"
	ColClassroomType        = "type"
	ColClassroomStatus      = "status"
	ColClassroomDescription = "description"
)

// ClassroomsImporter imports classrooms, upserting each row by name (the unique
// key). Re-importing an existing classroom updates its mutable fields rather
// than erroring.
type ClassroomsImporter struct {
	db *sql.DB
}

func NewClassroomsImporter(db *sql.DB) *ClassroomsImporter {
	return &ClassroomsImporter{db: db}
}

func (i *ClassroomsImporter) Analyze(ctx context.Context, payload string) (Result, error) {
	return analyzeClassrooms(ctx, i.db, payload)
}

func (i *ClassroomsImporter) Commit(ctx context.Context, payload string) (Result, error) {
	return commitClassrooms(ctx, i.db, payload)
}

type classroomRow struct {
	classroom.ClassroomInput
	rowNum int
}

func (r classroomRow) toPreviewMap() map[string]any {
	return map[string]any{
		"name":        r.Name,
		"building":    r.Building,
		"capacity":    r.Capacity,
		"type":        r.Type,
		"status":      r.Status,
		"description": r.Description,
	}
}

func parseClassrooms(payload string) (clean []classroomRow, errs []RowError, dataRows int, err error) {
	headers, rows, headerErr := parseWorkbook(payload)
	if headerErr != nil {
		return nil, []RowError{{Row: 1, Error: headerErr.Error()}}, 1, headerErr
	}
	if rerr, ok := requireColumns(headers, ColClassroomName); !ok {
		return nil, []RowError{rerr}, 1, fmt.Errorf("%s", rerr.Error)
	}

	for i, rec := range rows {
		rowNum := i + 2
		dataRows++
		in := classroom.ClassroomInput{
			Name:        rec[ColClassroomName],
			Building:    rec[ColClassroomBuilding],
			Capacity:    atoiOr(rec[ColClassroomCapacity], 0),
			Type:        rec[ColClassroomType],
			Status:      rec[ColClassroomStatus],
			Description: rec[ColClassroomDescription],
		}
		if msg, ok := classroom.NormalizeInput(&in); !ok {
			errs = append(errs, RowError{Row: rowNum, Error: msg})
			continue
		}
		clean = append(clean, classroomRow{ClassroomInput: in, rowNum: rowNum})
	}
	return clean, errs, dataRows, nil
}

func analyzeClassrooms(ctx context.Context, db *sql.DB, payload string) (Result, error) {
	clean, errs, dataRows, err := parseClassrooms(payload)
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

func commitClassrooms(ctx context.Context, db *sql.DB, payload string) (Result, error) {
	clean, errs, dataRows, err := parseClassrooms(payload)
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

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO classrooms (name, building, capacity, type, status, description)
VALUES (?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE building=VALUES(building), capacity=VALUES(capacity), type=VALUES(type), status=VALUES(status), description=VALUES(description)`)
	if err != nil {
		res.FailedRows = dataRows
		res.Errors = append(res.Errors, RowError{Row: 0, Error: "导入中断：" + err.Error()})
		return res, fmt.Errorf("prepare upsert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, c := range clean {
		if _, err := stmt.ExecContext(ctx, c.Name, c.Building, c.Capacity, c.Type, c.Status, c.Description); err != nil {
			res.FailedRows = dataRows
			res.Errors = append(res.Errors, RowError{Row: c.rowNum, Error: "导入中断：" + err.Error()})
			return res, fmt.Errorf("upsert classroom: %w", err)
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
