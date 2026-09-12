# OCM IoT 数据面规范（MQTT 契约 v1）

本文件是 OCM 物联网数据面的**规范性契约**：设备固件、边缘网关、教室中控、
后端（`backend/internal/iot`）以及模拟器（`cmd/iot-sim`）都以本文件为准。
载荷的机器可读 schema 在 [`schema/`](./schema) 目录，与本文件同源演进。

> **品牌策略**：本规范与 OCM 物联网模块是部署无关的通用基础设施，统一使用
> 中性命名（`iot`）。**OCM Things** 是自研设备与自研驱动产品线的子品牌
> （如未来的自研教室中控主机及其运行时），不出现在本规范的通用契约中。

## 1. 拓扑角色

```
设备/驱动 ──MQTT──▶ 云端 mosquitto ◀──MQTT── backend（internal/iot）
                        ▲
             iot/_meta 上的遗嘱与心跳（源生命周期）
```

- **源（source）**：一个 MQTT 凭证对应的发布者进程——学校边缘网关、教室
  中控、或调试模拟器。一个源可以代理多台设备（如网关管数十间教室）。
- **设备（device）**：源代理下的一台物理/虚拟设备，以 `deviceId`（源侧
  稳定外部编号）标识。注册表身份 = `(site, sourceId, deviceId)`。
- **后端**：订阅 `iot/#` 的唯一业务消费者，同时是命令的唯一下行发布者。

## 2. Topic 语法

```
iot/{site}/{sourceId}/{deviceId}/{channel}     设备数据与命令
iot/_meta/{sourceId}/{channel}                 源生命周期（无 site 段）
```

- `site`：部署标识（env `IOT_SITE_ID`，默认 `main`），用于共享 broker 时
  的多校隔离。
- `sourceId` / `deviceId`：稳定标识，字符集 `[A-Za-z0-9._-]`，≤128 字符，
  不以 `$` 开头（broker 保留前缀）。
- **topic 里只放稳定标识**：不放版本号（版本在载荷 `v` 字段）、不放教室、
  不放设备类型。教室绑定与类别是注册表数据，随管理面（REST）演进。
- channel 取值：`state` / `event` / `cmd` / `ack`；meta 的 channel 见 §6。
- topic 结构变更 = 新契约 = 换根（如 `iot2/`），事到临头再定，不预设版本段。

## 3. QoS / retained 矩阵

| Topic | QoS | retained | 说明 |
|---|---|---|---|
| `.../state` | 1 | **是** | 最新状态；后端重启后靠 retained 快速重建视图 |
| `.../event` | 1 | 否 | 事件流；QoS1 至少一次，注册表按
(deviceId,type,at,payload) 去重 |
| `.../cmd` | 1 | **否** | 命令严禁 retained：残留的 retained 命令等于迟到执行 |
| `.../ack` | 1 | 否 | 命令回执 |
| `_meta/...` | 1 | 遗嘱消息 retained | 见 §6 |

## 4. 载荷

所有载荷统一 `{"v": 1, ...}`；时间一律 **Unix 毫秒 UTC**。事件时间（`at`）
与到达时间（注册表 `received_at`）双记——离线缓冲重放后两者可能相差数小时。

- [`state.json`](./schema/state.json)：`{v, at, category?, attrs}`
- [`event.json`](./schema/event.json)：`{v, at, type, data?}`
- [`command.json`](./schema/command.json)：`{v, command_id, type, payload?,
  issued_at, expires_at}`
- [`ack.json`](./schema/ack.json)：`{v, command_id, status, detail?, at}`

## 5. 命令生命周期与安全规则

```
queued → delivered → acked | failed
   └─────────────────────▶ expired
```

- `queued`：后端已落库；`delivered`：broker 已接受发布；`acked/failed`：
  设备回报；`expired`：截止时间已过仍未回执。
- **强制项**：`cmd` 载荷必须带 `expires_at`（默认 30s，开门类命令 ≤10s）。
  设备**必须丢弃**已过期命令，且对过期命令回 `failed`（detail 注明
  expired）以便审计——断网期间排队的开门命令只允许过期，**绝不允许迟到
  执行**。
- 对已 settled（acked/failed/expired）命令的迟到 ack，注册表直接丢弃，
  不复活历史行。
- `command_id` 为 32 位小写十六进制，全局唯一，ack 必须原样回带。
- 命令签发走 REST（`POST /api/iot/devices/{id}/commands`），受
  `iot:control` 权限门控并进入系统审计日志。

## 6. 源生命周期（_meta）

- **遗嘱**：源连接时设置 will 到 `iot/_meta/{sourceId}/offline`（retained
  可选）。遗嘱触发后，后端将该源下全部在线设备置为 offline——这是"站点级
  离线"信号（网关/中控或其上行消失），区别于单设备离线。
- 心跳类 meta 消息（如 `heartbeat`）v1 仅记录日志；源健康页属后续工作。

## 7. 断线缓冲政策

- **state 只缓冲最新值**：源为每台设备保留最近一次 state，重连后自动重发
  （retained）。历史 state 没有意义，不进队列。
- **event/ack 走 FIFO 持久队列**：断线期间写入本地缓冲文件，重连后按序
  补发；队列有容量上限（默认 10000 条），满则丢最旧。SDK（`iot/mqttc`）
  已实现该语义，设备侧程序建议直接复用。
- 后端侧：clean session 关闭 + QoS1 订阅，后端重启期间 broker 代为排队。

## 8. 安全基线

- 每个**源**一个 broker 凭证（mosquitto dynsec），ACL 限定：
  - 发布：`iot/{site}/{sourceId}/#`、`iot/_meta/{sourceId}/#`
  - 订阅：`iot/{site}/{sourceId}/+/cmd`
- 后端凭证只存在于部署环境变量（`IOT_MQTT_USERNAME/PASSWORD`）。
- 生产启用 8883 TLS；设备侧校验 broker 证书，杜绝明文上公网。
- 凭证生命周期 v1 由 `deploy/mosquitto/dynsec-init.sh` 带外创建；按设备
  发放/吊销（认领/解绑联动）属中控阶段工作。

## 9. 明确的边界

- **消防联动走消防系统硬线，不经过本系统**——不进中控、不进 MQTT、不进云。
  本平台不做任何消防功能，避免合规与可靠性风险。
- 注册表只存最新状态（latest state）与事件流，不做时序曲线库；高频采样
  应在源侧聚合降频后再上报。

## 10. 参考实现

- 后端消费/发布：`backend/internal/iot/mqtt`（独立实现，不 import 本
  module——Docker build context 隔离）
- 设备侧 SDK：`iot/mqttc`（连接/重连/最新态重放/FIFO 缓冲/命令订阅与 ack）
- 可执行样例与开发工具：`iot/sim` + `cmd/iot-sim`（虚拟教室）
- 契约一致性由 e2e 集成测试保证（`go test -tags=integration`，CI 中对
  真 broker + 真 MySQL 运行）。
