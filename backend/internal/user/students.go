package user

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/dbutil"
	"ocm-backend/internal/httpx"
	"ocm-backend/internal/systemlog"
)

var (
	ErrStudentNotFound   = errors.New("student profile not found")
	ErrStudentNotStudent = errors.New("user is not a student")
	ErrStudentAssigned   = errors.New("student already assigned to an admin class")
)

// StudentProfileView is one roster entry: a student account bound to an admin
// class. Username/DisplayName come from the users table; StudentNo/Note live
// in the profile so the users table stays untouched.
type StudentProfileView struct {
	UserID       int64  `json:"userId"`
	Username     string `json:"username"`
	DisplayName  string `json:"displayName"`
	StudentNo    string `json:"studentNo"`
	AdminClassID int64  `json:"adminClassId"`
	Note         string `json:"note"`
}

// StudentProfileInput is the add/update payload for one roster entry.
type StudentProfileInput struct {
	UserID    int64  `json:"userId"`
	StudentNo string `json:"studentNo"`
	Note      string `json:"note"`
}

// ---- Store ----

// ListStudents returns the roster of one admin class, ordered by display name.
func (s *Store) ListStudents(ctx context.Context, adminClassID int64) ([]StudentProfileView, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT sp.user_id, u.username, u.display_name, sp.student_no, sp.admin_class_id, sp.note
FROM student_profiles sp
JOIN users u ON u.id = sp.user_id
WHERE sp.admin_class_id = ?
ORDER BY u.display_name, u.id`, adminClassID)
	if err != nil {
		return nil, fmt.Errorf("list students: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var list []StudentProfileView
	for rows.Next() {
		var v StudentProfileView
		if err := rows.Scan(&v.UserID, &v.Username, &v.DisplayName, &v.StudentNo, &v.AdminClassID, &v.Note); err != nil {
			return nil, fmt.Errorf("scan student: %w", err)
		}
		list = append(list, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if list == nil {
		list = []StudentProfileView{}
	}
	return list, nil
}

// AddStudent binds an existing student account to the admin class. The user
// must exist and be of type student; one student belongs to one class
// (UNIQUE(user_id)).
func (s *Store) AddStudent(ctx context.Context, adminClassID int64, in StudentProfileInput) (StudentProfileView, error) {
	var userType string
	if err := s.db.QueryRowContext(ctx,
		`SELECT user_type FROM users WHERE id = ?`, in.UserID).Scan(&userType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return StudentProfileView{}, ErrStudentNotFound
		}
		return StudentProfileView{}, fmt.Errorf("lookup student user: %w", err)
	}
	if userType != "student" {
		return StudentProfileView{}, ErrStudentNotStudent
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO student_profiles (user_id, admin_class_id, student_no, note) VALUES (?, ?, ?, ?)`,
		in.UserID, adminClassID, strings.TrimSpace(in.StudentNo), strings.TrimSpace(in.Note)); err != nil {
		if dbutil.IsDuplicateEntry(err) {
			return StudentProfileView{}, ErrStudentAssigned
		}
		return StudentProfileView{}, fmt.Errorf("add student: %w", err)
	}
	return s.getStudent(ctx, adminClassID, in.UserID)
}

// UpdateStudent updates the profile metadata (student no, note).
func (s *Store) UpdateStudent(ctx context.Context, adminClassID, userID int64, in StudentProfileInput) (StudentProfileView, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE student_profiles SET student_no = ?, note = ? WHERE admin_class_id = ? AND user_id = ?`,
		strings.TrimSpace(in.StudentNo), strings.TrimSpace(in.Note), adminClassID, userID)
	if err != nil {
		return StudentProfileView{}, fmt.Errorf("update student: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return StudentProfileView{}, fmt.Errorf("update student rows affected: %w", err)
	}
	if n == 0 {
		return StudentProfileView{}, ErrStudentNotFound
	}
	return s.getStudent(ctx, adminClassID, userID)
}

// DeleteStudent removes the student from the class roster. Historical checkin
// records are untouched (they store user_id only).
func (s *Store) DeleteStudent(ctx context.Context, adminClassID, userID int64) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM student_profiles WHERE admin_class_id = ? AND user_id = ?`, adminClassID, userID)
	if err != nil {
		return fmt.Errorf("delete student: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete student rows affected: %w", err)
	}
	if n == 0 {
		return ErrStudentNotFound
	}
	return nil
}

func (s *Store) getStudent(ctx context.Context, adminClassID, userID int64) (StudentProfileView, error) {
	var v StudentProfileView
	err := s.db.QueryRowContext(ctx, `
SELECT sp.user_id, u.username, u.display_name, sp.student_no, sp.admin_class_id, sp.note
FROM student_profiles sp
JOIN users u ON u.id = sp.user_id
WHERE sp.admin_class_id = ? AND sp.user_id = ?`, adminClassID, userID).
		Scan(&v.UserID, &v.Username, &v.DisplayName, &v.StudentNo, &v.AdminClassID, &v.Note)
	if errors.Is(err, sql.ErrNoRows) {
		return StudentProfileView{}, ErrStudentNotFound
	}
	if err != nil {
		return StudentProfileView{}, fmt.Errorf("load student: %w", err)
	}
	return v, nil
}

// ---- Routes ----

// registerStudentRoutes mounts the roster endpoints under the admin-class
// resource. Reading needs attendance:read (roster viewing is an attendance
// concern); maintenance needs attendance:manage so teachers can curate the
// roster of their own classes.
func (h *Handler) registerStudentRoutes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	withPerm := func(perm string, handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(perm)(http.HandlerFunc(handler)))
	}
	mux.Handle("GET /api/admin-classes/{id}/students", withPerm(authz.AttendanceRead, h.listStudents))
	mux.Handle("POST /api/admin-classes/{id}/students", withPerm(authz.AttendanceManage, h.addStudent))
	mux.Handle("PUT /api/admin-classes/{id}/students/{userId}", withPerm(authz.AttendanceManage, h.updateStudent))
	mux.Handle("DELETE /api/admin-classes/{id}/students/{userId}", withPerm(authz.AttendanceManage, h.deleteStudent))
}

// ---- Handlers ----

// @Summary      List a class's roster
// @Tags         org
// @Produce      json
// @Param        id path int true "admin class id"
// @Success      200 {array} StudentProfileView "roster entries"
// @Failure      400 {object} httpx.ErrorResponse "invalid admin class id"
// @Failure      404 {object} httpx.ErrorResponse "admin class not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/admin-classes/{id}/students [get]
func (h *Handler) listStudents(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid admin class id")
		return
	}
	if _, err := h.store.GetAdminClass(r.Context(), id); err != nil {
		if errors.Is(err, ErrAdminClassNotFound) {
			httpx.RespondError(w, http.StatusNotFound, "admin class not found")
			return
		}
		httpx.Error500(w, r, "could not load admin class", err)
		return
	}
	list, err := h.store.ListStudents(r.Context(), id)
	if err != nil {
		httpx.Error500(w, r, "could not list students", err)
		return
	}
	httpx.RespondJSON(w, http.StatusOK, list)
}

// @Summary      Add a student to the roster
// @Tags         org
// @Accept       json
// @Produce      json
// @Param        id path int true "admin class id"
// @Param        body body StudentProfileInput true "roster entry input"
// @Success      201 {object} StudentProfileView "added roster entry"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / user is not a student"
// @Failure      404 {object} httpx.ErrorResponse "admin class not found"
// @Failure      409 {object} httpx.ErrorResponse "student already in a class"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/admin-classes/{id}/students [post]
func (h *Handler) addStudent(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid admin class id")
		return
	}
	var in StudentProfileInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if in.UserID <= 0 {
		httpx.RespondError(w, http.StatusBadRequest, "userId is required")
		return
	}
	v, err := h.store.AddStudent(r.Context(), id, in)
	switch {
	case errors.Is(err, ErrStudentNotFound):
		httpx.RespondError(w, http.StatusNotFound, "用户不存在")
	case errors.Is(err, ErrStudentNotStudent):
		httpx.RespondError(w, http.StatusBadRequest, "该用户不是学生账号")
	case errors.Is(err, ErrStudentAssigned):
		httpx.RespondError(w, http.StatusConflict, "该学生已分配到行政班")
	case err != nil:
		httpx.Error500(w, r, "could not add student", err)
	default:
		systemlog.WithSummary(r.Context(), fmt.Sprintf("行政班 #%d 添加学生 %s", id, v.DisplayName))
		httpx.RespondJSON(w, http.StatusCreated, v)
	}
}

// @Summary      Update a roster entry
// @Tags         org
// @Accept       json
// @Produce      json
// @Param        id path int true "admin class id"
// @Param        userId path int true "user id"
// @Param        body body StudentProfileInput true "roster entry input"
// @Success      200 {object} StudentProfileView "updated roster entry"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / user is not in this class"
// @Failure      404 {object} httpx.ErrorResponse "admin class not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/admin-classes/{id}/students/{userId} [put]
func (h *Handler) updateStudent(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid admin class id")
		return
	}
	userID, err := strconv.ParseInt(r.PathValue("userId"), 10, 64)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var in StudentProfileInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	v, err := h.store.UpdateStudent(r.Context(), id, userID, in)
	switch {
	case errors.Is(err, ErrStudentNotFound):
		httpx.RespondError(w, http.StatusNotFound, "该班无此学生")
	case err != nil:
		httpx.Error500(w, r, "could not update student", err)
	default:
		systemlog.WithSummary(r.Context(), fmt.Sprintf("更新学生档案 %s", v.DisplayName))
		httpx.RespondJSON(w, http.StatusOK, v)
	}
}

// @Summary      Remove a student from the roster
// @Tags         org
// @Param        id path int true "admin class id"
// @Param        userId path int true "user id"
// @Success      204 "no content"
// @Failure      400 {object} httpx.ErrorResponse "user is not in this class"
// @Failure      404 {object} httpx.ErrorResponse "admin class not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/admin-classes/{id}/students/{userId} [delete]
func (h *Handler) deleteStudent(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid admin class id")
		return
	}
	userID, err := strconv.ParseInt(r.PathValue("userId"), 10, 64)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.store.DeleteStudent(r.Context(), id, userID); err != nil {
		switch {
		case errors.Is(err, ErrStudentNotFound):
			httpx.RespondError(w, http.StatusNotFound, "该班无此学生")
		default:
			httpx.Error500(w, r, "could not delete student", err)
		}
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("行政班 #%d 移除学生 #%d", id, userID))
	w.WriteHeader(http.StatusNoContent)
}
