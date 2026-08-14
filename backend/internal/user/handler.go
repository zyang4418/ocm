package user

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"ocm-backend/internal/auth"
	"ocm-backend/internal/authz"
	"ocm-backend/internal/dbutil"
	"ocm-backend/internal/httpx"
)

type Handler struct {
	store *Store
}

func NewHandler(store *Store) *Handler {
	return &Handler{store: store}
}

// RegisterRoutes mounts the user-management endpoints on mux. Every route
// runs behind authenticate (the composed JWT + user-loading middleware) and
// a permission check.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	wrap := func(perm string, handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(perm)(http.HandlerFunc(handler)))
	}
	mux.Handle("GET /api/users", wrap(authz.UserManage, h.list))
	mux.Handle("POST /api/users", wrap(authz.UserManage, h.create))
	mux.Handle("GET /api/users/{id}", wrap(authz.UserManage, h.get))
	mux.Handle("PUT /api/users/{id}", wrap(authz.UserManage, h.update))
	mux.Handle("PATCH /api/users/{id}/password", wrap(authz.UserManage, h.changePassword))
	mux.Handle("DELETE /api/users/{id}", wrap(authz.UserManage, h.delete))

	// Admin classes (行政班) and teaching classes (教学班) belong to the
	// user/people module. Mounted via registerOrgRoutes so handler.go stays
	// focused on account management.
	h.registerOrgRoutes(mux, authenticate)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := httpx.ParsePageParams(q)
	users, total, err := h.store.PageUsers(r.Context(), httpx.ParseSearch(q),
		dbutil.Pagination{Limit: p.PageSize, Offset: p.Offset()})
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not list users")
		return
	}
	if users == nil {
		users = []User{}
	}
	httpx.RespondPaged(w, users, total, p)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateUserInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.Username == "" || in.Password == "" || in.DisplayName == "" {
		httpx.RespondError(w, http.StatusBadRequest, "username, password, and displayName are required")
		return
	}
	if !validRole(in.Role) {
		httpx.RespondError(w, http.StatusBadRequest, "role must be 'admin' or 'user'")
		return
	}
	u, err := h.store.Create(r.Context(), in)
	if errors.Is(err, ErrUsernameTaken) {
		httpx.RespondError(w, http.StatusConflict, "username already taken")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not create user")
		return
	}
	httpx.RespondJSON(w, http.StatusCreated, u)
}

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
		httpx.RespondError(w, http.StatusInternalServerError, "could not load user")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, u)
}

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
	if in.DisplayName == "" {
		httpx.RespondError(w, http.StatusBadRequest, "displayName is required")
		return
	}
	if !validRole(in.Role) {
		httpx.RespondError(w, http.StatusBadRequest, "role must be 'admin' or 'user'")
		return
	}
	u, err := h.store.Update(r.Context(), id, in)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not update user")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, u)
}

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
	err = h.store.UpdatePassword(r.Context(), id, in.Password)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not update password")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if subject, ok := authz.SubjectFrom(r.Context()); ok && id == subject.ID {
		httpx.RespondError(w, http.StatusConflict, "cannot delete your own account")
		return
	}
	err = h.store.Delete(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not delete user")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LoadSubject is an auth-pipeline middleware that resolves the authenticated
// username (placed in context by auth.Middleware) to a user record and stores
// an authz.Subject in the context for downstream permission checks. It looks
// up the database on every request (Option B) so role changes take effect
// immediately, for both the web console and the mini-program.
func LoadSubject(store *Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, ok := auth.UsernameFrom(r.Context())
			if !ok {
				httpx.RespondError(w, http.StatusUnauthorized, "not authenticated")
				return
			}
			u, err := store.GetByUsername(r.Context(), username)
			if err != nil {
				httpx.RespondError(w, http.StatusUnauthorized, "account not found")
				return
			}
			next.ServeHTTP(w, r.WithContext(authz.WithSubject(r.Context(), authz.Subject{ID: u.ID, Role: u.Role})))
		})
	}
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func validRole(role string) bool {
	return role == RoleAdmin || role == RoleUser
}
