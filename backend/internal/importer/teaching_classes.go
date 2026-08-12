package importer

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"ocm-backend/internal/user"
)

// Column names for the teaching_classes import (header-mapped, order-independent).
// The file is flattened: one row per member admin class, with the teaching
// class columns repeated. The importer groups by name and replaces the member set.
const (
	ColTcName       = "name"
	ColTcNote       = "note"
	ColTcAdminGrade = "admin_grade"
	ColTcAdminName  = "admin_name"
)

// TeachingClassesImporter imports teaching classes and their member admin
// classes. Each row names a teaching class plus one member admin class
// (grade + name); rows sharing a teaching-class name form one group. The group
// is upserted by name and its member set replaced -- unless the teaching class
// is already referenced by a course offering AND the new member set differs
// from the current one, in which case the group is skipped (members are frozen
// for in-use classes, matching the update handler's rule).
type TeachingClassesImporter struct {
	db *sql.DB
}

func NewTeachingClassesImporter(db *sql.DB) *TeachingClassesImporter {
	return &TeachingClassesImporter{db: db}
}

func (i *TeachingClassesImporter) Analyze(ctx context.Context, payload string) (Result, error) {
	return analyzeTeachingClasses(ctx, i.db, payload)
}

func (i *TeachingClassesImporter) Commit(ctx context.Context, payload string) (Result, error) {
	return commitTeachingClasses(ctx, i.db, payload)
}

type teachingGroup struct {
	name    string
	note    string
	members []memberRef // deduped, in first-seen order
	rowNum  int         // first row of the group, for error reporting
}

type memberRef struct {
	grade  string
	name   string
	rowNum int
}

type teachingClassInsert struct {
	name      string
	note      string
	memberIDs []int64
	exists    bool  // teaching class already exists (update vs insert)
	id        int64 // existing id when exists
	rowNum    int
	labels    []string
}

func (g teachingClassInsert) toPreviewMap() map[string]any {
	return map[string]any{
		"name":          g.name,
		"note":          g.note,
		"admin_classes": g.labels,
	}
}

// parseTeachingClasses parses the workbook, groups rows by teaching-class name,
// resolves member admin classes to IDs, and enforces the in-use member freeze.
// It performs no writes. A system error (a reference query fails) is returned as
// err and aborts the parse; unresolvable members or frozen groups are per-group
// errors that do not block the rest.
func parseTeachingClasses(ctx context.Context, db *sql.DB, payload string) (clean []teachingClassInsert, errs []RowError, dataRows int, err error) {
	headers, rows, headerErr := parseWorkbook(payload)
	if headerErr != nil {
		return nil, []RowError{{Row: 1, Error: headerErr.Error()}}, 1, headerErr
	}
	if rerr, ok := requireColumns(headers, ColTcName, ColTcAdminGrade, ColTcAdminName); !ok {
		return nil, []RowError{rerr}, 1, fmt.Errorf("%s", rerr.Error)
	}

	// Group rows by teaching-class name, preserving first-seen order.
	groups := map[string]*teachingGroup{}
	var order []string
	for i, rec := range rows {
		rowNum := i + 2
		dataRows++
		name := rec[ColTcName]
		if name == "" {
			errs = append(errs, RowError{Row: rowNum, Error: "name 为空"})
			continue
		}
		grade := rec[ColTcAdminGrade]
		mname := rec[ColTcAdminName]
		if grade == "" || mname == "" {
			errs = append(errs, RowError{Row: rowNum, Error: "admin_grade / admin_name 为空"})
			continue
		}
		g, ok := groups[name]
		if !ok {
			g = &teachingGroup{name: name, note: rec[ColTcNote], rowNum: rowNum}
			groups[name] = g
			order = append(order, name)
		} else if g.note == "" {
			g.note = rec[ColTcNote] // fill note from a later row if the first was blank
		}
		g.members = append(g.members, memberRef{grade: grade, name: mname, rowNum: rowNum})
	}

	if len(order) == 0 {
		return clean, errs, dataRows, nil
	}

	// Load reference data in three queries: admin classes (grade|name -> id),
	// existing teaching classes (name -> id), their current members (id -> ids),
	// and which teaching classes are in use (referenced by course_offerings).
	acMap, err := loadAdminClassMap(ctx, db)
	if err != nil {
		errs = append(errs, RowError{Row: 0, Error: "导入中断：" + err.Error()})
		return nil, errs, dataRows, fmt.Errorf("load admin classes: %w", err)
	}
	tcByName, membersByTC, inUse, err := loadTeachingClassState(ctx, db)
	if err != nil {
		errs = append(errs, RowError{Row: 0, Error: "导入中断：" + err.Error()})
		return nil, errs, dataRows, fmt.Errorf("load teaching class state: %w", err)
	}

	for _, name := range order {
		g := groups[name]
		// Dedup members within the group and resolve to IDs.
		seen := map[string]bool{}
		var ids []int64
		var labels []string
		failed := false
		for _, m := range g.members {
			key := m.grade + "|" + m.name
			if seen[key] {
				continue
			}
			seen[key] = true
			id, ok := acMap[key]
			if !ok {
				errs = append(errs, RowError{Row: m.rowNum, Error: "行政班不存在：" + m.grade + "/" + m.name})
				failed = true
				continue
			}
			ids = append(ids, id)
			labels = append(labels, m.grade+"/"+m.name)
		}
		if failed {
			continue // drop the whole group: its member set is incomplete
		}
		if len(ids) == 0 {
			errs = append(errs, RowError{Row: g.rowNum, Error: "教学班至少需要一个行政班"})
			continue
		}
		ins := teachingClassInsert{
			name: g.name, note: g.note, memberIDs: sortedInt64s(ids),
			labels: labels, rowNum: g.rowNum,
		}
		if tcID, ok := tcByName[g.name]; ok {
			ins.exists = true
			ins.id = tcID
			// In-use classes whose member set would change are frozen: skip the
			// group so the operator edits members through the UI (or creates a
			// new teaching class), matching the update handler's ErrClassInUse.
			if inUse[tcID] && !sameInt64Set(membersByTC[tcID], ins.memberIDs) {
				errs = append(errs, RowError{Row: g.rowNum, Error: "教学班「" + g.name + "」已被开课引用，成员不可修改，请新建教学班"})
				continue
			}
		}
		clean = append(clean, ins)
	}
	return clean, errs, dataRows, nil
}

func analyzeTeachingClasses(ctx context.Context, db *sql.DB, payload string) (Result, error) {
	clean, errs, dataRows, err := parseTeachingClasses(ctx, db, payload)
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

func commitTeachingClasses(ctx context.Context, db *sql.DB, payload string) (Result, error) {
	clean, errs, dataRows, err := parseTeachingClasses(ctx, db, payload)
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

	// Reuse user.NormalizeTeachingClass for the name/required-member rule. The
	// in-use freeze was already enforced in parse; here we just write.
	for _, c := range clean {
		in := user.TeachingClassInput{Name: c.name, Note: c.note, ClassIDs: c.memberIDs}
		if msg, ok := user.NormalizeTeachingClass(&in); !ok {
			errs = append(errs, RowError{Row: c.rowNum, Error: msg})
			continue
		}
		if c.exists {
			if _, err := tx.ExecContext(ctx,
				`UPDATE teaching_classes SET name = ?, note = ? WHERE id = ?`,
				in.Name, in.Note, c.id); err != nil {
				res.FailedRows = dataRows
				res.Errors = append(res.Errors, RowError{Row: c.rowNum, Error: "导入中断：" + err.Error()})
				return res, fmt.Errorf("update teaching class: %w", err)
			}
			if _, err := tx.ExecContext(ctx,
				`DELETE FROM teaching_class_members WHERE teaching_class_id = ?`, c.id); err != nil {
				res.FailedRows = dataRows
				res.Errors = append(res.Errors, RowError{Row: c.rowNum, Error: "导入中断：" + err.Error()})
				return res, fmt.Errorf("clear members: %w", err)
			}
			if err := insertMembersTx(ctx, tx, c.id, in.ClassIDs); err != nil {
				res.FailedRows = dataRows
				res.Errors = append(res.Errors, RowError{Row: c.rowNum, Error: "导入中断：" + err.Error()})
				return res, fmt.Errorf("insert members: %w", err)
			}
		} else {
			res2, err := tx.ExecContext(ctx,
				`INSERT INTO teaching_classes (name, note) VALUES (?, ?)`, in.Name, in.Note)
			if err != nil {
				if isDuplicateEntry(err) {
					// A teaching class of this name was created between preview
					// and commit; treat as a conflict and skip this group.
					errs = append(errs, RowError{Row: c.rowNum, Error: "教学班「" + in.Name + "」已存在（并发）"})
					continue
				}
				res.FailedRows = dataRows
				res.Errors = append(res.Errors, RowError{Row: c.rowNum, Error: "导入中断：" + err.Error()})
				return res, fmt.Errorf("insert teaching class: %w", err)
			}
			id, err := res2.LastInsertId()
			if err != nil {
				res.FailedRows = dataRows
				res.Errors = append(res.Errors, RowError{Row: c.rowNum, Error: "导入中断：" + err.Error()})
				return res, fmt.Errorf("teaching class last insert id: %w", err)
			}
			if err := insertMembersTx(ctx, tx, id, in.ClassIDs); err != nil {
				res.FailedRows = dataRows
				res.Errors = append(res.Errors, RowError{Row: c.rowNum, Error: "导入中断：" + err.Error()})
				return res, fmt.Errorf("insert members: %w", err)
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

// insertMembersTx inserts teaching_class_members rows within tx.
func insertMembersTx(ctx context.Context, tx *sql.Tx, teachingClassID int64, adminClassIDs []int64) error {
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO teaching_class_members (teaching_class_id, admin_class_id) VALUES (?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare member insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()
	for _, acID := range adminClassIDs {
		if _, err := stmt.ExecContext(ctx, teachingClassID, acID); err != nil {
			return fmt.Errorf("insert member: %w", err)
		}
	}
	return nil
}

// loadAdminClassMap returns a map from "grade|name" to admin_class id.
func loadAdminClassMap(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, grade, name FROM admin_classes`)
	if err != nil {
		return nil, fmt.Errorf("query admin classes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int64{}
	for rows.Next() {
		var id int64
		var grade, name string
		if err := rows.Scan(&id, &grade, &name); err != nil {
			return nil, fmt.Errorf("scan admin class: %w", err)
		}
		out[strings.TrimSpace(grade)+"|"+strings.TrimSpace(name)] = id
	}
	return out, rows.Err()
}

// loadTeachingClassState returns: name -> id, id -> current member admin_class
// ids (sorted), and the set of in-use teaching-class ids (referenced by
// course_offerings).
func loadTeachingClassState(ctx context.Context, db *sql.DB) (nameByID map[string]int64, members map[int64][]int64, inUse map[int64]bool, err error) {
	nameByID = map[string]int64{}
	members = map[int64][]int64{}
	inUse = map[int64]bool{}

	rows, err := db.QueryContext(ctx, `SELECT id, name FROM teaching_classes`)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("query teaching classes: %w", err)
	}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			_ = rows.Close()
			return nil, nil, nil, fmt.Errorf("scan teaching class: %w", err)
		}
		nameByID[strings.TrimSpace(name)] = id
	}
	_ = rows.Close()

	rows, err = db.QueryContext(ctx, `SELECT teaching_class_id, admin_class_id FROM teaching_class_members`)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("query members: %w", err)
	}
	for rows.Next() {
		var tcID, acID int64
		if err := rows.Scan(&tcID, &acID); err != nil {
			_ = rows.Close()
			return nil, nil, nil, fmt.Errorf("scan member: %w", err)
		}
		members[tcID] = append(members[tcID], acID)
	}
	_ = rows.Close()
	for k := range members {
		members[k] = sortedInt64s(members[k])
	}

	rows, err = db.QueryContext(ctx, `SELECT DISTINCT teaching_class_id FROM course_offerings`)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("query offerings: %w", err)
	}
	for rows.Next() {
		var tcID int64
		if err := rows.Scan(&tcID); err != nil {
			_ = rows.Close()
			return nil, nil, nil, fmt.Errorf("scan offering: %w", err)
		}
		inUse[tcID] = true
	}
	_ = rows.Close()
	return nameByID, members, inUse, nil
}

func sortedInt64s(ids []int64) []int64 {
	out := append([]int64(nil), ids...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sameInt64Set(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
