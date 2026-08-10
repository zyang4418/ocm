package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/go-sql-driver/mysql"
)

var (
	ErrAdminClassNotFound  = errors.New("admin class not found")
	ErrTeachingClassNotFound = errors.New("teaching class not found")
	ErrClassNameTaken      = errors.New("class name already taken")
	ErrClassInUse          = errors.New("class is in use and cannot be modified or deleted")
	ErrMemberRequired      = errors.New("teaching class must have at least one admin class")
)

// Migrate creates the admin_classes, teaching_classes and
// teaching_class_members tables. It is idempotent and safe to run on every
// startup. Admin/teaching classes belong to the user/people module; the course
// module references teaching_class_id as a logical foreign key.
func (s *Store) Migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS admin_classes (
    id         BIGINT AUTO_INCREMENT PRIMARY KEY,
    grade      VARCHAR(64)  NOT NULL DEFAULT '',
    name       VARCHAR(64)  NOT NULL,
    note       VARCHAR(255) NOT NULL DEFAULT '',
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (grade, name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS teaching_classes (
    id         BIGINT AUTO_INCREMENT PRIMARY KEY,
    name       VARCHAR(64)  NOT NULL UNIQUE,
    note       VARCHAR(255) NOT NULL DEFAULT '',
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS teaching_class_members (
    teaching_class_id BIGINT    NOT NULL,
    admin_class_id    BIGINT    NOT NULL,
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (teaching_class_id, admin_class_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, q := range stmts {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("create org table: %w", err)
		}
	}
	return nil
}

// isDuplicateEntry reports whether err is a MySQL 1062 unique-constraint
// violation, used to detect duplicate class names on insert and update.
func isDuplicateEntry(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

// ---- Admin classes ----

func (s *Store) ListAdminClasses(ctx context.Context) ([]AdminClass, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, grade, name, note, created_at FROM admin_classes ORDER BY grade, name`)
	if err != nil {
		return nil, fmt.Errorf("list admin classes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []AdminClass
	for rows.Next() {
		var c AdminClass
		if err := rows.Scan(&c.ID, &c.Grade, &c.Name, &c.Note, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan admin class: %w", err)
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *Store) GetAdminClass(ctx context.Context, id int64) (AdminClass, error) {
	var c AdminClass
	err := s.db.QueryRowContext(ctx,
		`SELECT id, grade, name, note, created_at FROM admin_classes WHERE id = ?`, id,
	).Scan(&c.ID, &c.Grade, &c.Name, &c.Note, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AdminClass{}, ErrAdminClassNotFound
	}
	if err != nil {
		return AdminClass{}, fmt.Errorf("get admin class: %w", err)
	}
	return c, nil
}

func (s *Store) CreateAdminClass(ctx context.Context, in AdminClassInput) (AdminClass, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO admin_classes (grade, name, note) VALUES (?, ?, ?)`,
		in.Grade, in.Name, in.Note,
	)
	if err != nil {
		if isDuplicateEntry(err) {
			return AdminClass{}, ErrClassNameTaken
		}
		return AdminClass{}, fmt.Errorf("create admin class: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return AdminClass{}, fmt.Errorf("create admin class last insert id: %w", err)
	}
	return s.GetAdminClass(ctx, id)
}

func (s *Store) UpdateAdminClass(ctx context.Context, id int64, in AdminClassInput) (AdminClass, error) {
	_, err := s.db.ExecContext(ctx,
		`UPDATE admin_classes SET grade = ?, name = ?, note = ? WHERE id = ?`,
		in.Grade, in.Name, in.Note, id,
	)
	if err != nil {
		if isDuplicateEntry(err) {
			return AdminClass{}, ErrClassNameTaken
		}
		return AdminClass{}, fmt.Errorf("update admin class: %w", err)
	}
	c, err := s.GetAdminClass(ctx, id)
	if errors.Is(err, ErrAdminClassNotFound) {
		return AdminClass{}, err
	}
	return c, err
}

func (s *Store) DeleteAdminClass(ctx context.Context, id int64) error {
	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM teaching_class_members WHERE admin_class_id = ?`, id,
	).Scan(&count); err != nil {
		return fmt.Errorf("count teaching class members: %w", err)
	}
	if count > 0 {
		return ErrClassInUse
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM admin_classes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete admin class: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete admin class rows affected: %w", err)
	}
	if n == 0 {
		return ErrAdminClassNotFound
	}
	return nil
}

// ---- Teaching classes ----

// classMembersByTeachingClass loads member ClassRefs for the given teaching
// class IDs in one query, returning a map keyed by teaching_class_id.
func (s *Store) classMembersByTeachingClass(ctx context.Context, ids []int64) (map[int64][]ClassRef, error) {
	out := make(map[int64][]ClassRef)
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	q := fmt.Sprintf(
		`SELECT m.teaching_class_id, ac.id, ac.grade, ac.name
		 FROM teaching_class_members m
		 JOIN admin_classes ac ON ac.id = m.admin_class_id
		 WHERE m.teaching_class_id IN (%s)
		 ORDER BY m.teaching_class_id, ac.grade, ac.name`,
		joinPlaceholders(placeholders),
	)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query teaching class members: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var tcID, acID int64
		var grade, name string
		if err := rows.Scan(&tcID, &acID, &grade, &name); err != nil {
			return nil, fmt.Errorf("scan teaching class member: %w", err)
		}
		out[tcID] = append(out[tcID], ClassRef{ID: acID, Grade: grade, Name: name})
	}
	return out, rows.Err()
}

func joinPlaceholders(p []string) string {
	out := ""
	for i, s := range p {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}

func (s *Store) ListTeachingClasses(ctx context.Context) ([]TeachingClassView, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, note, created_at FROM teaching_classes ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list teaching classes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []TeachingClassView
	var ids []int64
	for rows.Next() {
		var v TeachingClassView
		if err := rows.Scan(&v.ID, &v.Name, &v.Note, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan teaching class: %w", err)
		}
		list = append(list, v)
		ids = append(ids, v.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	members, err := s.classMembersByTeachingClass(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Classes = members[list[i].ID]
		if list[i].Classes == nil {
			list[i].Classes = []ClassRef{}
		}
	}
	return list, nil
}

func (s *Store) GetTeachingClass(ctx context.Context, id int64) (TeachingClassView, error) {
	var v TeachingClassView
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, note, created_at FROM teaching_classes WHERE id = ?`, id,
	).Scan(&v.ID, &v.Name, &v.Note, &v.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return TeachingClassView{}, ErrTeachingClassNotFound
	}
	if err != nil {
		return TeachingClassView{}, fmt.Errorf("get teaching class: %w", err)
	}
	members, err := s.classMembersByTeachingClass(ctx, []int64{id})
	if err != nil {
		return TeachingClassView{}, err
	}
	v.Classes = members[id]
	if v.Classes == nil {
		v.Classes = []ClassRef{}
	}
	return v, nil
}

// teachingClassMemberIDs returns the current member admin_class IDs of a
// teaching class, sorted.
func (s *Store) teachingClassMemberIDs(ctx context.Context, id int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT admin_class_id FROM teaching_class_members WHERE teaching_class_id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("query teaching class members: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var acID int64
		if err := rows.Scan(&acID); err != nil {
			return nil, fmt.Errorf("scan member id: %w", err)
		}
		ids = append(ids, acID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, rows.Err()
}

// teachingClassInUse reports whether any course offering references the
// teaching class. Course offerings live in the course module; this is a
// logical cross-module FK check (no Go dependency on the course package).
func (s *Store) teachingClassInUse(ctx context.Context, id int64) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM course_offerings WHERE teaching_class_id = ?`, id,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("count offerings for teaching class: %w", err)
	}
	return count > 0, nil
}

// adminClassesExist returns ErrAdminClassNotFound if any of ids is absent from
// admin_classes.
func (s *Store) adminClassesExist(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	q := fmt.Sprintf(
		`SELECT COUNT(*) FROM admin_classes WHERE id IN (%s)`,
		joinPlaceholders(placeholders),
	)
	var count int
	if err := s.db.QueryRowContext(ctx, q, args...).Scan(&count); err != nil {
		return fmt.Errorf("count admin classes: %w", err)
	}
	if count != len(ids) {
		return ErrAdminClassNotFound
	}
	return nil
}

func (s *Store) CreateTeachingClass(ctx context.Context, in TeachingClassInput) (TeachingClassView, error) {
	if len(in.ClassIDs) == 0 {
		return TeachingClassView{}, ErrMemberRequired
	}
	if err := s.adminClassesExist(ctx, in.ClassIDs); err != nil {
		return TeachingClassView{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TeachingClassView{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO teaching_classes (name, note) VALUES (?, ?)`, in.Name, in.Note)
	if err != nil {
		if isDuplicateEntry(err) {
			return TeachingClassView{}, ErrClassNameTaken
		}
		return TeachingClassView{}, fmt.Errorf("create teaching class: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return TeachingClassView{}, fmt.Errorf("create teaching class last insert id: %w", err)
	}
	if err := insertMembers(ctx, tx, id, in.ClassIDs); err != nil {
		return TeachingClassView{}, err
	}
	if err := tx.Commit(); err != nil {
		return TeachingClassView{}, fmt.Errorf("commit teaching class: %w", err)
	}
	return s.GetTeachingClass(ctx, id)
}

func (s *Store) UpdateTeachingClass(ctx context.Context, id int64, in TeachingClassInput) (TeachingClassView, error) {
	if len(in.ClassIDs) == 0 {
		return TeachingClassView{}, ErrMemberRequired
	}
	if err := s.adminClassesExist(ctx, in.ClassIDs); err != nil {
		return TeachingClassView{}, err
	}

	// Hardening (decision 2): once a teaching class is referenced by any
	// offering, its member set is frozen -- editing members would rewrite the
	// class list shown on past offerings. To change membership, create a new
	// teaching class. Name/note remain editable.
	current, err := s.teachingClassMemberIDs(ctx, id)
	if err != nil {
		return TeachingClassView{}, err
	}
	newMembers := append([]int64(nil), in.ClassIDs...)
	sort.Slice(newMembers, func(i, j int) bool { return newMembers[i] < newMembers[j] })
	inUse, err := s.teachingClassInUse(ctx, id)
	if err != nil {
		return TeachingClassView{}, err
	}
	if inUse && !sameInt64Set(current, newMembers) {
		return TeachingClassView{}, ErrClassInUse
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TeachingClassView{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`UPDATE teaching_classes SET name = ?, note = ? WHERE id = ?`, in.Name, in.Note, id)
	if err != nil {
		if isDuplicateEntry(err) {
			return TeachingClassView{}, ErrClassNameTaken
		}
		return TeachingClassView{}, fmt.Errorf("update teaching class: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return TeachingClassView{}, fmt.Errorf("update teaching class rows affected: %w", err)
	}
	if n == 0 {
		return TeachingClassView{}, ErrTeachingClassNotFound
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM teaching_class_members WHERE teaching_class_id = ?`, id); err != nil {
		return TeachingClassView{}, fmt.Errorf("clear members: %w", err)
	}
	if err := insertMembers(ctx, tx, id, newMembers); err != nil {
		return TeachingClassView{}, err
	}
	if err := tx.Commit(); err != nil {
		return TeachingClassView{}, fmt.Errorf("commit teaching class: %w", err)
	}
	return s.GetTeachingClass(ctx, id)
}

func (s *Store) DeleteTeachingClass(ctx context.Context, id int64) error {
	inUse, err := s.teachingClassInUse(ctx, id)
	if err != nil {
		return err
	}
	if inUse {
		return ErrClassInUse
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM teaching_class_members WHERE teaching_class_id = ?`, id); err != nil {
		return fmt.Errorf("delete members: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM teaching_classes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete teaching class: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete teaching class rows affected: %w", err)
	}
	if n == 0 {
		return ErrTeachingClassNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete teaching class: %w", err)
	}
	return nil
}

// insertMembers inserts teaching_class_members rows within tx.
func insertMembers(ctx context.Context, tx *sql.Tx, teachingClassID int64, adminClassIDs []int64) error {
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
