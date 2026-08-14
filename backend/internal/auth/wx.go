package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"ocm-backend/internal/logging"
)

// WxService exchanges WeChat mini-program login codes for openids via the
// code2Session API. It is the only place the backend talks to WeChat, and it
// uses the mini-program AppSecret (WX_APP_SECRET) to do so.
//
// The openid is NEVER trusted from the X-WX-OPENID header that the cloud
// gateway injects into callContainer requests: the backend is also reachable
// over the public internet (for the web console), and a direct caller could
// forge that header. Resolving the openid server-side from a single-use code
// is what makes mini-program login safe under public exposure -- and what
// keeps the backend hosting-agnostic (the same flow works in WeChat Cloud Run
// or on a self-hosted server).
type WxService struct {
	appID  string
	secret string
	client *http.Client
}

// NewWxService reads WX_APP_ID/WX_APP_SECRET from the environment.
func NewWxService() *WxService {
	s := &WxService{
		appID:  os.Getenv("WX_APP_ID"),
		secret: os.Getenv("WX_APP_SECRET"),
		client: &http.Client{Timeout: 10 * time.Second},
	}
	if !s.Enabled() {
		logging.L.Warn("auth: WX_APP_ID/WX_APP_SECRET not set; mini-program login (wx-bind/wx-login) will be unavailable")
	}
	return s
}

// Enabled reports whether WeChat login is configured.
func (s *WxService) Enabled() bool {
	return s.appID != "" && s.secret != ""
}

const code2SessionURL = "https://api.weixin.qq.com/sns/jscode2session"

type code2SessionResponse struct {
	Openid     string `json:"openid"`
	SessionKey string `json:"session_key"`
	Unionid    string `json:"unionid"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

// CodeToOpenid exchanges a short-lived wx.login code for the caller's stable
// openid. The code is single-use; callers must obtain a fresh one each time.
func (s *WxService) CodeToOpenid(ctx context.Context, code string) (string, error) {
	if !s.Enabled() {
		return "", errors.New("wx login not configured (set WX_APP_ID and WX_APP_SECRET)")
	}
	q := url.Values{}
	q.Set("appid", s.appID)
	q.Set("secret", s.secret)
	q.Set("js_code", code)
	q.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, code2SessionURL+"?"+q.Encode(), nil)
	if err != nil {
		return "", fmt.Errorf("build code2session request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("call code2session: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var r code2SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", fmt.Errorf("decode code2session response: %w", err)
	}
	if r.ErrCode != 0 {
		return "", fmt.Errorf("code2session error %d: %s", r.ErrCode, r.ErrMsg)
	}
	if r.Openid == "" {
		return "", errors.New("code2session returned empty openid")
	}
	return r.Openid, nil
}
