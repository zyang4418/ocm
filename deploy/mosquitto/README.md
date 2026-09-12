# Mosquitto 部署配置（OCM IoT）

overlay `docker-compose.iot.yml` 用这里的一套配置启动 broker：

```
docker compose -f docker-compose.yml -f docker-compose.iot.yml up -d
```

## 组件

- `mosquitto.conf` — 监听 1883、关闭匿名、`password_file` + `acl_file` 认证、
  开启持久化。8883 TLS listener 以注释形式给出（生产放开并挂证书）。
- `auth-init.sh` — one-shot 容器 `mosquitto-init` 的入口，在 broker 启动前
  生成凭据：
  - backend 用户（`IOT_MQTT_USERNAME/PASSWORD`，后者必填、fail-fast），
    ACL 为整个 `iot/#` 读写；
  - 每个 `IOT_SOURCES` 中声明的源一个用户 `src-<sourceId>`，ACL 限定其
    子树：发布 `iot/{site}/{src}/{state,event,ack}`、订阅
    `iot/{site}/{src}/+/cmd`、发布遗嘱 `iot/_meta/{src}/offline`。
  - 源密码随机生成并持久化在 `mosquitto_auth` 卷的 `credentials.txt`
    （`src-<sourceId> <password>` 一行一条），重启不轮换；接入网关/
    模拟器时从这里取密码。

## 与 spec 的关系

ACL 模型即 `iot/spec/spec.md` §8 的安全基线。v1 用
`password_file + acl_file` 静态凭据实现（工具链简单、重启可复现）；按设备
发放/吊销（认领/解绑联动）需要运行时管理接口，属于 WP-8（教室中控）阶段
切换到 mosquitto dynamic security 的工作，`internal/iot/mqtt/dynsec.go`
已预留该接口。

## 注意

- CI 的 e2e 用的是另一份匿名配置（`.github/mosquitto.conf`），与本目录
  无关。
- 本目录变更会触发 `backend.yml`（paths 含 `deploy/**`）。
