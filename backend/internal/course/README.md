# 课程数据模型与教学班（合班）设计

> 本文档面向后续开发者，说明 OCM 课程管理的数据模型，重点解释 **教学班（teaching class）** 这一层的引入背景与实现细节。阅读本文可快速理解「一个课程如何被多个行政班在同一时段、同一教室合班执行」是如何落地的。

## 1. 概述：课程三层结构

课程数据分为三层，自上而下：

| 层 | 表 | 中文 | 说明 |
|---|---|---|---|
| L1 | `course_catalog` | 课程库（抽象课程） | 课程的元信息，如「高等数学」「MATH101」，不含学期/教师 |
| L2 | `course_offerings` | 开课 | 某课程在某学期、由某教师、面向某教学班的一次开课 |
| L3 | `course_sessions` | 课次（上课实例） | 某开课在具体某天、某教室、某节次的一次上课 |

关系链：`course_catalog 1—N course_offerings 1—N course_sessions`。

L2「开课」是承上启下的关键：它把一门抽象课程、一个学期、一位教师、一个授课群体绑定在一起。**「授课群体」就是教学班。**

## 2. 为什么引入「教学班」

### 2.1 旧模型的问题

早期 `course_offerings` 用一个自由文本字段 `class_name`（如「计科2201」）表示班级，且唯一约束为 `UNIQUE(catalog_id, semester)`——即同一课程同一学期只能开一次。

这无法表达两类常见场景：

1. **合班**：一门课由多个行政班**在同一时段、同一教室**一起上。例如「高等数学」由「计科2401 + 计科2402」合班上课。旧模型只有一个 `class_name` 字符串，无法结构化表达「多个行政班」，也无法保证引用的行政班真实存在。
2. **同教师同学期面向不同群体分次开课**：例如高数老师上午给 1+2 班上一组、下午给 3+4 班上一组。旧模型的 `UNIQUE(catalog_id, semester)` 会把这两组视为重复而拒绝创建。

### 2.2 教学班的定义

引入**教学班（teaching class）**：一个**命名的行政班集合**，代表「被一起授课的一组行政班」。

- **行政班（admin class）**：持久的学生编制，由 `年级 + 名称` 标识，如 `2024级 / 计算机244`。属于用户/人员管理模块（组织单元，不是课程概念）。
- **教学班（teaching class）**：若干行政班的命名组合（合班），如「高数-A班（1+2）」= {计算机241, 计算机242}。属于用户/人员管理模块。

一次开课（L2）面向**恰好一个教学班**。于是：

- **合班** = 教学班包含多个行政班成员。同一时段、同一教室上一节课，由一个教学班（多成员）承载，天然成立。
- **同教师同学期分群体开课** = 两个不同教学班 → 两次开课。唯一约束改为 `UNIQUE(catalog_id, teaching_class_id, semester)`，两组互不冲突。

### 2.3 场景示例

> 高数老师同一学期：上午给「计算机241 + 计算机242」上课，下午给「计算机243 + 计算机244」上课。

建模：

- 行政班：计算机241、计算机242、计算机243、计算机244
- 教学班 A「高数-上午」= {241, 242}；教学班 B「高数-下午」= {243, 244}
- 开课 1：高数 / 教学班 A / 张老师 / 2026秋
- 开课 2：高数 / 教学班 B / 张老师 / 2026秋

两次开课因 `teaching_class_id` 不同而满足唯一约束；各自的课次（L3）落在不同时段/教室，互不冲突。

## 3. 数据表

### 3.1 组织表（`internal/user/org_store.go`，由 `user.Store.Migrate` 创建）

```sql
-- 行政班
CREATE TABLE admin_classes (
  id         BIGINT AUTO_INCREMENT PRIMARY KEY,
  grade      VARCHAR(64)  NOT NULL DEFAULT '',   -- 如 "2024级"
  name       VARCHAR(64)  NOT NULL,              -- 如 "计算机244"
  note       VARCHAR(255) NOT NULL DEFAULT '',
  created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (grade, name)
);

-- 教学班
CREATE TABLE teaching_classes (
  id         BIGINT AUTO_INCREMENT PRIMARY KEY,
  name       VARCHAR(64)  NOT NULL UNIQUE,       -- 如 "高数-A班（1+2）"
  note       VARCHAR(255) NOT NULL DEFAULT '',
  created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 教学班成员（多对多：教学班 N—N 行政班）
CREATE TABLE teaching_class_members (
  teaching_class_id BIGINT    NOT NULL,
  admin_class_id    BIGINT    NOT NULL,
  created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (teaching_class_id, admin_class_id)
);
```

### 3.2 课程表（`internal/course/store.go`，由 `course.Store.Migrate` 创建）

```sql
CREATE TABLE course_catalog (
  id          BIGINT AUTO_INCREMENT PRIMARY KEY,
  name        VARCHAR(128) NOT NULL UNIQUE,
  code        VARCHAR(64)  NOT NULL DEFAULT '',
  description VARCHAR(255) NOT NULL DEFAULT '',
  created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE course_offerings (
  id                BIGINT AUTO_INCREMENT PRIMARY KEY,
  catalog_id        BIGINT       NOT NULL,          -- → course_catalog.id（逻辑外键）
  teaching_class_id BIGINT       NOT NULL,          -- → teaching_classes.id（逻辑外键）
  teacher           VARCHAR(64)  NOT NULL DEFAULT '',
  semester          VARCHAR(32)  NOT NULL,
  note              VARCHAR(255) NOT NULL DEFAULT '',
  created_at        TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (catalog_id, teaching_class_id, semester)
);

CREATE TABLE course_sessions (
  id           BIGINT AUTO_INCREMENT PRIMARY KEY,
  offering_id  BIGINT NOT NULL,   -- → course_offerings.id（逻辑外键）
  classroom_id BIGINT NOT NULL,   -- → classrooms.id（逻辑外键）
  date         DATE NOT NULL,
  period_index INT NOT NULL,
  note         VARCHAR(255) NOT NULL DEFAULT '',
  created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (classroom_id, date, period_index)   -- 同教室同时段只能一节课
);
```

> **注意**：`course_offerings` 已**移除**旧的 `class_name` 列，改为 `teaching_class_id`。这是本次改造的核心变更点。

## 4. 关键约束与逻辑外键

- **无数据库级 FOREIGN KEY 约束**。所有跨表引用（`catalog_id`、`teaching_class_id`、`offering_id`、`classroom_id`）均为**逻辑外键**，由应用层校验存在性。这与项目既有约定一致（见 `CLAUDE.md` 的 MySQL 幂等迁移模式）。
- **存在性校验**：创建/更新开课时，`course.Store` 先 `SELECT 1 FROM teaching_classes WHERE id=?` 校验教学班存在，不存在返回 `ErrTeachingClassNotFound`。
- **教学班成员冻结（硬化）**：当教学班已被某开课引用（`course_offerings.teaching_class_id`）时，**不允许变更其成员集合**，否则返回 `ErrClassInUse`（HTTP 409）。这避免了一个教学班被排课后，成员变动导致课次归属语义错乱。仍可修改名称、备注。删除教学班同理：被引用时拒绝（`ErrClassInUse`）。
- **行政班引用保护**：删除行政班时，若已被某教学班引用（`teaching_class_members`），返回 `ErrClassInUse`（HTTP 409），需先从教学班移除。
- **教学班成员下限**：创建/更新教学班时至少需要 1 个行政班成员（`ErrMemberRequired`，HTTP 400）。
- **唯一约束**：
  - `admin_classes`：`UNIQUE(grade, name)`
  - `teaching_classes`：`UNIQUE(name)`
  - `course_offerings`：`UNIQUE(catalog_id, teaching_class_id, semester)`
  - `course_sessions`：`UNIQUE(classroom_id, date, period_index)`

## 5. 权限模型

行政班/教学班属用户/人员模块，配有独立权限（`internal/authz/authz.go`）：

| 权限常量 | 值 | admin | user |
|---|---|---|---|
| `AdminClassRead` | `admin_class:read` | ✓ | ✓ |
| `AdminClassManage` | `admin_class:manage` | ✓ | — |
| `TeachingClassRead` | `teaching_class:read` | ✓ | ✓ |
| `TeachingClassManage` | `teaching_class:manage` | ✓ | — |

`admin` 角色通配放行；`user` 角色仅可读。课程模块沿用 `course:read` / `course:manage`。权限链为 `auth.Middleware → user.LoadSubject → authz.RequirePermission`（见 `CLAUDE.md`）。

## 6. API 路由

由 `user.registerOrgRoutes`（`internal/user/org_handler.go`）注册，挂在 `user.Handler.RegisterRoutes` 末尾：

| 方法 | 路径 | 权限 |
|---|---|---|
| GET | `/api/admin-classes` | `admin_class:read` |
| POST | `/api/admin-classes` | `admin_class:manage` |
| GET | `/api/admin-classes/{id}` | `admin_class:read` |
| PUT | `/api/admin-classes/{id}` | `admin_class:manage` |
| DELETE | `/api/admin-classes/{id}` | `admin_class:manage` |
| GET | `/api/teaching-classes` | `teaching_class:read` |
| POST | `/api/teaching-classes` | `teaching_class:manage` |
| GET | `/api/teaching-classes/{id}` | `teaching_class:read` |
| PUT | `/api/teaching-classes/{id}` | `teaching_class:manage` |
| DELETE | `/api/teaching-classes/{id}` | `teaching_class:manage` |

教学班的创建/更新载荷为**替换语义**：

```json
{ "name": "高数-A班（1+2）", "note": "", "classIds": [10, 11] }
```

`classIds` 为成员行政班 ID 的**完整集合**，服务端据此重建 `teaching_class_members`。

开课（`/api/offerings`）载荷相应变更：

```json
{ "catalogId": 1, "teachingClassId": 5, "teacher": "张三", "semester": "2026秋", "note": "" }
```

`teacher` 现为**必填**（旧模型可空）。

## 7. 视图字段（开课 / 课次）

为便于前端展示，`OfferingView` / `SessionView` 同时返回教学班名与成员行政班名：

```json
{
  "id": 1, "catalogId": 1, "catalogName": "高等数学", "catalogCode": "MATH101",
  "teachingClassId": 5, "teachingClassName": "高数-A班（1+2）",
  "classNames": ["计算机241", "计算机242"],
  "teacher": "张三", "semester": "2026秋", "note": ""
}
```

- `teachingClassName`：教学班名称（JOIN `teaching_classes`）。
- `classNames`：成员行政班名称数组（`course_offerings → teaching_class_members → admin_classes`，按 `grade, name` 排序）。`classNamesByTeachingClass`（course 模块）/ `classNamesBySession`（按课次聚合）以单次批量查询填充，避免 N+1。

## 8. CSV 课表导入

`internal/importer` 的 CSV 表头中，原 `class` 列**改名为 `teaching_class`**，值为**教学班名称**（不再是行政班名/自由文本）。导入按 `课程名 | 教学班名 | 学期` 解析开课（`offeringByKey`），结构不变。

表头（按列名识别，顺序无关）：

```
date, period_index, classroom, course, teaching_class, semester, note
```

## 9. 迁移说明（重要）

本次改造**改变了 `course_offerings` 的表结构**（删 `class_name`、加 `teaching_class_id`、改唯一约束）。`course.Store.Migrate` 使用 `CREATE TABLE IF NOT EXISTS`，**不会自动改已有表**。组织三表为新增表，首次启动自动创建，无需处理。

由于项目处于开发阶段、无实质性数据，最简迁移方式是**删除旧的 `course_offerings`（及引用它的 `course_sessions`）**，让 Migrate 在下次启动时按新结构重建：

```sql
-- ⚠️ 破坏性操作：会清空这两张表的全部数据。仅在开发库执行。
DROP TABLE IF EXISTS course_sessions;
DROP TABLE IF EXISTS course_offerings;
```

执行后重启后端，`Migrate` 会以新结构重建 `course_offerings` / `course_sessions`，并新建 `admin_classes` / `teaching_classes` / `teaching_class_members`。

> 如需保留 `course_sessions` 数据，可改为 `ALTER`：先 `ALTER TABLE course_offerings DROP INDEX <旧唯一索引名>`，`DROP COLUMN class_name`，`ADD COLUMN teaching_class_id BIGINT NOT NULL DEFAULT 0`，再 `ADD UNIQUE (catalog_id, teaching_class_id, semester)`——但开发阶段不建议，DROP 重建更干净。

## 10. 前端

- **组织与权限**导航组下新增「行政班管理」「教学班管理」两个页面（`web/src/pages/AdminClassesPage.jsx`、`TeachingClassesPage.jsx`），路由 `/admin-classes`、`/teaching-classes`。
- **课程管理**页开课表单：原「班级」文本框改为「教学班」下拉（数据源 `/api/teaching-classes`），`teachingClassId` 必填；教师改为必填。列表新增「行政班」列（`classNames` 聚合展示）。
- **教室课表**页：课次单元格与开课下拉改用 `teachingClassName`。
- **课表导入**页：CSV 表头提示与预览列 `class → teaching_class`（预览字段 `teachingClass`）。
- 教学班表单用 Carbon `MultiSelect` 选择成员行政班；行政班为空时提示先创建。

## 11. 文件清单

| 文件 | 变更 |
|---|---|
| `internal/user/org_model.go` | 新增：行政班/教学班/成员类型 |
| `internal/user/org_store.go` | 新增：建表、CRUD、引用保护/成员冻结 |
| `internal/user/org_handler.go` | 新增：REST 路由与校验 |
| `internal/user/handler.go` | 改：注册 org 路由 |
| `internal/course/model.go` | 改：`Offering/SessionView/Inputs` 改 `teachingClassId` |
| `internal/course/store.go` | 改：建表、JOIN 教学班、`classNamesByTeachingClass` |
| `internal/course/sessions.go` | 改：JOIN 教学班、`classNamesBySession` |
| `internal/course/handler.go` | 改：`normalizeOffering`（教学班/教师必填） |
| `internal/importer/model.go` | 改：`ColClass→ColTeachingClass`、`PreviewRow` |
| `internal/importer/sessions_csv.go` | 改：CSV 列名与解析键 |
| `internal/authz/authz.go` | 改：新增 4 个权限常量并赋予 user 读权限 |
| `main.go` | 改：启动时调用 `userStore.Migrate` |
