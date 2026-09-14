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
  的多校隔离：后端只消费 site 等于自身配置的数据消息（state/event/ack），
  其他站点一律丢弃。隔离靠 topic 段把守，不靠载荷——ack 里没有 site。
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

### 5.1 命令类型词表

`type` 是平台与设备之间的受控词汇：后端在 API 入口按词表拒绝未知类型
（typo 进不了 broker，审计词汇保持统一）；设备侧对不认识但仍到达的 type
一律回 `failed` ack——词表是词汇门，不是能力门。核心词表：

| type | 寻址层级 | payload | 语义 |
|---|---|---|---|
| `door_open` / `door_close` | 功能 | `{"function": "<功能键或功能类型>"}` | 开门 / 关门 |
| `device_on` / `device_off` | 功能 | `{"function": "<功能键或功能类型>"}` | 通用开关 |
| `volume_set` | 教室 | `{"value": <0-100>}` | 音量等连续量 |
| `scene_start_class` / `scene_end_class` | 教室 | 无 | 一键上课 / 一键下课（教室级全开 / 全关） |
| `state_query` | 节点 | `{"types": [...]}` 可选 | 触发一轮状态回读 |

- `type` 说做什么，`payload.function` 说对谁做。`function` 取**功能键**
  （具体设备编码）或**功能类型**（作用于该类型的全部功能）；缺省 = 命令
  所寻址设备对应的功能，寻址实体无对应功能时设备必须回 `failed`——缺省
  不做隐式广播。功能键即设备侧状态上报使用的编码，平台透传不解释。
- 词表语法 `[a-z][a-z0-9_]*`、≤64 字符。部署可在后端注册点追加部署词表
  （`iot.RegisterCommandTypes`，从下游组装文件的 init 调入）；注册与设备
  侧实现必须同一次变更成对交付，与既有词冲突属接线错误（启动即 panic），
  核心词表名为平台保留。
- 按源声明能力做校验（设备上线自述接受的命令集）属后续工作，注册点是其
  前置位。

## 6. 源生命周期（_meta）

- **遗嘱**：源连接时设置 will 到 `iot/_meta/{sourceId}/offline`（retained
  可选）。遗嘱触发后，后端将该源下全部在线设备置为 offline——这是"站点级
  离线"信号（网关/中控或其上行消失），区别于单设备离线。
- 心跳类 meta 消息（如 `heartbeat`）v1 仅记录日志；源健康页属后续工作。
- `_meta` 不带 site 段：v1 的一个 broker 由一次 auth-init 引导，只服务一个
  site（sourceId 在 broker 内即唯一）。真正的多校共享 broker 引导属后续
  工作，届时 `_meta` 语法需要加 site 段。

## 7. 断线缓冲政策

- **state 只缓冲最新值**：源为每台设备保留最近一次 state，重连后自动重发
  （retained）。历史 state 没有意义，不进队列。
- **event/ack 走 FIFO 持久队列**：断线期间写入本地缓冲文件，重连后按序
  补发；队列有容量上限（默认 10000 条），满则丢最旧。SDK（`iot/mqttc`）
  已实现该语义，设备侧程序建议直接复用。
- 后端侧：clean session 关闭 + QoS1 订阅，后端重启期间 broker 代为排队。

## 8. 安全基线

- 每个**源**一个 broker 凭证（v1 由 `deploy/mosquitto/auth-init.sh` 以
  password_file + acl_file 静态引导），ACL 限定：
  - 发布：`iot/{site}/{sourceId}/+/state|event|ack`（数据 topic 按设备分层，
    通道前一级是 `+`）、遗嘱 `iot/_meta/{sourceId}/offline`
  - 订阅：`iot/{site}/{sourceId}/+/cmd`
- 后端凭证只存在于部署环境变量（`IOT_MQTT_USERNAME/PASSWORD`）。
- 生产启用 8883 TLS；设备侧校验 broker 证书，杜绝明文上公网。
- 凭证生命周期 v1 带外创建；按设备发放/吊销（认领/解绑联动）属中控阶段
  工作，届时切换 mosquitto dynamic security（后端侧接口已预留：
  `backend/internal/iot/mqtt/dynsec.go`）。

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

## 11. 教室节点模式（推荐的部署形态）

规范对设备颗粒度保持中立：一个源可按"每台设备一实体"或"每教室一节点"
上报，topic/载荷契约对两者同样成立（两种拓扑各有合法实现）。教室类部署
推荐**节点模式**：

- **每间教室一个设备实体**（deviceId 约定 `{教室}-CTL`，与控制器代次无关
  ——更换控制器硬件是同一实体的续用）。在位性 = 该实体的上报能力本身：
  控制器可达，实体就有新 state；控制器失联，由遗嘱/TTL 判离。**不给外围
  设备编造在位性**——门磁、继电器、屏不各自联网，为它们各建实体只会在
  控制器离线时制造 N 条症状（一次根因变成 N 次告警）。
- **外围是节点的功能，不是独立实体**：功能不可独立寻址，控制命令按
  `payload.function` 寻址（§5.1）；状态是节点实体的属性命名空间，推荐
  形态：

      { "online": true, "volume": 96,
        "functions": { "<功能键>": { "kind": "...", "state": "...", ... } } }

  `functions` 的键即功能键（§5.1 的取值空间）；`kind` 是节点自报的功能
  类型——**上报配置单**是节点控制器的义务，也是它相对黑箱控制器的进步
  空间。整包替换语义（§4）适用于节点级：每次状态变化发布合并后的完整
  attrs，一帧多设备变化只发一次。
- **事件带功能字段**：功能域事件（门磁、继电器、报警等）发布在节点实体
  上，`data.function` 携带功能键；节点域事件（如控制器连接断开）不带。
  事件只属于真实翻转/发生，节点重连后的首包状态回读只入属性、不发事件。
- **认领与台账**：节点实体按 pending→approve 流程认领一次；外围的型号/
  SN/安装位/更换记录等功能语义的家在台账（管理面数据，随管理面演进），
  不进 topic、不做运行时发现。
