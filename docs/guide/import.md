# 课表导入

OCM 支持批量导入课表（course sessions）。导入是**异步**的：上传一个 CSV 文件，后端解析并写入
`course_sessions`，前端轮询任务状态直到完成。本页说明 CSV 契约、用法，以及如何用 AI 把 Excel
课表转换成符合契约的 CSV。

## 前置条件

导入前，以下数据必须已在系统中建立，CSV 按名称引用它们：

- **教室**（classrooms）—— `classroom` 列必须与某个 `classrooms.name` 完全一致。
- **开课**（course offerings）—— 由 `(course, class, semester)` 三元组唯一确定一条 `course_offering`。
- **作息制度**（schedule regimes / periods）—— `period_index` 必须在该日期生效的作息制度中存在。

任一名称对不上，该行导入失败并在错误报告中指出。

## CSV 契约

首行为表头，**按列名识别**，列顺序无关，多余列忽略。必需列：

| 列 | 含义 | 规则 |
|----|------|------|
| `date` | 上课日期 | `YYYY-MM-DD` |
| `period_index` | 节次（作息制度中的节号） | 正整数；须在该日期生效的作息制度中存在 |
| `classroom` | 教室名 | 须与现有 `classrooms.name` 完全一致 |
| `course` | 课程名（如 高等数学） | 须与现有 `course_catalog.name` 一致 |
| `class` | 班级名（如 计科2301） | 与 course+semester 组合须匹配一条开课 |
| `semester` | 学期（如 2025秋） | 该开课的学期 |
| `note` | 备注，可选 | 可为空 |

一节 = 一行。某班周一第 3、4 节连上，是 **两行**（`period_index=3` 和 `period_index=4`），不是一行写范围。

## 用法

1. 以管理员登录 Web 端，进入 **教学管理 → 课表导入**。
2. 选择 CSV 文件并上传。接口立即返回 `202 { id, status: "pending" }`，后台开始处理。
3. 页面每 3 秒轮询任务状态，显示总行数 / 成功 / 失败。
4. 完成后：`status` 为 `succeeded`（可能仍有部分行失败）或 `failed`（无法提交）。
   `failedRows > 0` 时点击「查看错误」可看到逐行错误（JSON 数组，每项 `{ row, error }`），
   修正后重新导入失败行即可。

## 用 AI 从 Excel 生成 CSV

学校课表通常是 Excel（`.xlsx`），布局各异（合并单元格、二维网格、`课程\n班级\n教师` 之类单元格）。
OCM 后端**不解析 xlsx**，只接受上面的 CSV。可用一个 Claude Code 技能完成转换：

<a href="/skills/timetable-to-csv/SKILL.md" download>下载 timetable-to-csv 技能（SKILL.md）</a>

安装：将下载的文件放到 `~/.claude/skills/timetable-to-csv/SKILL.md`，然后在 Claude Code 中把 Excel
课表交给它。技能会检查文件、写一次性 Python 脚本（`openpyxl`）把每次课提取为一行 CSV，并在导入前
请你人工核对。

## 常见坑

- **编码**：CSV 须为 UTF-8 且无 BOM。后端用 `encoding/csv` 读取，BOM 会让首列名变成 `﻿date`
  而匹配失败。
- **合并单元格**：表头和跨节课常见，转换时须用左上角值向合并范围填充。
- **节次不是钟点**：`period_index` 是作息制度的节号（1 起），不是 `08:00` 这类钟点。若 Excel 用钟点，
  先用 `GET /api/schedule/active?date=...` 查生效制度，映射成节号再输出。
- **名称须完全一致**：教室、课程、班级、学期的名称必须与系统中的记录逐字一致，否则该行失败。
- **冲突**：导入只校验 `course_sessions` 的唯一约束（同一教室、日期、节次不能重复），不校验与教室
  预约的冲突。
