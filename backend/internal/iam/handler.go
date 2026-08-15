package iam

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/httpx"
	"ocm-backend/internal/systemlog"
)

// roleCodeRe validates role codes: lowercase start, then lowercase letters,
// digits or underscores, up to 64 chars.
var roleCodeRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

type Handler struct {
	store *Store
}

func NewHandler(store *Store) *Handler {
	return &Handler{store: store}
}

// RegisterRoutes mounts the permission catalog, role and user-group
// endpoints on mux. Every route runs behind authenticate and a permission
// check.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	wrap := func(perm string, handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(perm)(http.HandlerFunc(handler)))
	}
	wrapAny := func(handler http.HandlerFunc, perms ...string) http.Handler {
		return authenticate(authz.RequireAny(perms...)(http.HandlerFunc(handler)))
	}
	mux.Handle("GET /api/permissions", wrapAny(h.listPermissions, authz.RoleRead, authz.RoleManage, authz.UserManage))
	mux.Handle("GET /api/roles", wrapAny(h.listRoles, authz.RoleRead, authz.RoleManage, authz.UserManage))
	mux.Handle("POST /api/roles", wrap(authz.RoleManage, h.createRole))
	mux.Handle("PUT /api/roles/{id}", wrap(authz.RoleManage, h.updateRole))
	mux.Handle("DELETE /api/roles/{id}", wrap(authz.RoleManage, h.deleteRole))
	mux.Handle("GET /api/groups", wrapAny(h.listGroups, authz.GroupRead, authz.GroupManage, authz.UserManage))
	mux.Handle("GET /api/groups/{id}", wrapAny(h.getGroup, authz.GroupRead, authz.GroupManage))
	mux.Handle("POST /api/groups", wrap(authz.GroupManage, h.createGroup))
	mux.Handle("PUT /api/groups/{id}", wrap(authz.GroupManage, h.updateGroup))
	mux.Handle("DELETE /api/groups/{id}", wrap(authz.GroupManage, h.deleteGroup))
}

// listPermissions returns the full permission catalog (defined in code).
func (h *Handler) listPermissions(w http.ResponseWriter, _ *http.Request) {
	httpx.RespondJSON(w, http.StatusOK, authz.Catalog)
}

func (h *Handler) listRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.store.ListRoles(r.Context())
	if err != nil {
		httpx.Error500(w, r, "could not list roles", err)
		return
	}
	httpx.RespondJSON(w, http.StatusOK, roles)
}

// validateRoleInput checks the fields shared by create and update. Code is
// only validated when creating (immutable afterwards).
func validateRoleInput(in *RoleInput, requireCode bool) string {
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if requireCode && !roleCodeRe.MatchString(in.Code) {
		return "code must be lowercase letters, digits or underscores, starting with a letter"
	}
	if in.Name == "" {
		return "name is required"
	}
	for _, perm := range in.Permissions {
		if !authz.PermissionExists(perm) {
			// Also rejects "*": the wildcard is not part of the catalog and
			// can only exist on the system admin role.
			return fmt.Sprintf("unknown permission: %s", perm)
		}
	}
	return ""
}

func (h *Handler) createRole(w http.ResponseWriter, r *http.Request) {
	var in RoleInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := validateRoleInput(&in, true); msg != "" {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	role, err := h.store.CreateRole(r.Context(), in)
	if errors.Is(err, ErrCodeTaken) {
		httpx.RespondError(w, http.StatusConflict, "role code already taken")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not create role", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("创建角色 %s", role.Name))
	httpx.RespondJSON(w, http.StatusCreated, role)
}

func (h *Handler) updateRole(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid role id")
		return
	}
	existing, err := h.store.GetRoleByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "role not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not load role", err)
		return
	}
	if existing.IsSystem {
		httpx.RespondError(w, http.StatusConflict, "系统内置角色不可修改")
		return
	}
	var in RoleInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Code is immutable: PUT updates only name/description/permissions.
	in.Code = existing.Code
	if msg := validateRoleInput(&in, false); msg != "" {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	role, err := h.store.UpdateRole(r.Context(), id, in)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "role not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not update role", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("更新角色 %s", role.Name))
	httpx.RespondJSON(w, http.StatusOK, role)
}

func (h *Handler) deleteRole(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid role id")
		return
	}
	existing, err := h.store.GetRoleByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "role not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not load role", err)
		return
	}
	if existing.IsSystem {
		httpx.RespondError(w, http.StatusConflict, "系统内置角色不可删除")
		return
	}
	users, groups, err := h.store.RoleUsageCounts(r.Context(), id)
	if err != nil {
		httpx.Error500(w, r, "could not check role usage", err)
		return
	}
	if users+groups > 0 {
		// Refuse rather than cascade: silently stripping a role from users
		// would be worse than asking the operator to reassign first.
		httpx.RespondError(w, http.StatusConflict,
			fmt.Sprintf("该角色仍被 %d 个用户、%d 个用户组使用，请先移除关联", users, groups))
		return
	}
	if err := h.store.DeleteRole(r.Context(), id); err != nil {
		httpx.Error500(w, r, "could not delete role", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("删除角色 %s", existing.Name))
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.store.ListGroups(r.Context())
	if err != nil {
		httpx.Error500(w, r, "could not list groups", err)
		return
	}
	httpx.RespondJSON(w, http.StatusOK, groups)
}

func (h *Handler) getGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid group id")
		return
	}
	group, err := h.store.GetGroupByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "group not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not load group", err)
		return
	}
	httpx.RespondJSON(w, http.StatusOK, group)
}

// validateGroupInput checks the shared create/update fields and the
// admin-role guard: only wildcard holders may attach the admin role to a
// group (otherwise a group manager could grant themselves the wildcard via
// group membership).
func (h *Handler) validateGroupInput(w http.ResponseWriter, r *http.Request, in *GroupInput) bool {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if in.Name == "" {
		httpx.RespondError(w, http.StatusBadRequest, "name is required")
		return false
	}
	ok, err := h.store.UsersExist(r.Context(), in.Members)
	if err != nil {
		httpx.Error500(w, r, "could not verify members", err)
		return false
	}
	if !ok {
		httpx.RespondError(w, http.StatusBadRequest, "one or more members do not exist")
		return false
	}
	if len(in.Roles) > 0 {
		roles, err := h.store.ListRoles(r.Context())
		if err != nil {
			httpx.Error500(w, r, "could not load roles", err)
			return false
		}
		byID := make(map[int64]Role, len(roles))
		for _, role := range roles {
			byID[role.ID] = role
		}
		subject, _ := authz.SubjectFrom(r.Context())
		for _, roleID := range in.Roles {
			role, exists := byID[roleID]
			if !exists {
				httpx.RespondError(w, http.StatusBadRequest, fmt.Sprintf("unknown role id: %d", roleID))
				return false
			}
			if role.Code == CodeAdmin && !subject.Has(authz.Wildcard) {
				httpx.RespondError(w, http.StatusForbidden, "only administrators can grant the admin role")
				return false
			}
		}
	}
	return true
}

func (h *Handler) createGroup(w http.ResponseWriter, r *http.Request) {
	var in GroupInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !h.validateGroupInput(w, r, &in) {
		return
	}
	group, err := h.store.CreateGroup(r.Context(), in)
	if errors.Is(err, ErrNameTaken) {
		httpx.RespondError(w, http.StatusConflict, "group name already taken")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not create group", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("创建用户组 %s", group.Name))
	httpx.RespondJSON(w, http.StatusCreated, group)
}

func (h *Handler) updateGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid group id")
		return
	}
	var in GroupInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !h.validateGroupInput(w, r, &in) {
		return
	}
	group, err := h.store.UpdateGroup(r.Context(), id, in)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "group not found")
		return
	}
	if errors.Is(err, ErrNameTaken) {
		httpx.RespondError(w, http.StatusConflict, "group name already taken")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not update group", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("更新用户组 %s", group.Name))
	httpx.RespondJSON(w, http.StatusOK, group)
}

func (h *Handler) deleteGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid group id")
		return
	}
	existing, err := h.store.GetGroupByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.RespondError(w, http.StatusNotFound, "group not found")
			return
		}
		httpx.Error500(w, r, "could not load group", err)
		return
	}
	if err := h.store.DeleteGroup(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.RespondError(w, http.StatusNotFound, "group not found")
			return
		}
		httpx.Error500(w, r, "could not delete group", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("删除用户组 %s", existing.Name))
	w.WriteHeader(http.StatusNoContent)
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}
