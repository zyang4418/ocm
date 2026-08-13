---
name: timetable-to-csv
description: Convert a school's Excel timetable (.xlsx) into the OCM backend's sessions CSV import format. Use when the user provides an Excel timetable file (or path) and wants to import it into the classroom management system. Writes a one-off Python script to untangle arbitrary layouts (merged cells, 2D grids) and emits a tidy CSV the backend can import.
---

# Timetable Excel → CSV

You are given a school timetable in Excel (.xlsx). The backend (OCM) only imports
a tidy CSV — it does NOT parse xlsx. Your job: inspect the file, write a one-off
Python script that extracts every class meeting into the CSV format below, run
it, and present the result for human review before import.

## The CSV contract (the backend consumes exactly this)

First row is a header. Columns are matched **by name**, so order does not matter
and extra columns are ignored. Required columns:

| column         | meaning                                              | rules |
|----------------|------------------------------------------------------|-------|
| `date`         | meeting date                                         | `YYYY-MM-DD` |
| `period_index` | bell-time period (节次)                              | positive integer; must exist in the active regime for that date |
| `classroom`    | classroom name                                       | must match an existing `classrooms.name` exactly |
| `course`       | catalog course name (e.g. 高等数学)                  | must match an existing `course_catalog.name` |
| `class`        | class name (e.g. 计科2301)                           | combined with course+semester, must match an existing offering |
| `semester`     | semester (e.g. 2025秋)                               | the offering's semester |
| `note`         | free text, optional                                  | may be empty |

An offering is resolved by the triple `(course, class, semester)` — that triple
must uniquely identify an existing `course_offering`. If any name does not match
an existing record, that row fails import with a clear error.

## How to proceed

1. **Inspect** the workbook with Python (`openpyxl`). Identify: which sheet
   holds the timetable, how rows map to dates/periods, how columns map to
   days/classrooms, where course/class/teacher info lives inside cells (often
   `课程\n班级\n教师` or `课程@教室`). Watch for **merged cells** (common in
   header rows and spanned periods) — resolve them by filling from the
   top-left value across the merged range.

2. **Write a one-off Python script** (`openpyxl`, or `pandas` if the layout is
   already tabular) that walks the grid and emits one CSV row per
   (date, period, classroom) meeting. Print a short summary (row count, date
   range, distinct classrooms/courses seen).

3. **Pre-validate names** if the user gives you an API base URL and a bearer
   token: `GET /api/classrooms` and `GET /api/offerings` to fetch the legal
   names, and flag any CSV row whose classroom/course/class/semester is not
   found. If no credentials are provided, skip this — the backend will report
   mismatches per row on import.

4. **Show the CSV** to the user (first ~20 rows + counts) and ask them to
   review before importing. A silently misassigned classroom or period is a
   real problem; do not skip this review step.

5. **Upload** (after the user confirms):
   - Via the web admin: 教学管理 → 课表导入, attach the `.csv`.
   - Or via curl:
     ```
     curl -F file=@timetable.csv \
       -H "Authorization: Bearer <token>" \
       <API_BASE>/api/imports/sessions
     ```
     Returns `202 {"id":..., "status":"pending"}`. Poll
     `GET /api/imports/{id}` until `status` is `succeeded` or `failed`.
     `failedRows > 0` means some rows were rejected — fetch
     `GET /api/imports/{id}` and read `errorReport` (JSON array of
     `{"row","error"}`) to fix and re-import the failed rows.

## Requirements & gotchas

- Python with `openpyxl` (or `pandas` + `openpyxl`) must be available. If not,
  install it (`pip install openpyxl`) or ask the user to.
- The output CSV must be UTF-8. If it may contain a BOM, strip it; the backend
  reads with `encoding/csv` which does not expect a BOM.
- One CSV row = one meeting. A class that meets periods 3 and 4 on Monday is
  **two** rows (period_index=3 and period_index=4), not one range row.
- `period_index` is the regime's period number (1-based), NOT a clock time.
  If the source Excel uses clock times, map them to period indices using the
  active regime (`GET /api/schedule/active?date=...`) before emitting.
- Do not invent data the source doesn't have. If a cell is ambiguous, surface
  it to the user rather than guessing.
