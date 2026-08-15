package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"ocm-backend/internal/httpx"
	"ocm-backend/internal/iam"
	"ocm-backend/internal/logging"
	"ocm-backend/internal/systemlog"
)

// Handler exposes the /api/auth endpoints.
type Handler struct {
	store  *Store
	tokens *TokenService
	wx     *WxService
	iam    *iam.Store
	logs   *systemlog.Store
}

func NewHandler(store *Store, tokens *TokenService, wx *WxService, iamStore *iam.Store, logStore *systemlog.Store) *Handler {
	return &Handler{store: store, tokens: tokens, wx: wx, iam: iamStore, logs: logStore}
}

// record writes an audit row for auth endpoints, which sit outside the
// audit middleware (they authenticate with credentials, not a JWT Subject).
func (h *Handler) record(r *http.Request, e systemlog.Entry) {
	e.ClientIP = httpx.ClientIP(r)
	h.logs.Record(r.Context(), e)
}

// RegisterRoutes mounts the auth endpoints on mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/login", h.login)
	mux.Handle("GET /api/auth/me", Middleware(h.tokens)(http.HandlerFunc(h.me)))
	// Mini-program login lifecycle. wx-bind/wx-login are public: the caller is
	// authenticated by WeChat via code2Session, not by a JWT. wx-unbind requires
	// an existing session.
	mux.HandleFunc("POST /api/auth/wx-bind", h.wxBind)
	mux.HandleFunc("POST /api/auth/wx-login", h.wxLogin)
	mux.Handle("POST /api/auth/wx-unbind", Middleware(h.tokens)(http.HandlerFunc(h.wxUnbind)))
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// userView is the identity shape returned by every endpoint that hands the
// client a user: base account fields plus the resolved RBAC state, so the
// console and the mini-program can gate UI immediately after login without a
// follow-up /api/auth/me call.
type userView struct {
	ID          int64            `json:"id"`
	Username    string           `json:"username"`
	DisplayName string           `json:"displayName"`
	Type        string           `json:"type"`
	Roles       []iam.RoleBrief  `json:"roles"`
	Groups      []iam.GroupBrief `json:"groups"`
	Permissions []string         `json:"permissions"`
}

type loginResponse struct {
	Token string   `json:"token"`
	User  userView `json:"user"`
}

// enrichUser resolves a user's effective permissions, roles and groups.
func (h *Handler) enrichUser(ctx context.Context, u User) (userView, error) {
	eff, err := h.iam.EffectivePermissions(ctx, u.ID)
	if err != nil {
		return userView{}, err
	}
	groups, err := h.iam.GroupBriefs(ctx, u.ID)
	if err != nil {
		return userView{}, err
	}
	view := userView{
		ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, Type: u.Type,
		Roles: []iam.RoleBrief{}, Groups: groups, Permissions: []string{},
	}
	for _, role := range eff.Roles {
		view.Roles = append(view.Roles, iam.RoleBrief{ID: role.ID, Code: role.Code, Name: role.Name})
	}
	for perm := range eff.Permissions {
		view.Permissions = append(view.Permissions, perm)
	}
	sort.Strings(view.Permissions)
	return view, nil
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
		h.record(r, systemlog.Entry{
			ActorName: req.Username, Method: http.MethodPost, Path: "/api/auth/login",
			StatusCode: http.StatusUnauthorized, Summary: "登录失败"})
		httpx.RespondError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "login failed", err)
		return
	}

	token, err := h.tokens.Issue(user)
	if err != nil {
		httpx.Error500(w, r, "could not issue token", err)
		return
	}
	view, err := h.enrichUser(r.Context(), user)
	if err != nil {
		httpx.Error500(w, r, "could not load account", err)
		return
	}
	h.record(r, systemlog.Entry{
		ActorID: user.ID, ActorName: user.DisplayName, Method: http.MethodPost, Path: "/api/auth/login",
		StatusCode: http.StatusOK, Summary: "用户登录成功"})
	httpx.RespondJSON(w, http.StatusOK, loginResponse{Token: token, User: view})
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
		httpx.Error500(w, r, "could not load account", err)
		return
	}
	view, err := h.enrichUser(r.Context(), user)
	if err != nil {
		httpx.Error500(w, r, "could not load account", err)
		return
	}
	httpx.RespondJSON(w, http.StatusOK, view)
}

// ---- Mini-program (WeChat) login ----

type wxLoginRequest struct {
	Code string `json:"code"`
}

// wxLogin silently re-issues a JWT for a returning mini-program user whose
// WeChat openid is already bound. The code comes from wx.login(); the openid is
// resolved server-side via code2Session -- never trusted from a header.
func (h *Handler) wxLogin(w http.ResponseWriter, r *http.Request) {
	var req wxLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	if req.Code == "" {
		httpx.RespondError(w, http.StatusBadRequest, "code is required")
		return
	}
	openid, err := h.wx.CodeToOpenid(r.Context(), req.Code)
	if err != nil {
		httpx.RespondError(w, http.StatusBadGateway, "微信登录校验失败")
		return
	}
	user, err := h.store.GetByOpenid(r.Context(), openid)
	if errors.Is(err, ErrNotBound) {
		h.record(r, systemlog.Entry{
			Method: http.MethodPost, Path: "/api/auth/wx-login",
			StatusCode: http.StatusNotFound, Summary: "微信登录失败：微信号未绑定"})
		httpx.RespondError(w, http.StatusNotFound, "微信号未绑定账号")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "登录失败", err)
		return
	}
	token, err := h.tokens.Issue(user)
	if err != nil {
		httpx.Error500(w, r, "could not issue token", err)
		return
	}
	view, err := h.enrichUser(r.Context(), user)
	if err != nil {
		httpx.Error500(w, r, "could not load account", err)
		return
	}
	h.record(r, systemlog.Entry{
		ActorID: user.ID, ActorName: user.DisplayName, Method: http.MethodPost, Path: "/api/auth/wx-login",
		StatusCode: http.StatusOK, Summary: "微信小程序登录成功"})
	httpx.RespondJSON(w, http.StatusOK, loginResponse{Token: token, User: view})
}

type wxBindRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Code     string `json:"code"`
}

// wxBind verifies username/password, then binds the caller's WeChat openid
// (resolved from code) to that account and issues a JWT. An account may only be
// bound once; re-binding requires unbinding first.
func (h *Handler) wxBind(w http.ResponseWriter, r *http.Request) {
	var req wxBindRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Code = strings.TrimSpace(req.Code)
	if req.Username == "" || req.Password == "" || req.Code == "" {
		httpx.RespondError(w, http.StatusBadRequest, "请填写账号、密码")
		return
	}
	// Authenticate first so a wrong password short-circuits before consuming
	// the single-use WeChat code.
	user, err := h.store.Authenticate(r.Context(), req.Username, req.Password)
	if errors.Is(err, ErrInvalidCredentials) {
		h.record(r, systemlog.Entry{
			ActorName: req.Username, Method: http.MethodPost, Path: "/api/auth/wx-bind",
			StatusCode: http.StatusUnauthorized, Summary: "微信绑定失败"})
		httpx.RespondError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "登录失败", err)
		return
	}
	openid, err := h.wx.CodeToOpenid(r.Context(), req.Code)
	if err != nil {
		httpx.RespondError(w, http.StatusBadGateway, "微信登录校验失败")
		return
	}
	if err := h.store.BindOpenid(r.Context(), user.ID, openid); err != nil {
		switch {
		case errors.Is(err, ErrAlreadyBound):
			httpx.RespondError(w, http.StatusConflict, "该账号已绑定其他微信号，请先解绑")
		case errors.Is(err, ErrOpenidTaken):
			httpx.RespondError(w, http.StatusConflict, "该微信号已绑定其他账号")
		default:
			httpx.Error500(w, r, "绑定失败", err)
		}
		return
	}
	token, err := h.tokens.Issue(user)
	if err != nil {
		httpx.Error500(w, r, "could not issue token", err)
		return
	}
	view, err := h.enrichUser(r.Context(), user)
	if err != nil {
		httpx.Error500(w, r, "could not load account", err)
		return
	}
	h.record(r, systemlog.Entry{
		ActorID: user.ID, ActorName: user.DisplayName, Method: http.MethodPost, Path: "/api/auth/wx-bind",
		StatusCode: http.StatusOK, Summary: "绑定微信账号"})
	httpx.RespondJSON(w, http.StatusOK, loginResponse{Token: token, User: view})
}

// wxUnbind clears the WeChat openid bound to the authenticated account. After
// this, the next mini-program entry must re-bind with credentials.
func (h *Handler) wxUnbind(w http.ResponseWriter, r *http.Request) {
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
		httpx.Error500(w, r, "解绑失败", err)
		return
	}
	if err := h.store.UnbindOpenid(r.Context(), username); err != nil {
		httpx.Error500(w, r, "解绑失败", err)
		return
	}
	h.record(r, systemlog.Entry{
		ActorID: user.ID, ActorName: user.DisplayName, Method: http.MethodPost, Path: "/api/auth/wx-unbind",
		StatusCode: http.StatusNoContent, Summary: "解绑微信账号"})
	w.WriteHeader(http.StatusNoContent)
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
			// Make the username visible to the access-log middleware, which sits
			// upstream and cannot see derived request contexts.
			logging.WithUser(r.Context(), username)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, username)))
		})
	}
}
