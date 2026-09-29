# 实战教训（lessons）

> 每条一个根因，写「现象 → 根因 → 规则」。过时即删，项目经验保留。

## 错误吞噬与计数

- **「失败 N 条却无错误明细」的幻影**：importer commit 阶段曾有错误被
  append 到局部 `errs` 而非 `res.Errors`，UI 只见失败数不见原因。规则：
  结果聚合只用 `Result.Errors` 一个出口，局部切片不许存在。
- **成功计数不要用 `len(clean)`**：被跳过/预检失败的行会混进成功数。
  规则：显式计数器，只在写库成功后递增。
- **任何 `rows` 循环必查 `rows.Err()`**——漏查时数据库中途断连表现为
  「静默少数据」，不是报错。

## 数据库与并发

- **「缺字段炸下游」必查 Go Scan 层**：课程库 code 曾经历可空阶段，
  8 处 `Scan` 把 NULL 扫进 `string` 直接全线报错。规则：建模型时 nullable
  与 Scan 目标类型逐一对表，别只看 SQL。
- **check-then-write 必须 TOCTOU 加固**：成员冻结检查在事务外查、事务内
  写会竞态。规则：事务内 `SELECT ... FOR UPDATE` 重校验（REPEATABLE READ
  下 `COUNT(*) ... FOR UPDATE` 用间隙锁封住插入）。
- **IN 列表分批**（约 1000 元组/批）：冲突预查全量 IN 在大文件下会撞包
  上限；sessions/bookings 的冲突查询均已分批，新写批量查询照抄。
- **幂等迁移**：ALTER 忽略 1060/1061；**1062 故意不忽略**（数据级重复是
  数据问题，不静默）；MySQL 错误消息解析收敛在 `dbutil`，不要散落拼字符串。

## 契约语义（勿当 bug 修）

- **教务处主课表「一课一周多行」是设计行为**：同一课程一周多节各占一行，
  不是重复数据（真实库分布核实过）。
- **课程库 `code` 是身份键**：`name` 可重名（同名不同码合法），一切
  upsert/引用按 code；文件内重码拒绝（防 last-wins 静默覆盖）。
- **jwc 拆分后必须按依赖顺序确认**（教室→课程库→行政班→教学班→开课→
  课次）：预览期的「不存在」是信息性提示，不是失败。

## 依赖行为变化

- **Mosquitto 2.x 与 2.0 行为差异**：`auth-init.sh` 曾踩两坑——2.1 的
  secure `fopen 'w'` 等价 O_EXCL，`-c` 遇已存在文件 EEXIST；2.1 只 chown
  data 目录、不再整树兜底（pwfile 权限要先备好）。升级 broker 前读
  `deploy/mosquitto/README.md`。
- **promtail 已 EOL（2026-03）**：日志采集用 Grafana Alloy，别再引入
  promtail 配置。

## 环境（国内开发机）

- `GOPROXY=https://goproxy.cn,direct`（默认代理源不通）；Docker Hub 不通，
  镜像 tag 用网络检索核实后到服务器 pull 验证。
- **httptest 死锁**：handler 里等 `r.Context().Done()` + 测试里
  `ts.Close()`（等 handler 返回）= 互相等死。用显式 release channel，
  别用请求取消当信号。
- shell 多层引号传中文 WHERE 会静默零行——中文条件进 SQL 用文件或参数化，
  别拼命令行。
