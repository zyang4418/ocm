package importer

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"ocm-backend/internal/course"
	"ocm-backend/internal/user"
)

// Column names for the course_offerings import. course / teaching_class /
// semester / note are shared with the sessions import (same header values);
// only teacher is specific to offerings.
const ColTeacher = "teacher"

// OfferingsImporter imports course_offerings (开课) from an xlsx file. It
// resolves the course name to a catalog id and the teaching-class name to a
// teaching-class id, then upserts by the natural key (catalog_id,
// teaching_class_id, semester). Insert-only columns teacher/note are refreshed
// on conflict, matching the replace semantics of the CRUD handlers.
type OfferingsImporter struct {
	db      *sql.DB
	courses *course.Store
	org     *user.Store
}

func NewOfferingsImporter(db *sql.DB, courses *course.Store, org *user.Store) *OfferingsImporter {
	return &OfferingsImporter{db: db, courses: courses, org: org}
}

// loadRefs builds name->id lookup maps for catalog courses and teaching
// classes. The returned error already carries a user-facing Chinese prefix.
func (i *OfferingsImporter) loadRefs(ctx context.Context) (catalog, teaching map[string]int64, err error) {
	list, err := i.courses.ListCatalog(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("加载课程目录失败：%w", err)
	}
	catalog = make(map[string]int64, len(list))
	for _, c := range list {
		catalog[strings.TrimSpace(c.Name)] = c.ID
	}
	tcs, err := i.org.ListTeachingClasses(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("加载教学班失败：%w", err)
	}
	teaching = make(map[string]int64, len(tcs))
	for _, t := range tcs {
		teaching[strings.TrimSpace(t.Name)] = t.ID
	}
	return catalog, teaching, nil
}

func (i *OfferingsImporter) Analyze(ctx context.Context, payload string) (Result, error) {
	catalog, teaching, err := i.loadRefs(ctx)
	if err != nil {
		return Result{Errors: []RowError{{Row: 0, Error: err.Error()}}}, nil
	}
	return analyzeOfferings(catalog, teaching, payload)
}

func (i *OfferingsImporter) Commit(ctx context.Context, payload string) (Result, error) {
	catalog, teaching, err := i.loadRefs(ctx)
	if err != nil {
		return Result{Errors: []RowError{{Row: 0, Error: err.Error()}}}, nil
	}
	return commitOfferings(ctx, i.db, catalog, teaching, payload)
}

// offeringInsert is a fully resolved, validated offering ready to upsert. The
// name fields mirror the resolved IDs purely for preview display.
type offeringInsert struct {
	catalogID         int64
	teachingClassID   int64
	teacher           string
	semester          string
	note              string
	courseName        string
	teachingClassName string
	rowNum            int
}

func (o offeringInsert) toPreviewMap() map[string]any {
	return map[string]any{
		"course":        o.courseName,
		"teachingClass": o.teachingClassName,
		"semester":      o.semester,
		"teacher":       o.teacher,
		"note":          o.note,
	}
}

// naturalKey is the dedup / upsert key for an offering.
func (o offeringInsert) naturalKey() string {
	return fmt.Sprintf("%d|%d|%s", o.catalogID, o.teachingClassID, o.semester)
}

// parseOfferings parses the workbook, resolves names, validates, and dedups
// within the file. No writes.
func parseOfferings(catalog, teaching map[string]int64, payload string) (clean []offeringInsert, errs []RowError, dataRows int, err error) {
	headers, rows, headerErr := parseWorkbook(payload)
	if headerErr != nil {
		return nil, []RowError{{Row: 1, Error: headerErr.Error()}}, 1, headerErr
	}
	if rerr, ok := requireColumns(headers, ColCourse, ColTeachingClass, ColSemester, ColTeacher, ColNote); !ok {
		return nil, []RowError{rerr}, 1, fmt.Errorf("%s", rerr.Error)
	}

	seen := make(map[string]bool)
	for i, rec := range rows {
		rowNum := i + 2
		dataRows++

		courseName := strings.TrimSpace(rec[ColCourse])
		teachingClassName := strings.TrimSpace(rec[ColTeachingClass])
		semester := strings.TrimSpace(rec[ColSemester])
		if courseName == "" || teachingClassName == "" || semester == "" {
			errs = append(errs, RowError{Row: rowNum, Error: "course / teaching_class / semester 为空"})
			continue
		}
		catalogID, ok := catalog[courseName]
		if !ok {
			errs = append(errs, RowError{Row: rowNum, Error: "课程不存在：" + courseName})
			continue
		}
		teachingClassID, ok := teaching[teachingClassName]
		if !ok {
			errs = append(errs, RowError{Row: rowNum, Error: "教学班不存在：" + teachingClassName})
			continue
		}

		in := course.OfferingInput{
			CatalogID:       catalogID,
			TeachingClassID: teachingClassID,
			Teacher:         rec[ColTeacher],
			Semester:        semester,
			Note:            rec[ColNote],
		}
		if msg, ok := course.NormalizeOffering(&in); !ok {
			errs = append(errs, RowError{Row: rowNum, Error: msg})
			continue
		}

		ins := offeringInsert{
			catalogID:         in.CatalogID,
			teachingClassID:   in.TeachingClassID,
			teacher:           in.Teacher,
			semester:          in.Semester,
			note:              in.Note,
			courseName:        courseName,
			teachingClassName: teachingClassName,
			rowNum:            rowNum,
		}
		key := ins.naturalKey()
		if seen[key] {
			errs = append(errs, RowError{Row: rowNum, Error: "本文件内重复开课：相同课程+教学班+学期"})
			continue
		}
		seen[key] = true
		clean = append(clean, ins)
	}
	return clean, errs, dataRows, nil
}

func analyzeOfferings(catalog, teaching map[string]int64, payload string) (Result, error) {
	clean, errs, dataRows, err := parseOfferings(catalog, teaching, payload)
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

func commitOfferings(ctx context.Context, db *sql.DB, catalog, teaching map[string]int64, payload string) (Result, error) {
	clean, errs, dataRows, err := parseOfferings(catalog, teaching, payload)
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

	// Upsert by (catalog_id, teaching_class_id, semester). The natural-key
	// columns are part of the unique index and not updated; teacher/note are the
	// mutable payload columns refreshed on conflict.
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO course_offerings (catalog_id, teaching_class_id, teacher, semester, note)
VALUES (?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE teacher = VALUES(teacher), note = VALUES(note)`)
	if err != nil {
		res.FailedRows = dataRows
		res.Errors = append(res.Errors, RowError{Row: 0, Error: "导入中断：" + err.Error()})
		return res, fmt.Errorf("prepare offering upsert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, o := range clean {
		if _, err := stmt.ExecContext(ctx, o.catalogID, o.teachingClassID, o.teacher, o.semester, o.note); err != nil {
			res.FailedRows = dataRows
			res.Errors = append(res.Errors, RowError{Row: o.rowNum, Error: "导入中断：" + err.Error()})
			return res, fmt.Errorf("upsert offering: %w", err)
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
