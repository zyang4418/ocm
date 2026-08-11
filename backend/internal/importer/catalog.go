package importer

import (
	"context"
	"database/sql"
	"fmt"

	"ocm-backend/internal/course"
)

// Column names for the course_catalog import (header-mapped, order-independent).
const (
	ColCatalogName        = "name"
	ColCatalogCode        = "code"
	ColCatalogDescription = "description"
)

// CatalogImporter imports course catalog entries, upserting each row by name
// (the unique key). Re-importing updates code and description.
type CatalogImporter struct {
	db *sql.DB
}

func NewCatalogImporter(db *sql.DB) *CatalogImporter {
	return &CatalogImporter{db: db}
}

func (i *CatalogImporter) Analyze(ctx context.Context, payload string) (Result, error) {
	return analyzeCatalog(ctx, i.db, payload)
}

func (i *CatalogImporter) Commit(ctx context.Context, payload string) (Result, error) {
	return commitCatalog(ctx, i.db, payload)
}

type catalogRow struct {
	course.CatalogInput
	rowNum int
}

func (r catalogRow) toPreviewMap() map[string]any {
	return map[string]any{
		"name":        r.Name,
		"code":        r.Code,
		"description": r.Description,
	}
}

func parseCatalog(payload string) (clean []catalogRow, errs []RowError, dataRows int, err error) {
	headers, rows, headerErr := parseWorkbook(payload)
	if headerErr != nil {
		return nil, []RowError{{Row: 1, Error: headerErr.Error()}}, 1, headerErr
	}
	if rerr, ok := requireColumns(headers, ColCatalogName); !ok {
		return nil, []RowError{rerr}, 1, fmt.Errorf("%s", rerr.Error)
	}

	for i, rec := range rows {
		rowNum := i + 2
		dataRows++
		in := course.CatalogInput{
			Name:        rec[ColCatalogName],
			Code:        rec[ColCatalogCode],
			Description: rec[ColCatalogDescription],
		}
		if msg, ok := course.NormalizeCatalog(&in); !ok {
			errs = append(errs, RowError{Row: rowNum, Error: msg})
			continue
		}
		clean = append(clean, catalogRow{CatalogInput: in, rowNum: rowNum})
	}
	return clean, errs, dataRows, nil
}

func analyzeCatalog(ctx context.Context, db *sql.DB, payload string) (Result, error) {
	clean, errs, dataRows, err := parseCatalog(payload)
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

func commitCatalog(ctx context.Context, db *sql.DB, payload string) (Result, error) {
	clean, errs, dataRows, err := parseCatalog(payload)
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

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO course_catalog (name, code, description)
VALUES (?, ?, ?)
ON DUPLICATE KEY UPDATE code=VALUES(code), description=VALUES(description)`)
	if err != nil {
		res.FailedRows = dataRows
		res.Errors = append(res.Errors, RowError{Row: 0, Error: "导入中断：" + err.Error()})
		return res, fmt.Errorf("prepare upsert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, c := range clean {
		if _, err := stmt.ExecContext(ctx, c.Name, c.Code, c.Description); err != nil {
			res.FailedRows = dataRows
			res.Errors = append(res.Errors, RowError{Row: c.rowNum, Error: "导入中断：" + err.Error()})
			return res, fmt.Errorf("upsert catalog: %w", err)
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
