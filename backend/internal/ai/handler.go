package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/booking"
	"ocm-backend/internal/classroom"
	"ocm-backend/internal/course"
	"ocm-backend/internal/httpx"
	"ocm-backend/internal/schedule"
	"ocm-backend/internal/systemlog"
)

// Request caps: the frontend re-sends the in-memory history on every turn, so
// the backend bounds how much of it is accepted.
const (
	maxHistoryMessages = 30
	maxMessageLen      = 4000
)

// Handler serves the AI settings (admin-only) and the streaming chat endpoint
// (permission-gated). It depends on the classroom/schedule/course/booking
// stores for the assistant's tools.
type Handler struct {
	store      *Store
	classrooms *classroom.Store
	regimes    *schedule.Store
	courses    *course.Store
	bookings   *booking.Store
}

func NewHandler(store *Store, classrooms *classroom.Store, regimes *schedule.Store,
	courses *course.Store, bookings *booking.Store) *Handler {
	return &Handler{
		store:      store,
		classrooms: classrooms,
		regimes:    regimes,
		courses:    courses,
		bookings:   bookings,
	}
}

func validateChatRequest(in *ChatRequest) (string, bool) {
	if len(in.Messages) > maxHistoryMessages {
		return "too many messages (max 30)", false
	}
	for _, m := range in.Messages {
		if m.Role != "user" && m.Role != "assistant" {
			return "invalid message role", false
		}
		if len(m.Content) > maxMessageLen {
			return "message content too long", false
		}
	}
	return "", true
}

// chat streams one assistant turn as SSE. Everything that can fail before the
// stream is committed (bad request, settings disabled, upstream transport
// error) answers as plain JSON with a proper status code; once streaming
// starts, failures become `error` events. This request runs through the audit
// middleware, so the summary annotation lands on its own audit row.
func (h *Handler) chat(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var in ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := validateChatRequest(&in); !ok {
		httpx.RespondError(w, http.StatusBadRequest, msg)
		return
	}
	settings, err := h.store.Get(r.Context())
	if err != nil {
		httpx.Error500(w, r, "could not load ai settings", err)
		return
	}
	if !settings.Enabled {
		httpx.RespondError(w, http.StatusServiceUnavailable, "AI 助手未启用，请联系管理员在参数配置中启用")
		return
	}
	subject, ok := authz.SubjectFrom(r.Context())
	if !ok {
		httpx.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	client := NewClient(settings.BaseURL, settings.APIKey, settings.Model)
	agent := NewAgent(client, h.classrooms, h.regimes, h.courses, h.bookings, subject)

	// Preflight the first upstream round before committing to the stream, so
	// transport-level failures still answer as JSON (and audit as such).
	ctx, cancel := context.WithTimeout(r.Context(), totalTimeout)
	defer cancel()
	msgs := agent.buildMessages(in.Messages)
	roundCtx, roundCancel := context.WithTimeout(ctx, roundTimeout)
	stream, err := client.StreamRequest(roundCtx, msgs, agent.Definitions())
	if err != nil {
		roundCancel()
		if ctx.Err() != nil {
			return // client went away before we answered
		}
		httpx.RespondError(w, http.StatusBadGateway, friendlyUpstreamErr(err))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	systemlog.WithSummary(r.Context(), "使用AI助手")

	emit := func(name string, data any) {
		b, err := json.Marshal(data)
		if err != nil {
			return
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b); err != nil {
			return
		}
		_ = http.NewResponseController(w).Flush()
	}

	if err := agent.runRounds(ctx, msgs, roundCtx, roundCancel, stream, emit); err != nil {
		if errors.Is(err, context.Canceled) && r.Context().Err() != nil {
			return // client aborted; the partial text stays rendered
		}
		emit("error", map[string]any{"message": friendlyUpstreamErr(err)})
	}
}

// friendlyUpstreamErr maps internal errors to user-facing Chinese messages.
// Nothing upstream-specific (URL, key, status, body) is ever echoed.
func friendlyUpstreamErr(err error) string {
	switch {
	case errors.Is(err, ErrUpstreamAuth), errors.Is(err, ErrUpstreamNotFound), errors.Is(err, ErrUpstreamRate):
		return err.Error()
	case errors.Is(err, ErrUpstreamUnavailable):
		return ErrUpstreamUnavailable.Error()
	case errors.Is(err, context.DeadlineExceeded):
		return "AI 助手响应超时，请稍后再试"
	default:
		// Agent-level errors are already Chinese user-facing messages
		// (e.g. "AI 助手处理步骤过多…").
		return err.Error()
	}
}
