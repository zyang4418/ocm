package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"ocm-backend/internal/httpx"
)

// Handler exposes the /api/auth endpoints.
type Handler struct {
	store  *Store
	tokens *TokenService
}

func NewHandler(store *Store, tokens *TokenService) *Handler {
	return &Handler{store: store, tokens: tokens}
}

// RegisterRoutes mounts the auth endpoints on mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/login", h.login)
	mux.Handle("GET /api/auth/me", Middleware(h.tokens)(http.HandlerFunc(h.me)))
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		httpx.RespondError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	user, err := h.store.Authenticate(r.Context(), req.Username, req.Password)
	if errors.Is(err, ErrInvalidCredentials) {
		httpx.RespondError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "login failed")
		return
	}

	token, err := h.tokens.Issue(user)
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, loginResponse{Token: token, User: user})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	username, ok := UsernameFrom(r.Context())
	if !ok {
		httpx.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	user, err := h.store.ByUsername(r.Context(), username)
	if errors.Is(err, ErrInvalidCredentials) {
		httpx.RespondError(w, http.StatusUnauthorized, "account no longer exists")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not load account")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, user)
}

type contextKey struct{}

// UsernameFrom extracts the authenticated username stored by Middleware.
func UsernameFrom(ctx context.Context) (string, bool) {
	username, ok := ctx.Value(contextKey{}).(string)
	return username, ok
}

// Middleware returns an HTTP middleware that validates a Bearer token and
// stores the authenticated username in the request context.
func Middleware(tokens *TokenService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			tokenString, found := strings.CutPrefix(header, "Bearer ")
			if !found || tokenString == "" {
				httpx.RespondError(w, http.StatusUnauthorized, "missing bearer token")
				return
			}
			username, err := tokens.Parse(tokenString)
			if err != nil {
				httpx.RespondError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, username)))
		})
	}
}
