---
title: 课表导入
---

OCM 支持批量导入(**xlsx**)。导入是**异步**的:上传一个 `.xlsx` → 后端解析生成**预览(preview)** → 你**确认提交**后写库 → 前端轮询状态。本页说明 xlsx 契约、用法、8 种导入类型与教务处拆分流程。路由与权限见 [API 概述](/api#导入apiimports异步)。

## 通用规则(所有类型)

- 文件 `.xlsx`(`.xls`/`.csv` **不接受**)。后端用 `excelize` 解析,不是 `encoding/csv`。
- 表头**按列名识别**:小写化 + 去空格后匹配,列顺序无关,多余列忽略,空行跳过。
- 上限 5 MiB。
- 列名须与下表逐字一致(小写)。

## 前置条件

导入按**名称**引用已存在的数据,任一名称对不上,该行失败并在错误报告中指出:

- **教室**(`classrooms`)—— `classroom` 列须与某 `classrooms.name` 完全一致。
- **开课**(`course_offerings`)—— 由 `(course, teaching_class, semester)` 三元组定位一条开课。
- **作息制度**(`schedule_regimes`/`schedule_periods`)—— `period_index` 须在该日期生效的制度中存在。

## 课次(sessions)xlsx 契约

必需列(按列名识别,顺序无关):

| 列 | 含义 | 规则 |
|----|------|------|
| `date` | 上课日期 | `YYYY-MM-DD` |
| `period_index` | 节次(作息制度节号) | 正整数;须在该日期生效制度中存在 |
| `classroom` | 教室名 | 须与 `classrooms.name` 一致 |
| `course` | 课程名 | 须与 `course_catalog.name` 一致 |
| `teaching_class` | 教学班名 | 与 course+semester 组合须匹配一条开课 |
| `semester` | 学期 | 该开课的学期 |
| `note` | 备注,可选 | 可为空 |

一节 = 一行。某班周一第 3、4 节连上是**两行**(`period_index=3` 和 `=4`),不是一行写范围。

> 注:列名是 `teaching_class`(**不是** `class`)。后端早期接受过 CSV,现已改为 xlsx;若沿用旧 CSV 流程,需改为输出 xlsx。

## 用法

### Web 端

1. 以管理员登录 → 侧边栏「数据导入」(仅 admin 可见,默认导入类型为「课次」)。
2. 选 `.xlsx` 上传 → 接口返回 `202 { id, status: "pending" }`,后台开始解析。
3. 解析完进入 `preview` 态,弹窗列出**将要写入的行**与逐行校验错误。
4. **确认导入**(后端 `POST /api/imports/{id}/commit`)→ 重新校验并写库;或**取消**丢弃 preview。
5. 页面每 3 秒轮询(仅 `pending`/`processing` 时),完成后显示总行数 / 成功 / 失败。`failedRows > 0` 时「查看明细」逐行列出错误,修正后重导失败行即可。

### API

| 步骤 | 请求 |
|------|------|
| 上传 | `POST /api/imports/{type}`(multipart `file`)→ `202 { id, status:"pending" }` |
| 查预览 | `GET /api/imports/{id}` → 含 `preview`(行)+ `errorReport` |
| 提交 | `POST /api/imports/{id}/commit` → `202 { id, status:"processing" }` |
| 取消 | `POST /api/imports/{id}/cancel` |

job 状态:`pending → processing → preview →(commit)→ succeeded/failed`;`cancelled`。部分行失败仍 `succeeded`,查 `failedRows` + `errorReport`。

## 八种导入类型

| type | 权限 | 必需列(其余可选) |
|------|------|------|
| `sessions` | CourseManage | `date, period_index, classroom, course, teaching_class, semester`(+ `note`) |
| `classrooms` | ClassroomManage | `name`(+ `building, capacity, type, floor, campus, status, description`) |
| `admin_classes` | AdminClassManage | `name`(+ `grade, note`) |
| `teaching_classes` | TeachingClassManage | `name, admin_grade, admin_name`(+ `note`) |
| `catalog` | CourseManage | `name`(+ `code, credits, total_hours, category, exam_type, description`) |
| `offerings` | CourseManage | `course, teaching_class, semester, teacher, note`(+ 教务元数据) |
| `regimes` | CourseManage | `regime_name, effective_month, effective_day, period_index, start_time, end_time` |
| `bookings` | BookingApprove(admin) | `classroom, username, date, period_start, period_end, status, purpose` |

每个资源的 `GET /api/{resource}/export` 导出的 xlsx 表头与对应 importer 必需列对齐——**导出再改再回导**是准备文件的最快方式。

## 教务处课表拆分(jwc_split)

学校教务处课表通常是一个**聚合 xlsx**(一张表含教室/课程/班级/教师等多维信息),与单类型契约不符。后端内置拆分:

- 上传聚合 xlsx + `semester` + `week1_monday`(`YYYY-MM-DD`)→ `POST /api/imports/jwc_split`(`CourseManage`)。
- 后端拆成 **6 个 job**,按依赖顺序:**教室 → 课程库 → 行政班 → 教学班 → 开课 → 课次**。
- 致命拆分错误返回 400,不创建任何 job;拆分成功后逐个 job 走上面的 preview→commit 流程。

## 从学校课表准备 xlsx

若你的课表不是教务处聚合格式(无法用 jwc_split),需写一次性脚本把每次课提取成一行、列名符合上表。推荐 `openpyxl` 读 `.xlsx`,处理合并单元格(向合并范围填左上角值)、把钟点映射成 `period_index`(用 `GET /api/schedule/active?date=...` 查生效制度)。

## 常见坑

- **只收 xlsx**:`.csv`/`.xls` 不接受(后端 `excelize`,非 `encoding/csv`)。
- **列名逐字一致**:小写,如 `teaching_class`(不是 `class`/`班级`);表头会被小写化 + 去空格后匹配。
- **节次不是钟点**:`period_index` 是作息制度节号(1 起),不是 `08:00`;原始表用钟点须先映射成节号。
- **名称须完全一致**:教室、课程、教学班、学期须与系统记录逐字一致,否则该行失败。
- **合并单元格**:表头和跨节课常见,转换时须向合并范围填充左上角值。
- **冲突**:导入只校验 `course_sessions` 唯一约束(同教室+日期+节次不重复),不校验与教室预约的冲突。
- **必须 commit**:`preview` 态不写库;不点「确认导入」job 会停在 preview。
