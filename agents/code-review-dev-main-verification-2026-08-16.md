# dev → main 分支代码审查 —— 问题核实报告（2026-08-16）

> 背景：以下是早前一次针对 `dev → main` 的代码审查，共提出 10 个问题（P1×3 / P2×4 / P3×3）。
> 本次逐条核对当前工作区（`repair` 分支 = `main` 合并 `go` + 后续模块）代码，判断问题是否仍存在。
> 核实方式：只读静态分析，未改动任何代码。

## 总结论

**10 个问题已全部修复（或已妥善处理），当前代码中不存在该列表的遗留问题。**

---

## P1 缺陷（3 个）—— 全部已修复

| # | 问题 | 现状 | 证据 |
|---|---|---|---|
| 1 | `commitTeachingClasses` 错误被静默丢弃（追加到 `errs` 而非 `res.Errors`） | ✅ 已修复 | `importer/teaching_classes.go`：`res := Result{... Errors: errs}`（L212）初始化后，所有 continue 路径均改为 `res.Errors = append(res.Errors, ...)`（L233/250/273/305 等），无遗留 `errs = append` |
| 2 | `SucceededRows = len(clean)` 把被跳过行计为成功 | ✅ 已修复 | 引入显式计数器 `var succeeded int`（L217），仅在写库成功后 `succeeded++`（L297/323），最终 `res.SucceededRows = succeeded`（L331） |
| 3 | `loadTeachingClassState` 三个查询缺 `rows.Err()` | ✅ 已修复 | 三个查询（teaching_classes L415、members L433、offerings L454）均补上 `rows.Err()` 检查并返回错误 |

## P2 设计问题（4 个）—— 全部已修复

| # | 问题 | 现状 | 证据 |
|---|---|---|---|
| 4 | `jwcSplit` 循环创建任务，部分失败留孤儿 | ✅ 已修复 | 改用原子批 `store.CreateJobs(...)`（`handler.go` L267-270），注释明确「atomic batch: no jobs created, no goroutines started」 |
| 5 | `existingBookingConflicts` 未分批 IN 列表 | ✅ 已修复 | 采用与 sessions 一致的 `batchTupleSize` 分批（`bookings.go` L423-427） |
| 6 | 成员冻结检查 TOCTOU（事务外检查/事务内写入） | ✅ 已修复 | commit 阶段在事务内**重新**校验：`SELECT ... FOR UPDATE` 锁 teaching_classes 行（L246）+ `COUNT(*) ... FOR UPDATE` 间隙锁 course_offerings（L259）+ 重读成员比对 `slices.Equal`（L272），注释详细说明锁顺序防死锁 |
| 7 | `bookingFilter` 未校验 from/to 日期 | ✅ 已修复 | `booking/handler.go` `bookingFilter`（L66-77）对 from/to 做 `time.Parse("2006-01-02")` 校验，非法返回 400，list/export 共用 |

## P3 代码质量（3 个）—— 全部已处理

| # | 问题 | 现状 | 证据 |
|---|---|---|---|
| 8 | `org_handler.go` 的 `read` 包裹器命名误导 | ✅ 已修复 | 改为单一 `withPerm(perm, handler)` 包装器，调用点显式传 `authz.AdminClassManage` / `authz.TeachingClassManage` 等，命名诚实 |
| 9 | `duplicateKeyName` 用 `strings.LastIndex` 解析 MySQL 错误消息 | ✅ 已妥善处理 | 收敛到 `dbutil.DuplicateKeyName`（`dbutil/dbutil.go` L39），并补了详细注释说明「格式跨 5.7/8.0/8.4/MariaDB 稳定，解析失败返回空、降级为 ErrNameTaken 而非崩溃」；`IsDuplicateEntry` 则改用规范的 `mysql.MySQLError.Number == 1062` 判断，不再拼字符串 |
| 10 | 辅助函数跨包重复（`isDuplicateEntry`×3、`sameInt64Set`×2） | ✅ 已修复 | `isDuplicateEntry` 统一为 `dbutil.IsDuplicateEntry`（全仓仅 1 处定义）；`sameInt64Set` 已删除，改用标准库 `slices.Equal` |

---

## 附带说明

- 原审查「做得好的地方」所列优点（SQL 全参数化、commit 重新校验、sessions 分批、幂等迁移、成员冻结、事务配对 rollback、导出/导入往返）在当前代码中依然成立，未见回退。
- 本次仅核实该旧审查列表本身，未重新对 importer 做全面扫描。
