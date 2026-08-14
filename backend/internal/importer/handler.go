package importer

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ocm-backend/internal/authz"
	"ocm-backend/internal/dbutil"
	"ocm-backend/internal/httpx"
	"ocm-backend/internal/importer/jwc"
	"ocm-backend/internal/logging"
	"ocm-backend/internal/schedule"
	"ocm-backend/internal/systemlog"
)

// maxUploadBytes caps the xlsx upload size. Imports are bounded text data; this
// guards against accidental large uploads through the edge proxy.
const maxUploadBytes = 5 << 20

type Handler struct {
	store         *Store
	registry      *Registry
	scheduleStore *schedule.Store // jwc_split needs regimes to pre-validate expanded dates
	logs          *systemlog.Store
	sem           chan struct{} // limits concurrent import processing
}

func NewHandler(store *Store, registry *Registry, scheduleStore *schedule.Store, logStore *systemlog.Store) *Handler {
	return &Handler{
		store:         store,
		registry:      registry,
		scheduleStore: scheduleStore,
		logs:          logStore,
		sem:           make(chan struct{}, 2),
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
	// jwc_split is registered before the {type} wildcard; Go 1.22 ServeMux gives
	// the literal segment precedence over the wildcard, so POST /api/imports/jwc_split
	// routes here while POST /api/imports/classrooms still routes to upload.
	mux.Handle("POST /api/imports/jwc_split", authOnly(h.jwcSplit))
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
		logging.L.Error("importer: recover stale jobs", "err", err)
		return
	}
	for _, id := range pendingIDs {
		go h.processJob(id)
	}
	if len(pendingIDs) > 0 {
		logging.L.Info("importer: requeued pending jobs", "count", len(pendingIDs))
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
	if !subject.Has(perm) {
		httpx.RespondError(w, http.StatusForbidden, "insufficient permissions")
		return false
	}
	return true
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := httpx.ParsePageParams(q)
	jobs, total, err := h.store.PageJobs(r.Context(), httpx.ParseSearch(q),
		dbutil.Pagination{Limit: p.PageSize, Offset: p.Offset()})
	if err != nil {
		httpx.Error500(w, r, "could not list import jobs", err)
		return
	}
	if jobs == nil {
		jobs = []Job{}
	}
	httpx.RespondPaged(w, jobs, total, p)
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
		httpx.Error500(w, r, "could not load import job", err)
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
		httpx.Error500(w, r, "could not create import job", err)
		return
	}
	// Process detached from the request so the client is not held open (the
	// edge proxy would otherwise time out on large/slow imports).
	go h.processJob(job.ID)
	systemlog.WithSummary(r.Context(), fmt.Sprintf("上传导入文件 %s（%s）", header.Filename, typ))
	httpx.RespondJSON(w, http.StatusAccepted, map[string]any{"id": job.ID, "status": job.Status})
}

// jwcSplit accepts a 教务处 aggregated schedule xlsx plus semester label + the
// Monday of week 1, splits it into 6 canonical xlsx via jwc.Split, and creates
// one import job per output in dependency order. Each job is processed (dry-run
// Analyze → preview) detached from the request, exactly like a manual upload.
//
// The 6 jobs are independent at preview time, but offerings/sessions reference
// catalog/teaching_classes by name, so their previews will show "课程不存在 /
// 教学班不存在" errors until the operator commits classrooms → catalog →
// admin_classes → teaching_classes first. Commit re-validates from the raw
// payload (see runCommit), so committing in dependency order still succeeds;
// the stale preview is informational only. This matches the existing manual
// multi-file import flow.
//
// Fatal split errors (bad params, unparseable xlsx, regime not covering an
// expanded date) are returned as 400 without creating any job, so the operator
// can fix inputs and retry without orphan jobs.
func (h *Handler) jwcSplit(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, authz.CourseManage) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "无效的上传或文件过大（上限 5MB）")
		return
	}
	semester := strings.TrimSpace(r.FormValue("semester"))
	week1Str := strings.TrimSpace(r.FormValue("week1_monday"))
	if semester == "" || week1Str == "" {
		httpx.RespondError(w, http.StatusBadRequest, "缺少 semester 或 week1_monday 参数")
		return
	}
	week1Monday, err := time.Parse("2006-01-02", week1Str)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "week1_monday 格式应为 YYYY-MM-DD")
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
	// Load regimes once for pre-validation of every expanded session date.
	regimes, err := h.scheduleStore.ListRegimes(r.Context())
	if err != nil {
		httpx.Error500(w, r, "加载作息制度失败", err)
		return
	}
	res, err := jwc.Split(data, semester, week1Monday, regimes)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "拆分失败："+err.Error())
		return
	}

	// 6 jobs in dependency order: base data first, then offerings, then sessions.
	// Filenames carry the source stem + type so the operator can group them in
	// the job list (e.g. 列表课程信息_classrooms.xlsx).
	stem := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	if stem == "" {
		stem = "jwc"
	}
	specs := []struct {
		typ string
		b   []byte
	}{
		{JobTypeClassrooms, res.Files.Classrooms},
		{JobTypeCatalog, res.Files.Catalog},
		{JobTypeAdminClasses, res.Files.AdminClasses},
		{JobTypeTeachingClasses, res.Files.TeachingClasses},
		{JobTypeOfferings, res.Files.Offerings},
		{JobTypeSessions, res.Files.Sessions},
	}
	jobSpecs := make([]JobSpec, 0, len(specs))
	for _, s := range specs {
		jobSpecs = append(jobSpecs, JobSpec{
			Type:     s.typ,
			Filename: stem + "_" + s.typ + ".xlsx",
			Payload:  base64.StdEncoding.EncodeToString(s.b),
		})
	}
	created, err := h.store.CreateJobs(r.Context(), subject.ID, jobSpecs)
	if err != nil {
		httpx.Error500(w, r, "创建导入任务失败", err)
		return // atomic batch: no jobs created, no goroutines started
	}
	jobs := make([]map[string]any, 0, len(created))
	for _, j := range created {
		jobs = append(jobs, map[string]any{"id": j.ID, "type": j.Type, "status": j.Status})
		go h.processJob(j.ID) // all rows committed before any worker starts
	}
	warnings := res.Stats.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("上传教务处课表 %s 并拆分为 6 个导入任务", header.Filename))
	httpx.RespondJSON(w, http.StatusAccepted, map[string]any{
		"jobs":     jobs,
		"stats":    res.Stats,
		"warnings": warnings,
	})
}

// processJob runs the dry-run parse/validate for one job and stores the result
// as a preview awaiting the user's commit. It uses a background context so it is
// not canceled when the uploading request completes.
func (h *Handler) processJob(id int64) {
	h.sem <- struct{}{}
	defer func() {
		if r := recover(); r != nil {
			logging.L.Error("importer: panic processing job", "job_id", id, "err", fmt.Sprint(r))
			h.finishFail(context.Background(), id, "处理任务时发生内部错误")
		}
		<-h.sem
	}()

	ctx := context.Background()
	if err := h.store.MarkProcessing(ctx, id, StatusPending); err != nil {
		if errors.Is(err, ErrJobStateConflict) {
			// A concurrent worker (e.g. a RecoverStale requeue of a job whose
			// original processJob is still alive) already moved this job out of
			// pending; leave it to that worker.
			return
		}
		logging.L.Error("importer: mark processing job", "job_id", id, "err", err)
		h.finishFail(ctx, id, "标记处理中失败："+err.Error())
		return
	}
	job, err := h.store.GetJob(ctx, id)
	if err != nil {
		logging.L.Error("importer: load job", "job_id", id, "err", err)
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
			logging.L.Error("importer: finish failed job", "job_id", id, "err", ferr)
		}
		return
	}
	// Even with zero valid rows we go to preview so the operator can see the
	// per-row failures and decide whether to cancel or fix and re-upload.
	if err := h.store.SavePreview(ctx, id, result); err != nil {
		logging.L.Error("importer: save preview job", "job_id", id, "err", err)
		h.finishFail(ctx, id, "保存预览失败："+err.Error())
	}
}

// runCommit performs the actual write for a job the user confirmed. It
// re-validates from the stored payload (not the preview rows) because database
// state may have changed since the preview.
func (h *Handler) runCommit(id int64) {
	h.sem <- struct{}{}
	defer func() {
		if r := recover(); r != nil {
			logging.L.Error("importer: panic committing job", "job_id", id, "err", fmt.Sprint(r))
			h.finishFail(context.Background(), id, "提交任务时发生内部错误")
			// The commit is the actual data write; record its failure even
			// though the request that started it has long since answered.
			h.logs.Record(context.Background(), systemlog.Entry{
				Method: http.MethodPost, Path: "/api/imports/" + strconv.FormatInt(id, 10) + "/commit",
				StatusCode: http.StatusInternalServerError,
				Summary:    fmt.Sprintf("导入失败：任务 #%d", id)})
		}
		<-h.sem
	}()

	ctx := context.Background()
	job, err := h.store.GetJob(ctx, id)
	if err != nil {
		logging.L.Error("importer: load job for commit", "job_id", id, "err", err)
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
		logging.L.Error("importer: finish commit job", "job_id", id, "err", err)
	}
	// The commit is the actual data write; it happens after the request that
	// started it has answered 202, so it gets its own audit row (the one
	// deliberate two-rows-per-request exception).
	summary := fmt.Sprintf("导入完成：%s（成功 %d 行，失败 %d 行）", job.Filename, result.SucceededRows, result.FailedRows)
	code := http.StatusOK
	if status == StatusFailed {
		summary = fmt.Sprintf("导入失败：%s（成功 %d 行，失败 %d 行）", job.Filename, result.SucceededRows, result.FailedRows)
		code = http.StatusInternalServerError
	}
	h.logs.Record(context.Background(), systemlog.Entry{
		ActorID: job.UserID, Method: http.MethodPost,
		Path:       "/api/imports/" + strconv.FormatInt(id, 10) + "/commit",
		StatusCode: code, Summary: summary})
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
		httpx.Error500(w, r, "could not load import job", err)
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
		httpx.Error500(w, r, "could not start commit", err)
		return
	}
	go h.runCommit(id)
	systemlog.WithSummary(r.Context(), fmt.Sprintf("提交导入任务 %s", job.Filename))
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
		httpx.Error500(w, r, "could not load import job", err)
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
		httpx.Error500(w, r, "could not cancel import job", err)
		return
	}
	systemlog.WithSummary(r.Context(), fmt.Sprintf("取消导入任务 %s", job.Filename))
	httpx.RespondJSON(w, http.StatusOK, map[string]any{"id": job.ID, "status": StatusCancelled})
}

func (h *Handler) finishFail(ctx context.Context, id int64, msg string) {
	res := Result{Errors: []RowError{{Row: 0, Error: msg}}}
	if err := h.store.Finish(ctx, id, StatusFailed, res); err != nil {
		logging.L.Error("importer: finish failed job", "job_id", id, "err", err)
	}
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}
