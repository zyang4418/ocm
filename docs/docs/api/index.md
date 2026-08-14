---
title: API 概述
---

后端 HTTP API。所有业务路由前缀 `/api`,Go 1.22+ ServeMux 方法路由。鉴权 `Authorization: Bearer <JWT>`(HS256、24h)。角色 `admin`(通配放行)/ `user`(只读 + 预约 + 报修)。后端实现细节见 [后端](/guide/backend)。

## 鉴权与权限

- 公开端点(无需登录):`POST /api/auth/login`、`POST /api/auth/wx-bind`、`POST /api/auth/wx-login`。
- 仅需登录(不限权限):`GET /api/auth/me`、`POST /api/auth/wx-unbind`、`GET /api/imports`、`GET /api/imports/{id}`。
- 其余写操作按 permission 校验;`admin` 全通配,`user` 仅 `*:read` + `classroom:book` + `repair:create`。权限检查基于 permission 字符串,不基于角色名。

## 分页与搜索

所有列表接口(users / admin-classes / teaching-classes / classrooms / courses / offerings / sessions / bookings / regimes / imports)均返回**分页信封**并支持模糊搜索:

```json
{ "items": [ ... ], "total": 1234, "page": 1, "pageSize": 100 }
```

| 参数 | 默认 | 说明 |
|------|------|------|
| `page` | `1` | 页码,≥1(非数字或 <1 回退 1) |
| `page_size` | `100` | 每页行数,钳制 `1..500` |
| `q` | 空 | 模糊搜索词(LIKE contains,`%`/`_`/`\` 已转义,逐字匹配) |

`total` 为全部匹配行数(跨页),`items` 永不返回 `null`(空列表为 `[]`)。各资源可搜索字段:

| 资源 | 搜索字段 |
|------|------|
| users | username、display_name |
| admin-classes | name、grade |
| teaching-classes | name |
| classrooms | name、building |
| courses(课程库) | name、code |
| offerings | 课程名/代码、教学班名、教师、学期 |
| sessions | 课程名/代码、教学班名、教室名、教师、学期 |
| bookings | purpose、显示名、用户名、教室名 |
| regimes | name |
| imports | filename、type |

**豁免**(不分页、无信封、无 q,保持全量):`GET /api/timetable`(周派生网格)、`GET /api/schedule/active`(单条)、全部 `GET /api/{resource}/export`、单条 GET。

> 注意:原 `GET /api/imports` 的 50 条硬上限已由统一分页取代(默认 100 条/页)。

## 健康检查

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/healthz` | 进程存活(DB 开库前注册,不查库) |
| GET | `/readyz` | 就绪(开库后注册,ping DB,不可达 503) |

## 认证(`/api/auth`)

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| POST | `/api/auth/login` | 公开 | 用户名密码 → `{token, user}` |
| GET | `/api/auth/me` | 登录 | 水合当前用户 |
| POST | `/api/auth/wx-bind` | 公开 | 首次绑定(openid 须未绑) |
| POST | `/api/auth/wx-login` | 公开 | 静默登录(openid 已绑) |
| POST | `/api/auth/wx-unbind` | 登录 | 解绑 openid |

## 用户(`/api/users`,perm `UserManage`)

| 方法 | 路径 |
|------|------|
| GET | `/api/users` |
| POST | `/api/users` |
| GET | `/api/users/{id}` |
| PUT | `/api/users/{id}` |
| PATCH | `/api/users/{id}/password` |
| DELETE | `/api/users/{id}` |

## 行政班(`/api/admin-classes`)与教学班(`/api/teaching-classes`)

每个资源:`GET`(list,`*Read`)、`POST`(`*Manage`)、`GET /export`(`*Read`)、`GET /{id}`(`*Read`)、`PUT /{id}`(`*Manage`)、`DELETE /{id}`(`*Manage`)。权限前缀 `admin_class` / `teaching_class`。

## 教室(`/api/classrooms`)

`GET`(`ClassroomRead`)、`POST`(`ClassroomManage`)、`GET /export`、`GET /{id}`、`PUT /{id}`、`DELETE /{id}`。

## 作息制度(`/api/schedule`)

| 方法 | 路径 | 权限 |
|------|------|------|
| GET | `/api/schedule/regimes` | `CourseRead` |
| POST | `/api/schedule/regimes` | `CourseManage` |
| GET | `/api/schedule/regimes/export` | `CourseRead` |
| GET/PUT/DELETE | `/api/schedule/regimes/{id}` | Read / Manage / Manage |
| PUT | `/api/schedule/regimes/{id}/periods` | `CourseManage` |
| GET | `/api/schedule/active` | `CourseRead`(查询 `?date=` 生效制度) |

## 课程(`/api/courses` 库、`/api/offerings` 开课、`/api/sessions` 课次)

每个资源同构:`GET`(`CourseRead`)、`POST`(`CourseManage`)、`GET /export`、`GET /{id}`、`PUT /{id}`、`DELETE /{id}`。

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| GET | `/api/timetable` | `CourseRead` | 课表;必填 `classroom_id` + `from` + `to`(YYYY-MM-DD) |

## 预约(`/api/bookings`)

| 方法 | 路径 | 权限 |
|------|------|------|
| GET | `/api/bookings` | `ClassroomRead` |
| POST | `/api/bookings` | `ClassroomBook` |
| GET | `/api/bookings/export` | `ClassroomRead` |
| GET | `/api/bookings/{id}` | `ClassroomRead` |
| POST | `/api/bookings/{id}/cancel` | `ClassroomBook`(预约人或 admin) |
| POST | `/api/bookings/{id}/review` | `BookingApprove`(admin) |

## 导入(`/api/imports`,异步)

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| POST | `/api/imports/jwc_split` | `CourseManage` | 教务处聚合 xlsx → 6 个 job |
| POST | `/api/imports/{type}` | 按 type(见下) | 上传 xlsx 建 job,返回 `202 {id, status:"pending"}` |
| GET | `/api/imports` | 登录 | 列出 job |
| GET | `/api/imports/{id}` | 登录 | 查 job(含 preview、errorReport) |
| POST | `/api/imports/{id}/commit` | 按 type | 提交 preview → 写库,返回 `202 {id, status:"processing"}` |
| POST | `/api/imports/{id}/cancel` | 按 type | 丢弃 preview |

`{type}` 取值与权限(xlsx 列契约见 [课表导入](/guide/import)):

| type | 权限 | 必需列(其余可选) |
|------|------|------|
| `sessions` | `CourseManage` | `date, period_start, classroom, course, teaching_class, semester`(+ `period_end, note`) |
| `classrooms` | `ClassroomManage` | `name`(+ `building, capacity, type, floor, campus, status, description`) |
| `admin_classes` | `AdminClassManage` | `name`(+ `grade, note`) |
| `teaching_classes` | `TeachingClassManage` | `name, admin_grade, admin_name`(+ `note`) |
| `catalog` | `CourseManage` | `name`(+ `code, credits, total_hours, category, exam_type, description`) |
| `offerings` | `CourseManage` | `course, teaching_class, semester, teacher, note`(+ 其余教务元数据) |
| `regimes` | `CourseManage` | `regime_name, effective_month, effective_day, period_index, start_time, end_time` |
| `bookings` | `BookingApprove`(admin) | `classroom, username, date, period_start, period_end, status, purpose` |

job 状态:`pending` → `processing` → `preview` →(commit)→ `succeeded` / `failed`;`cancelled`。部分行失败仍 `succeeded`,`failedRows` 与逐行 `errorReport` 见 `GET /api/imports/{id}`。

## 导出

每个资源的 `GET /api/{resource}/export` 流式输出 xlsx,表头与对应 importer 必需列对齐,可回导。导出**忽略分页与搜索参数**(不接受 `page`/`page_size`/`q`),始终覆盖全量(或列表接口支持的过滤条件,如 bookings 的 classroom_id/status/from/to)。
