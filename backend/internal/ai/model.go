package ai

import (
	"net/url"
	"strings"
)

// Field length caps mirror the ai_settings column widths; the store is the
// enforcement backstop, these give the API friendly messages first.
const (
	maxBaseURLLen = 255
	maxAPIKeyLen  = 255
	maxModelLen   = 128
)

// Settings is the admin-editable LLM configuration (single row, id=1).
// APIKey is stored as configured and never leaves the backend: the GET
// response masks it and reports APIKeySet instead.
type Settings struct {
	Enabled bool   `json:"enabled"`
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"`
}

// Normalize trims string fields and drops a trailing slash from the base URL
// so callers (the chat client) don't have to care how it was entered.
func (s *Settings) Normalize() {
	s.BaseURL = strings.TrimSuffix(strings.TrimSpace(s.BaseURL), "/")
	s.APIKey = strings.TrimSpace(s.APIKey)
	s.Model = strings.TrimSpace(s.Model)
}

// validate returns an error message when the settings cannot be activated
// (enabled) or are outright malformed (checked whenever a value is present).
// A disabled service may keep partial configuration, but a filled-in URL must
// always be well-formed. The handler resolves keep-on-empty for the key
// before calling, so APIKey here is the effective value.
func validate(in *Settings) (string, bool) {
	if len(in.BaseURL) > maxBaseURLLen {
		return "baseUrl is too long", false
	}
	if len(in.APIKey) > maxAPIKeyLen {
		return "apiKey is too long", false
	}
	if len(in.Model) > maxModelLen {
		return "model is too long", false
	}
	if in.BaseURL != "" {
		u, err := url.Parse(in.BaseURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "baseUrl must be a valid http(s) URL", false
		}
	}
	if !in.Enabled {
		return "", true
	}
	if in.BaseURL == "" {
		return "baseUrl is required when enabled", false
	}
	if in.Model == "" {
		return "model is required when enabled", false
	}
	if in.APIKey == "" {
		return "apiKey is required when enabled", false
	}
	return "", true
}

// Message is one turn of the conversation history the frontend sends. The
// session is stateless on the backend: the client keeps the history in memory
// and re-sends it with every request.
type Message struct {
	Role    string `json:"role"` // "user" | "assistant"
	Content string `json:"content"`
}

// ChatRequest is the body of POST /api/ai/chat.
type ChatRequest struct {
	Messages []Message `json:"messages"`
}
