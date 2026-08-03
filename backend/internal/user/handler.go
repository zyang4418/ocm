package user

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"ocm-backend/internal/auth"
	"ocm-backend/internal/httpx"
)

type Handler struct {
	store *Store
}

func NewHandler(store *Store) *Handler {
	return &Handler{store: store}
}

// RegisterRoutes mounts the user-management endpoints on mux. All routes are
// protected by authMw (token validation) followed by an admin-only check.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authMw func(http.Handler) http.Handler) {
	wrap := func(handler http.HandlerFunc) http.Handler {
		return authMw(h.requireAdmin(http.HandlerFunc(handler)))
	}
	mux.Handle("GET /api/users", wrap(h.list))
	mux.Handle("POST /api/users", wrap(h.create))
	mux.Handle("GET /api/users/{id}", wrap(h.get))
	mux.Handle("PUT /api/users/{id}", wrap(h.update))
	mux.Handle("PATCH /api/users/{id}/password", wrap(h.changePassword))
	mux.Handle("DELETE /api/users/{id}", wrap(h.delete))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	users, err := h.store.List(r.Context())
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not list users")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, users)
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
	if requester, ok := requesterFrom(r.Context()); ok && id == requester.ID {
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

type requesterKey struct{}

// requireAdmin ensures the authenticated user has the admin role. It loads the
// requester's profile once and stores it in the context for downstream handlers.
func (h *Handler) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, ok := auth.UsernameFrom(r.Context())
		if !ok {
			httpx.RespondError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		u, err := h.store.GetByUsername(r.Context(), username)
		if err != nil {
			httpx.RespondError(w, http.StatusUnauthorized, "account not found")
			return
		}
		if u.Role != RoleAdmin {
			httpx.RespondError(w, http.StatusForbidden, "admin access required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requesterKey{}, u)))
	})
}

func requesterFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(requesterKey{}).(User)
	return u, ok
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func validRole(role string) bool {
	return role == RoleAdmin || role == RoleUser
}
