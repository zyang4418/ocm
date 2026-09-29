# OCM — An Open-source Classroom Management System 智慧教室管理系统

An integrated smart-classroom system: courses, timetables, classroom bookings, attendance, lesson 
observation and IoT devices — one Go backend, a React (Carbon) web console and a WeChat mini-
program, plus a device-side IoT SDK and an optional monitoring stack.

课堂管理一体的智慧教室系统:课程 / 课表 / 教室预约 / 考勤 / 听课评课 / 物联网设备,三端一体——**Go 后端** + **Web 控制台**(React + Carbon) + **微信小程序**,外加 IoT 设备侧 SDK 与可选监控栈。

**Full documentation (deployment, contracts, per-client guides): [docs.zyang4418.cn](https://docs.zyang4418.cn)**

**完整文档(部署、契约、各端指南)见文档站: [docs.zyang4418.cn](https://docs.zyang4418.cn)**

## Quick Start 快速开始

```bash
git clone <repo> ocm && cd ocm
# dev stack: mysql + backend (air hot-reload) + web (vite)
# 开发全栈: mysql + backend(air 热更) + web(vite)
docker compose up -d
```

Web console at http://localhost:5173 , default account `admin` / `admin123` (dev default).
Production deployment, IoT and observability overlays: see [Quick Start](https://docs.zyang4418.cn/) on the docs site.

Web 控制台 http://localhost:5173 ,默认账号 `admin` / `admin123`(开发默认)。
生产部署、IoT 与监控 overlay 见文档站[快速开始](https://docs.zyang4418.cn/)。

## Repository Map 仓库地图

| Directory | Contents |
|---|---|
| `backend/` | Go backend (`net/http` ServeMux + MySQL 8, idempotent migrations) |
| `web/` | Web console (React 19 + Vite + Carbon, TypeScript strict) |
| `miniapp/` | WeChat mini-program (glass-easel + TDesign, WebView by default) |
| `iot/` | Device-side SDK & simulator (second Go module; MQTT contract in `iot/spec/spec.md`) |
| `docs/` | Documentation site (Docusaurus) |
| `deploy/` | Deployment configs (mosquitto, observability) |
| `agents/` | Shared reference for AI coding tools (lessons & research notes) |

| 目录 | 内容 |
|---|---|
| `backend/` | Go 后端(`net/http` ServeMux + MySQL 8,幂等迁移) |
| `web/` | Web 控制台(React 19 + Vite + Carbon,TypeScript strict) |
| `miniapp/` | 微信小程序(glass-easel + TDesign,默认 WebView) |
| `iot/` | 设备侧 SDK 与模拟器(第二个 Go module,MQTT 契约见 `iot/spec/spec.md`) |
| `docs/` | 文档站(Docusaurus) |
| `deploy/` | 部署配置(mosquitto、observability) |
| `agents/` | AI 编程工具共享参考(教训与选型记录) |
