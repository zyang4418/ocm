---
title: 后端
---

Go 后端(`module ocm-backend`,Go 1.26)。`net/http` ServeMux(Go 1.22+ 方法路由,如 `mux.Handle("GET /api/...", ...)`) + MySQL 8。鉴权 JWT(HS256、24h),小程序身份经后端 `code2Session` 解析 openid。权限链 `auth.Middleware` → `user.LoadSubject` → `authz.RequirePermission`。HTTP 契约见 [API 概述](/api)。

## 入口与运行

`backend/main.go` 启动顺序:

1. 注册 `/healthz`(**先于开库**;反映进程存活,不查 DB)。
2. 在 goroutine 里起 HTTP 服务(端口 `PORT`,默认 8080)——端口立即绑定。
3. 开 MySQL(`openDB` → `db.ConfigFromEnv` + `db.New`,带 60s 连接超时 + `pingWithRetry` 每 2s 重试)。
4. 依次跑各 store 的 `Migrate(ctx)`:auth → user → classroom → schedule → course → booking → importer。
5. 组装一次鉴权中间件 `authenticate`,各 handler 用 `RegisterRoutes(mux, authenticate)` 注册路由。
6. 构建 importer `Registry`(job 类型 → 权限 → importer),调 `RecoverStale`(恢复崩溃的 processing / 重排队 pending)。
7. 注册 `/readyz`(**开库后**;ping DB,3s 超时,不可达返回 503)。
8. 等 SIGINT/SIGTERM,graceful shutdown(10s 超时)。`httpx.Recover` 包裹 mux,把 panic 转 JSON 500。

三种本地跑法:

| 方式 | 命令 | 说明 |
|------|------|------|
| 最简 | `cd backend && go run .` | 需 MySQL 可达(环境变量或默认值) |
| 热更 | `cd backend && air` | `.air.toml` 配置;`Dockerfile.dev` 即用此 |
| 全栈 | `docker compose up -d`(仓库根) | mysql + backend(air) + web(vite) 一起起 |

`go run .` 需 MySQL 可达;`APP_ENV=development` 时允许 `DB_HOST` 缺省回落 localhost,生产模式(`APP_ENV=production`,默认)则**拒绝**静默回落 localhost。

## 路由组织

业务路由前缀 `/api`,Go 1.22+ 方法路由。按资源分组:auth、users、admin-classes、teaching-classes、classrooms、schedule(regimes/periods)、course(catalog/offerings/sessions/timetable)、bookings、imports。完整路由表(方法+路径+权限)见 [API 概述](/api)。

## 鉴权

**JWT** —— `internal/auth/token.go`:HS256;密钥取 `JWT_SECRET`(空则回落开发串 `ocm-dev-secret-do-not-use-in-production` 并告警);TTL 24h;签发 issuer `ocm-backend`、subject = 用户名;`Parse` 先校验签名方法是 HMAC 再用密钥。

**小程序身份(code2Session)** —— `internal/auth/wx.go`:`WxService` 读 `WX_APP_ID`/`WX_APP_SECRET`(仅在后端 env)。调 `https://api.weixin.qq.com/sns/jscode2session` 用一次性 `code` 换稳定 openid。**openid 绝不取自 `X-WX-OPENID` 头**——后端因网页端必须公网暴露,该头可伪造。

**绑定规则** —— `internal/auth/store.go`:`BindOpenid` 执行 `UPDATE users SET openid=? WHERE id=? AND openid IS NULL`(openid 必须为 NULL 才允许绑定,防劫持);`UNIQUE INDEX idx_users_openid(openid)` 防一个 openid 绑多账号(冲突 → MySQL 1062 → `ErrOpenidTaken`);多个 NULL 共存,故未绑账号可并存。`UnbindOpenid` 置 NULL;`GetByOpenid` 用于静默登录,未绑返回 `ErrNotBound`。

**中间件链** —— `main.go` 组装:

```go
authenticate := auth.Middleware(tokenService)(user.LoadSubject(userStore))
// 业务 handler 再包: authz.RequirePermission(perm)
```

1. `auth.Middleware`:读 `Authorization: Bearer <token>`,解析出 username 存入 context;缺/错返回 401。
2. `user.LoadSubject`:**每次请求**重查 user 记录(角色变更即时生效),存 `authz.Subject{ID, Role}`;账号已删返回 401。
3. `authz.RequirePermission(perm)`:校验 `Can(role, perm)`,不足返回 403。

仅鉴权、不限权限的路由(`/api/auth/me`、`/api/auth/wx-unbind`)用 `auth.Middleware` 直接包,不走 `LoadSubject`/`RequirePermission`。

## 权限模型

`internal/authz/authz.go`:

- handler 检查的是 **permission 字符串**(如 `classroom:manage`),**不是角色名**——role→permission 映射可变而无需改 handler。
- **admin 通配**:`role == "admin"` 时 `Can(...)` 对一切返回 true。
- `user` 角色仅得:`classroom:read`、`course:read`、`classroom:book`、`repair:create`、`admin_class:read`、`teaching_class:read`(可读教室/课程/作息、可预约教室、可提报修;不可管理任何资源)。
- 角色仅 `"admin"` / `"user"` 两值(user CRUD 强校验)。
- `Subject` 鉴权无关(JWT 网页端与 openid 小程序同形);后续阶段会加按用户附加权限 / 映射入 DB。

## 数据库与迁移

驱动 `github.com/go-sql-driver/mysql`,MySQL 8。

- **无独立迁移目录/工具**:每个 store 有幂等 `Migrate(ctx)`,启动时调用。用 `CREATE TABLE IF NOT EXISTS` + **幂等 ALTER**:`ALTER TABLE ADD COLUMN/INDEX`,若报错则断言为 MySQL 1060(重复列)或 1061(重复键)并忽略,其他错误致命。1062(重复行)由 `dbutil.IsDuplicateEntry` 在插入路径处理。共享 helper 在 `internal/dbutil`。
- 主要表:`users`、`classrooms`、`schedule_regimes`/`schedule_periods`、`course_catalog`/`course_offerings`/`course_sessions`、`admin_classes`、`teaching_classes`/`teaching_class_members`、`classroom_bookings`、`import_jobs`。
- 连接:`db.ConfigFromEnv` 出 DSN(`charset=utf8mb4&parseTime=true&loc=Local`);`db.New` 生产拒绝 localhost 回落(`db.go`),`pingWithRetry` 轮询至 DB 就绪;连接池默认 25 open / 25 idle / 5min lifetime(可调 `DB_*_CONNS`/`DB_CONN_MAX_LIFETIME`)。

## 异步导入

`internal/importer`:job 状态机——`pending → processing → preview →(commit)→ succeeded/failed`,`cancelled`。

- 上传建 job(文件 base64 入 `import_jobs.payload`)→ `processJob` 做 `Analyze` 干跑,存 `preview` → 状态 `preview`。
- **commit**:`POST /api/imports/{id}/commit` 从原始 payload 重新校验(非用 preview 缓存)、一个事务插入有效行 → `succeeded`(部分行失败仍 succeeded,记 `errorReport`)。
- cancel 丢弃 preview。状态迁移是原子条件更新(按 `fromStatus`,竞态败返回 `ErrJobStateConflict`)。启动 `RecoverStale` 恢复崩溃 job。
- `internal/xlsx` 用 `excelize` 解析,**按列名(小写化+trim)映射,列序无关,空行跳过**。
- `POST /api/imports/jwc_split`:一个教务处聚合 xlsx + `semester` + `week1_monday` → 原子批拆成 6 个 job(教室→课程库→行政班→教学班→开课→课次,按依赖序)。

## 环境变量

| 变量 | 默认 | 用途 |
|------|------|------|
| `PORT` | `8080` | HTTP 端口 |
| `APP_ENV` | `production` | `development` 允许 localhost DB 回落及开发行为 |
| `DB_HOST` | `localhost` | MySQL 主机;**生产必填** |
| `DB_PORT` | `3306` | MySQL 端口 |
| `DB_USER` / `DB_PASSWORD` / `DB_NAME` | `ocm` / `ocm` / `ocm` | 库账号 |
| `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` / `DB_CONN_MAX_LIFETIME` | `25` / `25` / `5m` | 连接池 |
| `JWT_SECRET` | 开发兜底串 | HS256 密钥;生产必填 |
| `ADMIN_PASSWORD` | `admin123` | 首次启动(users 表空)种入 admin 账号的密码;生产必填 |
| `WX_APP_ID` / `WX_APP_SECRET` | 空 | 小程序 code2Session;两者都设才启用 wx 登录,web-only 可留空 |

## Docker

- **`Dockerfile`**(生产,多阶段):`golang:1.26-alpine` 构建(`CGO_ENABLED=0`,`-ldflags="-s -w"`)→ `alpine:3.20` 运行,non-root `appuser`,`EXPOSE 8080`。
- **`Dockerfile.dev`**:`golang:1.26-alpine`,装 `air`,`EXPOSE 8080`,`CMD ["air"]`;源码由 compose 挂载热更。
- compose:`docker-compose.yml`(dev,backend 用 `Dockerfile.dev` + `command: air`,`APP_ENV=development`)、`docker-compose.prod.yml`(prod,backend 用 `Dockerfile`,对 `DB_PASSWORD`/`JWT_SECRET`/`ADMIN_PASSWORD`/`MYSQL_ROOT_PASSWORD` 用 `${VAR:?...}` fail-fast)。

## 并发与冲突(要点)

- **预约**:并发 `Create` 锁 `classrooms` 行 `FOR UPDATE`(以教室行为锁锚,序列化同槽位竞争)。
- **教学班成员**:被开课引用后冻结;`teachingClassInUse` 用 `SELECT COUNT(*) ... FOR UPDATE` 在 REPEATABLE READ 下 gap-lock,关闭 check-then-write TOCTOU。
- **预约冲突模型**:与任一节次区间 `[period_start, period_end]` 重叠的 `course_session`、或任一重叠区间的活跃预约冲突;sessions 与 bookings 同为 `(classroom_id, date, 节次区间)` 模型,双向互查。
- **预约状态机**:`pending → approved/rejected`(review,admin);`pending/approved → cancelled`(cancel,预约人或 admin);仅 `pending`+`approved` 占槽位并参与冲突检测。

## 导出 ↔ 导入回环

每个资源的 `GET /api/{resource}/export` 流式输出 xlsx,表头与对应 importer 的必需列对齐,故导出文件可直接回导。
