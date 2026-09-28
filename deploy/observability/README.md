# Observability 部署配置（参考监控栈）

overlay `docker-compose.observability.yml` 用这里的一套配置启动监控栈：

```
docker compose -f docker-compose.yml      -f docker-compose.observability.yml up -d   # dev
docker compose -f docker-compose.prod.yml -f docker-compose.observability.yml up -d   # prod
```

## 组件

- `prometheus/prometheus.yml` — 抓取 backend `:9091`（信号层
  `METRICS_ADDR`）、node-exporter `:9100`、prometheus 自身；TSDB 保留期
  30 天（compose 命令行参数）。
- `loki/loki.yml` — 单二进制 Loki，filesystem 存储、30 天保留
  （compactor 负责删除过期 chunk）。
- `alloy/config.alloy` — 容器日志采集进 Loki（Docker JSON 日志，
  `container` 标签取短容器名）。Alloy 是 Loki 的受支持采集器，
  promtail 已于 2026-03 EOL。
- `grafana/provisioning/` — 数据源（Prometheus + Loki）与开箱即用的
  「OCM 服务状态总览」面板（RED 指标、主机资源、DB 连接池、错误日志），
  uid 固定：`ocm-prometheus` / `ocm-loki` / `ocm-overview`。

## 安全基线

- 只有 Grafana 发布端口（`:3000`）；backend `/metrics`、node-exporter、
  prometheus、loki 全部仅 compose 内网可达，**不得**发布到宿主机。
- `GRAFANA_ADMIN_PASSWORD` 必填（缺省拒绝启动），交付前必须换强密码，
  建议置于 HTTPS 反向代理之后。
- 信号层端点无鉴权是设计使然（依赖网络隔离），见
  docs/docs/guide/observability.mdx。
