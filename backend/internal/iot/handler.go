package iot

import (
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

// Field length caps (mirroring the column definitions).
const (
	maxNameLen     = 128
	maxCategoryLen = 32
)

var validDeviceStatuses = map[string]bool{
	StatusPending: true,
	StatusOnline:  true,
	StatusOffline: true,
}

var validCommandStatuses = map[string]bool{
	CmdStatusQueued:    true,
	CmdStatusDelivered: true,
	CmdStatusAcked:     true,
	CmdStatusFailed:    true,
	CmdStatusExpired:   true,
}

// Handler serves the IoT registry over REST. The MQTT data plane is optional:
// when publisher is nil (no IOT_MQTT_URL) the registry is fully usable and
// command issuing answers 503 — the observation Renderer nil pattern.
type Handler struct {
	store     *Store
	publisher CommandPublisher
	hub       *Hub
	cfg       Config
}

func NewHandler(store *Store, publisher CommandPublisher, hub *Hub, cfg Config) *Handler {
	return &Handler{store: store, publisher: publisher, hub: hub, cfg: cfg}
}

// RegisterRoutes mounts the IoT endpoints. Read covers the registry and the
// live stream; manage covers device lifecycle (claim/update/delete) and
// command history; control is deliberately separate from manage — it gates
// physical-world actions (door open/close, classroom scenes).
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	read := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(authz.IotRead)(http.HandlerFunc(handler)))
	}
	manage := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(authz.IotManage)(http.HandlerFunc(handler)))
	}
	control := func(handler http.HandlerFunc) http.Handler {
		return authenticate(authz.RequirePermission(authz.IotControl)(http.HandlerFunc(handler)))
	}
	mux.Handle("GET /api/iot/devices", read(h.listDevices))
	mux.Handle("POST /api/iot/devices/{id}/approve", manage(h.approve))
	mux.Handle("GET /api/iot/devices/{id}", read(h.getDevice))
	mux.Handle("PATCH /api/iot/devices/{id}", manage(h.updateDevice))
	mux.Handle("DELETE /api/iot/devices/{id}", manage(h.deleteDevice))
	mux.Handle("GET /api/iot/devices/{id}/events", read(h.listEvents))
	mux.Handle("POST /api/iot/devices/{id}/commands", control(h.createCommand))
	mux.Handle("GET /api/iot/commands", manage(h.listCommands))
	mux.Handle("GET /api/iot/stream", read(h.stream))
}

// @Summary      List IoT devices
// @Tags         iot
// @Produce      json
// @Param        q query string false "search by name/external id"
// @Param        status query string false "pending|online|offline"
// @Param        source query string false "filter by source id"
// @Param        category query string false "filter by category"
// @Param        classroom_id query int false "filter by classroom"
// @Param        page query int false "1-based page" default(1)
// @Param        page_size query int false "page size" default(100)
// @Success      200 {object} httpx.Paged "paged devices"
// @Failure      400 {object} httpx.ErrorResponse "invalid filter"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/iot/devices [get]
func (h *Handler) listDevices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := DeviceFilter{
		SourceID: strings.TrimSpace(q.Get("source")),
		Category: strings.TrimSpace(q.Get("category")),
	}
	if s := strings.TrimSpace(q.Get("status")); s != "" {
		if !validDeviceStatuses[s] {
			httpx.RespondError(w, http.StatusBadRequest, "invalid status filter")
			return
		}
		f.Status = s
	}
	if raw := strings.TrimSpace(q.Get("classroom_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			httpx.RespondError(w, http.StatusBadRequest, "invalid classroom_id filter")
			return
		}
		f.ClassroomID = id
	}
	p := httpx.ParsePageParams(q)
	devices, total, err := h.store.PageDevices(r.Context(), f, httpx.ParseSearch(q),
		dbutil.Pagination{Limit: p.PageSize, Offset: p.Offset()})
	if err != nil {
		httpx.Error500(w, r, "could not list devices", err)
		return
	}
	httpx.RespondPaged(w, devices, total, p)
}

// @Summary      Approve (claim) a pending device
// @Tags         iot
// @Accept       json
// @Produce      json
// @Param        id path int true "device id"
// @Param        body body ApproveInput true "claim input"
// @Success      200 {object} Device "approved device"
// @Failure      400 {object} httpx.ErrorResponse "invalid body"
// @Failure      404 {object} httpx.ErrorResponse "device not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/iot/devices/{id}/approve [post]
func (h *Handler) approve(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var in ApproveInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Category = strings.TrimSpace(in.Category)
	if in.Name == "" || len(in.Name) > maxNameLen {
		httpx.RespondError(w, http.StatusBadRequest, "name is required (max 128 chars)")
		return
	}
	if in.ClassroomID <= 0 {
		httpx.RespondError(w, http.StatusBadRequest, "classroomId is required")
		return
	}
	if len(in.Category) > maxCategoryLen {
		httpx.RespondError(w, http.StatusBadRequest, "category too long (max 32 chars)")
		return
	}
	d, err := h.store.ApproveDevice(r.Context(), id, in, h.cfg.OnlineTTL)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not approve device", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("认领物联网设备 %s", displayName(d)))
	h.hub.Broadcast("device.updated", deviceWire(d))
	httpx.RespondJSON(w, http.StatusOK, d)
}

// @Summary      Get an IoT device
// @Tags         iot
// @Produce      json
// @Param        id path int true "device id"
// @Success      200 {object} Device "device detail"
// @Failure      400 {object} httpx.ErrorResponse "invalid device id"
// @Failure      404 {object} httpx.ErrorResponse "device not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/iot/devices/{id} [get]
func (h *Handler) getDevice(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	d, err := h.store.GetDeviceByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not load device", err)
		return
	}
	httpx.RespondJSON(w, http.StatusOK, d)
}

// @Summary      Update an IoT device
// @Tags         iot
// @Accept       json
// @Produce      json
// @Param        id path int true "device id"
// @Param        body body DeviceUpdateInput true "partial update"
// @Success      200 {object} Device "updated device"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / nothing to update"
// @Failure      404 {object} httpx.ErrorResponse "device not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/iot/devices/{id} [patch]
func (h *Handler) updateDevice(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var in DeviceUpdateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" || len(name) > maxNameLen {
			httpx.RespondError(w, http.StatusBadRequest, "name must be 1-128 chars")
			return
		}
		in.Name = &name
	}
	if in.Category != nil {
		category := strings.TrimSpace(*in.Category)
		if category == "" || len(category) > maxCategoryLen {
			httpx.RespondError(w, http.StatusBadRequest, "category must be 1-32 chars")
			return
		}
		in.Category = &category
	}
	if in.ClassroomID != nil && *in.ClassroomID < 0 {
		httpx.RespondError(w, http.StatusBadRequest, "classroomId must be >= 0")
		return
	}
	if in.Name == nil && in.Category == nil && in.ClassroomID == nil {
		httpx.RespondError(w, http.StatusBadRequest, "nothing to update")
		return
	}
	d, err := h.store.UpdateDevice(r.Context(), id, in)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not update device", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("更新物联网设备 %s", displayName(d)))
	h.hub.Broadcast("device.updated", deviceWire(d))
	httpx.RespondJSON(w, http.StatusOK, d)
}

// @Summary      Delete an IoT device
// @Tags         iot
// @Param        id path int true "device id"
// @Success      204 "no content"
// @Failure      400 {object} httpx.ErrorResponse "invalid device id"
// @Failure      404 {object} httpx.ErrorResponse "device not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/iot/devices/{id} [delete]
func (h *Handler) deleteDevice(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	existing, err := h.store.GetDeviceByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not load device", err)
		return
	}
	if err := h.store.DeleteDevice(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.RespondError(w, http.StatusNotFound, "device not found")
			return
		}
		httpx.Error500(w, r, "could not delete device", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("删除物联网设备 %s", displayName(existing)))
	w.WriteHeader(http.StatusNoContent)
}

// @Summary      List a device's events
// @Tags         iot
// @Produce      json
// @Param        id path int true "device id"
// @Param        page query int false "1-based page" default(1)
// @Param        page_size query int false "page size" default(100)
// @Success      200 {object} httpx.Paged "paged events, newest first"
// @Failure      400 {object} httpx.ErrorResponse "invalid device id"
// @Failure      404 {object} httpx.ErrorResponse "device not found"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/iot/devices/{id}/events [get]
func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if _, err := h.store.GetDeviceByID(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.RespondError(w, http.StatusNotFound, "device not found")
			return
		}
		httpx.Error500(w, r, "could not load device", err)
		return
	}
	p := httpx.ParsePageParams(r.URL.Query())
	events, total, err := h.store.PageEvents(r.Context(), id,
		dbutil.Pagination{Limit: p.PageSize, Offset: p.Offset()})
	if err != nil {
		httpx.Error500(w, r, "could not list events", err)
		return
	}
	httpx.RespondPaged(w, events, total, p)
}

// @Summary      Issue a command to a device
// @Tags         iot
// @Accept       json
// @Produce      json
// @Param        id path int true "device id"
// @Param        body body CommandInput true "command input"
// @Success      201 {object} DeviceCommand "created command (delivered when the broker accepted it)"
// @Failure      400 {object} httpx.ErrorResponse "invalid body / unknown command type"
// @Failure      404 {object} httpx.ErrorResponse "device not found"
// @Failure      409 {object} httpx.ErrorResponse "device is pending approval"
// @Failure      502 {object} httpx.ErrorResponse "could not publish command"
// @Failure      503 {object} httpx.ErrorResponse "iot messaging not configured"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/iot/devices/{id}/commands [post]
func (h *Handler) createCommand(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if h.publisher == nil {
		httpx.RespondError(w, http.StatusServiceUnavailable, "iot messaging is not configured")
		return
	}
	var in CommandInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !CommandTypes[in.Type] {
		httpx.RespondError(w, http.StatusBadRequest, "unknown command type")
		return
	}
	if len(in.Payload) > 0 && !isJSONObject(in.Payload) {
		httpx.RespondError(w, http.StatusBadRequest, "payload must be a JSON object")
		return
	}
	d, err := h.store.GetDeviceByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not load device", err)
		return
	}
	if d.Status == StatusPending {
		httpx.RespondError(w, http.StatusConflict, "device is pending approval")
		return
	}
	subject, ok := authz.SubjectFrom(r.Context())
	if !ok {
		httpx.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	cmd, err := h.store.CreateCommand(r.Context(), d.ID, in.Type, in.Payload,
		subject.Username, ClampCommandTTL(in.ExpiresInSeconds))
	if err != nil {
		httpx.Error500(w, r, "could not create command", err)
		return
	}
	if err := h.publisher.PublishCommand(r.Context(), d, cmd); err != nil {
		_ = h.store.FailCommand(r.Context(), cmd.CommandID, "publish failed")
		httpx.Error500(w, r, "could not publish command", err)
		return
	}
	cmd, err = h.store.GetCommandByID(r.Context(), cmd.CommandID)
	if err != nil {
		httpx.Error500(w, r, "could not record command delivery", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("下发设备命令 %s → %s", cmd.Type, displayName(d)))
	h.hub.Broadcast("command.updated", map[string]any{"commandId": cmd.CommandID, "status": cmd.Status})
	httpx.RespondJSON(w, http.StatusCreated, cmd)
}

// @Summary      List issued commands
// @Tags         iot
// @Produce      json
// @Param        status query string false "queued|delivered|acked|failed|expired"
// @Param        device_id query int false "filter by device"
// @Param        page query int false "1-based page" default(1)
// @Param        page_size query int false "page size" default(100)
// @Success      200 {object} httpx.Paged "paged commands, newest first"
// @Failure      400 {object} httpx.ErrorResponse "invalid filter"
// @Failure      500 {object} httpx.ErrorResponse "internal error"
// @Security     BearerAuth
// @Router       /api/iot/commands [get]
func (h *Handler) listCommands(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := CommandFilter{}
	if s := strings.TrimSpace(q.Get("status")); s != "" {
		if !validCommandStatuses[s] {
			httpx.RespondError(w, http.StatusBadRequest, "invalid status filter")
			return
		}
		f.Status = s
	}
	if raw := strings.TrimSpace(q.Get("device_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			httpx.RespondError(w, http.StatusBadRequest, "invalid device_id filter")
			return
		}
		f.DeviceID = id
	}
	p := httpx.ParsePageParams(q)
	cmds, total, err := h.store.PageCommands(r.Context(), f,
		dbutil.Pagination{Limit: p.PageSize, Offset: p.Offset()})
	if err != nil {
		httpx.Error500(w, r, "could not list commands", err)
		return
	}
	httpx.RespondPaged(w, cmds, total, p)
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.RespondError(w, http.StatusBadRequest, "invalid device id")
		return 0, false
	}
	return id, true
}

func displayName(d Device) string {
	if d.Name != "" {
		return d.Name
	}
	return d.ExternalID
}

// deviceWire is the broadcast payload for device SSE events (a trimmed view —
// the full record is one GET away, frames should stay small).
func deviceWire(d Device) map[string]any {
	m := map[string]any{
		"id":        d.ID,
		"site":      d.Site,
		"sourceId":  d.SourceID,
		"deviceId":  d.ExternalID,
		"name":      d.Name,
		"category":  d.Category,
		"status":    d.Status,
		"updatedAt": d.UpdatedAt.UnixMilli(),
	}
	if d.ClassroomID != nil {
		m["classroomId"] = *d.ClassroomID
	}
	if d.LastSeenAt != nil {
		m["lastSeenAt"] = d.LastSeenAt.UnixMilli()
	}
	return m
}

// isJSONObject reports whether b is valid JSON encoding of an object (nil is
// treated as absent, i.e. fine).
func isJSONObject(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	var v map[string]json.RawMessage
	return json.Unmarshal(b, &v) == nil
}
