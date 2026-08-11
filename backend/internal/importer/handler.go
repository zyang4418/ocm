package importer

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/httpx"
)

// maxUploadBytes caps the xlsx upload size. Imports are bounded text data; this
// guards against accidental large uploads through the edge proxy.
const maxUploadBytes = 5 << 20

type Handler struct {
	store    *Store
	registry *Registry
	sem      chan struct{} // limits concurrent import processing
}

func NewHandler(store *Store, registry *Registry) *Handler {
	return &Handler{
		store:    store,
		registry: registry,
		sem:      make(chan struct{}, 2),
	}
}

// RegisterRoutes mounts the import endpoints. Upload/commit/cancel each enforce
// the type-specific permission looked up from the Registry (so importing
// classrooms needs classroom:manage, sessions needs course:manage, etc.); list
// and get require only authentication.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	authOnly := func(handler http.HandlerFunc) http.Handler {
		return authenticate(http.HandlerFunc(handler))
	}
	mux.Handle("POST /api/imports/{type}", authOnly(h.upload))
	mux.Handle("GET /api/imports", authOnly(h.list))
	mux.Handle("GET /api/imports/{id}", authOnly(h.get))
	mux.Handle("POST /api/imports/{id}/commit", authOnly(h.commitJob))
	mux.Handle("POST /api/imports/{id}/cancel", authOnly(h.cancelJob))
}

// RecoverStale is called once at startup: jobs left "processing" by a crashed
// process are marked failed, and "pending" jobs that never started are
// requeued. Safe to call before serving traffic.
func (h *Handler) RecoverStale(ctx context.Context) {
	pendingIDs, err := h.store.RecoverStale(ctx)
	if err != nil {
		log.Printf("importer: recover stale jobs: %v", err)
		return
	}
	for _, id := range pendingIDs {
		go h.processJob(id)
	}
	if len(pendingIDs) > 0 {
		log.Printf("importer: requeued %d pending job(s)", len(pendingIDs))
	}
}

// checkPerm verifies the subject has perm, writing a 401/403 and returning false
// otherwise. Centralized so upload/commit/cancel share one permission path.
func (h *Handler) checkPerm(w http.ResponseWriter, r *http.Request, perm string) bool {
	subject, ok := authz.SubjectFrom(r.Context())
	if !ok {
		httpx.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return false
	}
	if !authz.Can(subject.Role, perm) {
		httpx.RespondError(w, http.StatusForbidden, "insufficient permissions")
		return false
	}
	return true
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.store.ListJobs(r.Context(), 50)
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not list import jobs")
		return
	}
	if jobs == nil {
		jobs = []Job{}
	}
	httpx.RespondJSON(w, http.StatusOK, jobs)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid job id")
		return
	}
	job, err := h.store.GetJob(r.Context(), id)
	if errors.Is(err, ErrJobNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "import job not found")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not load import job")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, job)
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	typ := r.PathValue("type")
	_, perm, ok := h.registry.Lookup(typ)
	if !ok {
		httpx.RespondError(w, http.StatusNotFound, "未知的导入类型")
		return
	}
	if !h.checkPerm(w, r, perm) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "无效的上传或文件过大（上限 5MB）")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "缺少 file 字段")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(file)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "could not read uploaded file")
		return
	}
	subject, ok := authz.SubjectFrom(r.Context())
	if !ok {
		httpx.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	// Payload is the binary xlsx, base64-encoded so it survives the TEXT column
	// used by import_jobs.payload (see parseWorkbook).
	payload := base64.StdEncoding.EncodeToString(data)
	job, err := h.store.CreateJob(r.Context(), typ, header.Filename, payload, subject.ID)
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not create import job")
		return
	}
	// Process detached from the request so the client is not held open (the
	// edge proxy would otherwise time out on large/slow imports).
	go h.processJob(job.ID)
	httpx.RespondJSON(w, http.StatusAccepted, map[string]any{"id": job.ID, "status": job.Status})
}

// processJob runs the dry-run parse/validate for one job and stores the result
// as a preview awaiting the user's commit. It uses a background context so it is
// not canceled when the uploading request completes.
func (h *Handler) processJob(id int64) {
	h.sem <- struct{}{}
	defer func() { <-h.sem }()

	ctx := context.Background()
	if err := h.store.MarkProcessing(ctx, id, StatusPending); err != nil {
		if errors.Is(err, ErrJobStateConflict) {
			// A concurrent worker (e.g. a RecoverStale requeue of a job whose
			// original processJob is still alive) already moved this job out of
			// pending; leave it to that worker.
			return
		}
		log.Printf("importer: mark processing job %d: %v", id, err)
		h.finishFail(ctx, id, "标记处理中失败："+err.Error())
		return
	}
	job, err := h.store.GetJob(ctx, id)
	if err != nil {
		log.Printf("importer: load job %d: %v", id, err)
		h.finishFail(ctx, id, "加载任务失败："+err.Error())
		return
	}

	imp, _, ok := h.registry.Lookup(job.Type)
	if !ok {
		h.finishFail(ctx, id, "未知的导入类型："+job.Type)
		return
	}

	result, err := imp.Analyze(ctx, job.Payload)
	if err != nil {
		if ferr := h.store.Finish(ctx, id, StatusFailed, result); ferr != nil {
			log.Printf("importer: finish failed job %d: %v", id, ferr)
		}
		return
	}
	// Even with zero valid rows we go to preview so the operator can see the
	// per-row failures and decide whether to cancel or fix and re-upload.
	if err := h.store.SavePreview(ctx, id, result); err != nil {
		log.Printf("importer: save preview job %d: %v", id, err)
		h.finishFail(ctx, id, "保存预览失败："+err.Error())
	}
}

// runCommit performs the actual write for a job the user confirmed. It
// re-validates from the stored payload (not the preview rows) because database
// state may have changed since the preview.
func (h *Handler) runCommit(id int64) {
	h.sem <- struct{}{}
	defer func() { <-h.sem }()

	ctx := context.Background()
	job, err := h.store.GetJob(ctx, id)
	if err != nil {
		log.Printf("importer: load job %d for commit: %v", id, err)
		h.finishFail(ctx, id, "加载任务失败："+err.Error())
		return
	}

	imp, _, ok := h.registry.Lookup(job.Type)
	if !ok {
		h.finishFail(ctx, id, "未知的导入类型："+job.Type)
		return
	}

	result, err := imp.Commit(ctx, job.Payload)
	status := StatusSucceeded
	if err != nil || result.SucceededRows == 0 {
		status = StatusFailed
	}
	if err := h.store.Finish(ctx, id, status, result); err != nil {
		log.Printf("importer: finish commit job %d: %v", id, err)
	}
}

// commitJob transitions a previewed job into processing and kicks off the
// asynchronous write. The status and permission checks are synchronous so a
// stale or unauthorized job is rejected before any work starts.
func (h *Handler) commitJob(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid job id")
		return
	}
	job, err := h.store.GetJob(r.Context(), id)
	if errors.Is(err, ErrJobNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "import job not found")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not load import job")
		return
	}
	_, perm, ok := h.registry.Lookup(job.Type)
	if !ok {
		httpx.RespondError(w, http.StatusNotFound, "未知的导入类型")
		return
	}
	if !h.checkPerm(w, r, perm) {
		return
	}
	if job.Status != StatusPreview {
		httpx.RespondError(w, http.StatusConflict, "该任务不在待确认状态")
		return
	}
	if err := h.store.MarkProcessing(r.Context(), id, StatusPreview); err != nil {
		if errors.Is(err, ErrJobStateConflict) {
			httpx.RespondError(w, http.StatusConflict, "该任务不在待确认状态")
			return
		}
		httpx.RespondError(w, http.StatusInternalServerError, "could not start commit")
		return
	}
	go h.runCommit(id)
	httpx.RespondJSON(w, http.StatusAccepted, map[string]any{"id": job.ID, "status": StatusProcessing})
}

// cancelJob discards a previewed job without committing.
func (h *Handler) cancelJob(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid job id")
		return
	}
	job, err := h.store.GetJob(r.Context(), id)
	if errors.Is(err, ErrJobNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "import job not found")
		return
	}
	if err != nil {
		httpx.RespondError(w, http.StatusInternalServerError, "could not load import job")
		return
	}
	_, perm, ok := h.registry.Lookup(job.Type)
	if !ok {
		httpx.RespondError(w, http.StatusNotFound, "未知的导入类型")
		return
	}
	if !h.checkPerm(w, r, perm) {
		return
	}
	if job.Status != StatusPreview {
		httpx.RespondError(w, http.StatusConflict, "该任务不在待确认状态")
		return
	}
	if err := h.store.Cancel(r.Context(), id); err != nil {
		if errors.Is(err, ErrJobStateConflict) {
			httpx.RespondError(w, http.StatusConflict, "该任务不在待确认状态")
			return
		}
		httpx.RespondError(w, http.StatusInternalServerError, "could not cancel import job")
		return
	}
	httpx.RespondJSON(w, http.StatusOK, map[string]any{"id": job.ID, "status": StatusCancelled})
}

func (h *Handler) finishFail(ctx context.Context, id int64, msg string) {
	res := Result{Errors: []RowError{{Row: 0, Error: msg}}}
	if err := h.store.Finish(ctx, id, StatusFailed, res); err != nil {
		log.Printf("importer: finish failed job %d: %v", id, err)
	}
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}
