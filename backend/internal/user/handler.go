package user

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"ocm-backend/internal/auth"
	"ocm-backend/internal/authz"
	"ocm-backend/internal/dbutil"
	"ocm-backend/internal/httpx"
	"ocm-backend/internal/iam"
	"ocm-backend/internal/logging"
	"ocm-backend/internal/systemlog"
)

type Handler struct {
	store *Store
	iam   *iam.Store
}

func NewHandler(store *Store, iamStore *iam.Store) *Handler {
	return &Handler{store: store, iam: iamStore}
}

// RegisterRoutes mounts the user-management endpoints on mux. Every route
// runs behind authenticate (the composed JWT + user-loading middleware) and
// a permission check.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	wrap := func(perm string, handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(perm)(http.HandlerFunc(handler)))
	}
	// List-like endpoints admit any of several management permissions; write
	// endpoints require a specific one.
	wrapAny := func(handler http.HandlerFunc, perms ...string) http.Handler {
		return authenticate(authz.RequireAny(perms...)(http.HandlerFunc(handler)))
	}
	// GET /api/users doubles as the group member picker, so group:manage
	// holders may also list users.
	mux.Handle("GET /api/users", wrapAny(h.list, authz.UserRead, authz.GroupManage))
	mux.Handle("POST /api/users", wrap(authz.UserManage, h.create))
	mux.Handle("GET /api/users/{id}", wrap(authz.UserRead, h.get))
	mux.Handle("PUT /api/users/{id}", wrap(authz.UserManage, h.update))
	mux.Handle("PATCH /api/users/{id}/password", wrap(authz.UserManage, h.changePassword))
	mux.Handle("DELETE /api/users/{id}", wrap(authz.UserManage, h.delete))
	mux.Handle("GET /api/users/{id}/grants", wrap(authz.UserRead, h.getGrants))
	mux.Handle("PUT /api/users/{id}/roles", wrap(authz.UserManage, h.putRoles))
	mux.Handle("PUT /api/users/{id}/permissions", wrap(authz.UserManage, h.putPermissions))

	// Admin classes (行政班) and teaching classes (教学班) belong to the
	// user/people module. Mounted via registerOrgRoutes so handler.go stays
	// focused on account management.
	h.registerOrgRoutes(mux, authenticate)
}

// @Summary      List users
// @Tags         users
// @Produce      json
// @Param        q query string false "search by username/display name"
// @Param        page query int false "1-based page" default(1)
// @Param        page_size query int false "page size" default(100)
// @Success      200 {object} httpx.Paged "paged users with role/group summaries"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/users [get]
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := httpx.ParsePageParams(q)
	users, total, err := h.store.PageUsers(r.Context(), httpx.ParseSearch(q),
		dbutil.Pagination{Limit: p.PageSize, Offset: p.Offset()})
	if err != nil {
		httpx.Error500(w, r, "could not list users", err)
		return
	}
	if users == nil {
		users = []User{}
	}
	// Fill the roles/groups summary columns. A failure here is logged but not
	// fatal: the list is still usable without the summary columns.
	if len(users) > 0 {
		ids := make([]int64, len(users))
		for i, u := range users {
			ids[i] = u.ID
		}
		summaries, err := h.iam.BatchSummaries(r.Context(), ids)
		if err != nil {
			logging.L.Error("user list summaries", "err", err)
		} else {
			for i := range users {
				s, ok := summaries[users[i].ID]
				if !ok {
					continue
				}
				users[i].Roles = s.Roles
				users[i].Groups = s.Groups
			}
		}
	}
	httpx.RespondPaged(w, users, total, p)
}

// @Summary      Create a user
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        body body CreateUserInput true "create user input"
// @Success      201 {object} User "created user"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / required fields missing"
// @Failure      409 {object} httpx.ErrorResponse "username already taken"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/users [post]
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateUserInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.Type = strings.TrimSpace(in.Type)
	if in.Username == "" || in.Password == "" || in.DisplayName == "" {
		httpx.RespondError(w, http.StatusBadRequest, "username, password, and displayName are required")
		return
	}
	if in.Type == "" {
		in.Type = TypeStaff
	}
	if !validType(in.Type) {
		httpx.RespondError(w, http.StatusBadRequest, "type must be 'student', 'teacher' or 'staff'")
		return
	}
	u, err := h.store.Create(r.Context(), in)
	if errors.Is(err, ErrUsernameTaken) {
		httpx.RespondError(w, http.StatusConflict, "username already taken")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not create user", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("创建用户 %s", u.DisplayName))
	httpx.RespondJSON(w, http.StatusCreated, u)
}

// @Summary      Get a user
// @Tags         users
// @Produce      json
// @Param        id path int true "user id"
// @Success      200 {object} User "user detail"
// @Failure      400 {object} httpx.ErrorResponse "invalid user id"
// @Failure      404 {object} httpx.ErrorResponse "user not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/users/{id} [get]
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	u, err := h.store.GetByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not load user", err)
		return
	}
	httpx.RespondJSON(w, http.StatusOK, u)
}

// @Summary      Update a user
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        id path int true "user id"
// @Param        body body UpdateUserInput true "update user input"
// @Success      200 {object} User "updated user"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / displayName required"
// @Failure      404 {object} httpx.ErrorResponse "user not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/users/{id} [put]
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var in UpdateUserInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.Type = strings.TrimSpace(in.Type)
	if in.DisplayName == "" {
		httpx.RespondError(w, http.StatusBadRequest, "displayName is required")
		return
	}
	if !validType(in.Type) {
		httpx.RespondError(w, http.StatusBadRequest, "type must be 'student', 'teacher' or 'staff'")
		return
	}
	u, err := h.store.Update(r.Context(), id, in)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not update user", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("更新用户 %s", u.DisplayName))
	httpx.RespondJSON(w, http.StatusOK, u)
}

// @Summary      Reset a user's password
// @Tags         users
// @Accept       json
// @Param        id path int true "user id"
// @Param        body body ChangePasswordInput true "new password"
// @Success      204 "no content"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / password required"
// @Failure      403 {object} httpx.ErrorResponse "only administrators can change an administrator's password"
// @Failure      404 {object} httpx.ErrorResponse "user not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/users/{id}/password [patch]
func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var in ChangePasswordInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if in.Password == "" {
		httpx.RespondError(w, http.StatusBadRequest, "password is required")
		return
	}
	// A user:manage holder must not take over an administrator account by
	// resetting its password — the wildcard would come with it. Mirrors the
	// admin-role guards in putRoles and group editing.
	eff, err := h.iam.EffectivePermissions(r.Context(), id)
	if err != nil {
		httpx.Error500(w, r, "could not load user permissions", err)
		return
	}
	if eff.Permissions[authz.Wildcard] {
		subject, ok := authz.SubjectFrom(r.Context())
		if !ok {
			httpx.RespondError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		if !subject.Has(authz.Wildcard) {
			httpx.RespondError(w, http.StatusForbidden, "only administrators can change an administrator's password")
			return
		}
	}
	err = h.store.UpdatePassword(r.Context(), id, in.Password)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not update password", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("重置密码：用户 #%d", id))
	w.WriteHeader(http.StatusNoContent)
}

// @Summary      Delete a user
// @Tags         users
// @Param        id path int true "user id"
// @Success      204 "no content"
// @Failure      400 {object} httpx.ErrorResponse "invalid user id"
// @Failure      403 {object} httpx.ErrorResponse "only administrators can delete an administrator's account"
// @Failure      409 {object} httpx.ErrorResponse "cannot delete your own account"
// @Failure      404 {object} httpx.ErrorResponse "user not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/users/{id} [delete]
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	subject, ok := authz.SubjectFrom(r.Context())
	if !ok {
		httpx.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if id == subject.ID {
		httpx.RespondError(w, http.StatusConflict, "cannot delete your own account")
		return
	}
	// A user:manage holder must not remove an administrator account (lock-out
	// is almost as good as takeover). Mirrors the admin guards in
	// changePassword, putRoles and group editing.
	eff, err := h.iam.EffectivePermissions(r.Context(), id)
	if err != nil {
		httpx.Error500(w, r, "could not load user permissions", err)
		return
	}
	if eff.Permissions[authz.Wildcard] && !subject.Has(authz.Wildcard) {
		httpx.RespondError(w, http.StatusForbidden, "only administrators can delete an administrator's account")
		return
	}
	target, err := h.store.GetByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not load user", err)
		return
	}
	err = h.store.Delete(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not delete user", err)
		return
	}
	// Clean up grant rows (role grants, direct permissions, group
	// membership). A failure only leaves harmless orphan rows, so log it.
	if err := h.iam.DeleteUserGrants(r.Context(), id); err != nil {
		logging.L.Error("user delete grants cleanup", "err", err, "user_id", id)
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("删除用户 %s", target.DisplayName))
	w.WriteHeader(http.StatusNoContent)
}

// ---- Grants (roles / permissions / groups) ----

// @Summary      Get a user's grants (roles / permissions / groups)
// @Tags         users
// @Produce      json
// @Param        id path int true "user id"
// @Success      200 {object} iam.UserGrantView "grant view incl. expired rows"
// @Failure      400 {object} httpx.ErrorResponse "invalid user id"
// @Failure      404 {object} httpx.ErrorResponse "user not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/users/{id}/grants [get]
func (h *Handler) getGrants(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if _, err := h.store.GetByID(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.RespondError(w, http.StatusNotFound, "user not found")
			return
		}
		httpx.Error500(w, r, "could not load user", err)
		return
	}
	view, err := h.iam.UserGrants(r.Context(), id)
	if err != nil {
		httpx.Error500(w, r, "could not load grants", err)
		return
	}
	httpx.RespondJSON(w, http.StatusOK, view)
}

type putRolesRequest struct {
	Roles []iam.RoleGrantInput `json:"roles"`
}

// putRoles replaces the whole set of direct role grants. Granting the admin
// role requires the wildcard (admin only), so a role manager cannot escalate
// themselves.
//
// @Summary      Replace a user's role grants
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        id path int true "user id"
// @Param        body body UserRolesInput true "whole-set role grants"
// @Success      200 {object} iam.UserGrantView "updated grant view"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / unknown role code"
// @Failure      403 {object} httpx.ErrorResponse "only administrators can grant the admin role"
// @Failure      404 {object} httpx.ErrorResponse "user not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/users/{id}/roles [put]
func (h *Handler) putRoles(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	subject, ok := authz.SubjectFrom(r.Context())
	if !ok {
		httpx.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	target, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.RespondError(w, http.StatusNotFound, "user not found")
			return
		}
		httpx.Error500(w, r, "could not load user", err)
		return
	}
	var req putRolesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	roles, err := h.iam.ListRoles(r.Context())
	if err != nil {
		httpx.Error500(w, r, "could not load roles", err)
		return
	}
	byCode := make(map[string]iam.Role, len(roles))
	for _, role := range roles {
		byCode[role.Code] = role
	}
	for _, g := range req.Roles {
		role, ok := byCode[g.RoleCode]
		if !ok {
			httpx.RespondError(w, http.StatusBadRequest, fmt.Sprintf("unknown role code: %s", g.RoleCode))
			return
		}
		if role.Code == iam.CodeAdmin && !subject.Has(authz.Wildcard) {
			httpx.RespondError(w, http.StatusForbidden, "only administrators can grant the admin role")
			return
		}
	}
	if err := h.iam.ReplaceUserRoles(r.Context(), id, req.Roles, subject.ID); err != nil {
		httpx.Error500(w, r, "could not update roles", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("调整用户 %s 的角色", target.DisplayName))
	h.respondGrants(w, r, id)
}

type putPermissionsRequest struct {
	Permissions []iam.PermGrantInput `json:"permissions"`
}

// putPermissions replaces the whole set of direct permission grants. The
// wildcard is rejected by the catalog check (it is not a catalog entry).
//
// @Summary      Replace a user's permission grants
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        id path int true "user id"
// @Param        body body UserPermissionsInput true "whole-set permission grants"
// @Success      200 {object} iam.UserGrantView "updated grant view"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / unknown permission"
// @Failure      404 {object} httpx.ErrorResponse "user not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/users/{id}/permissions [put]
func (h *Handler) putPermissions(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	subject, ok := authz.SubjectFrom(r.Context())
	if !ok {
		httpx.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	target, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.RespondError(w, http.StatusNotFound, "user not found")
			return
		}
		httpx.Error500(w, r, "could not load user", err)
		return
	}
	var req putPermissionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	for _, g := range req.Permissions {
		if !authz.PermissionExists(g.Permission) {
			httpx.RespondError(w, http.StatusBadRequest, fmt.Sprintf("unknown permission: %s", g.Permission))
			return
		}
	}
	if err := h.iam.ReplaceUserPermissions(r.Context(), id, req.Permissions, subject.ID); err != nil {
		httpx.Error500(w, r, "could not update permissions", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("调整用户 %s 的权限", target.DisplayName))
	h.respondGrants(w, r, id)
}

// respondGrants re-reads and returns the updated grant view after a
// successful replace-set operation.
func (h *Handler) respondGrants(w http.ResponseWriter, r *http.Request, id int64) {
	view, err := h.iam.UserGrants(r.Context(), id)
	if err != nil {
		httpx.Error500(w, r, "could not load grants", err)
		return
	}
	httpx.RespondJSON(w, http.StatusOK, view)
}

// LoadSubject is an auth-pipeline middleware that resolves the authenticated
// username (placed in context by auth.Middleware) to a user record and stores
// an authz.Subject in the context for downstream permission checks. It looks
// up the database on every request (Option B) so role and permission changes
// — including grant expirations — take effect immediately, for both the web
// console and the mini-program.
func LoadSubject(userStore *Store, iamStore *iam.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, ok := auth.UsernameFrom(r.Context())
			if !ok {
				httpx.RespondError(w, http.StatusUnauthorized, "not authenticated")
				return
			}
			u, err := userStore.GetByUsername(r.Context(), username)
			if err != nil {
				httpx.RespondError(w, http.StatusUnauthorized, "account not found")
				return
			}
			eff, err := iamStore.EffectivePermissions(r.Context(), u.ID)
			if err != nil {
				// Permissions failed to load: this is a server error, not a
				// denial — failing open here would bypass every check.
				httpx.Error500(w, r, "could not load permissions", err)
				return
			}
			next.ServeHTTP(w, r.WithContext(authz.WithSubject(r.Context(), authz.Subject{
				ID:          u.ID,
				Type:        u.Type,
				Username:    u.Username,
				DisplayName: u.DisplayName,
				Permissions: eff.Permissions,
				Roles:       eff.RoleCodes(),
				Groups:      eff.GroupIDs,
			})))
		})
	}
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func validType(t string) bool {
	return t == TypeStudent || t == TypeTeacher || t == TypeStaff
}
