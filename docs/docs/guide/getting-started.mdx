---
title: 快速开始
slug: /
---

OCM = 智慧教室管理平台,三端一体:**Go 后端** + **微信小程序** + **Web 控制台**,外加 MySQL 8。最快的方式是用 `docker compose` 一键起全栈。

## 前置

- **Docker**(推荐用 compose 一键起);或本地装 Go 1.26+、Node ≥ 20.19、MySQL 8。
- **微信开发者工具**(仅开发小程序时需要)。
- 仓库根 `.env.example`:`cp .env.example .env`,按需填(开发可不填,用默认值;生产必填,见下)。

## 一键起全栈(推荐)

```bash
git clone <repo> ocm && cd ocm
docker compose up -d            # mysql + backend(air 热更)+ web(vite)
```

启动后:

| 服务 | 地址 | 说明 |
|------|------|------|
| Web 控制台 | http://localhost:5173 | 登录用下方默认账号 |
| 后端 API | http://localhost:8080 | `/api/...`;`/healthz`、`/readyz` |
| MySQL | localhost:3306 | 库 `ocm` / 用户 `ocm` / 密码 `ocm`(dev) |

dev compose 用开发默认值:`JWT_SECRET` 空(后端用开发兜底密钥)、`ADMIN_PASSWORD` 空(后端用 `admin123`)。所以**不建 `.env` 也能跑开发栈**;但小程序登录需要 `WX_APP_ID`/`WX_APP_SECRET`(见下)。

**默认账号**:首次启动 `users` 表为空时种入管理员,用户名 `admin`,密码 = `ADMIN_PASSWORD`(开发默认 `admin123`)。用它在 http://localhost:5173 登录 Web 端。

## 环境变量(.env)

compose 自动加载仓库根 `.env`。关键项:

| 变量 | 开发 | 生产 | 说明 |
|------|------|------|------|
| `JWT_SECRET` | 可空(兜底) | **必填** | HS256 签名密钥 |
| `ADMIN_PASSWORD` | 可空(`admin123`) | **必填** | 种入 admin 的密码 |
| `WX_APP_ID` / `WX_APP_SECRET` | 仅小程序需要 | 仅小程序需要 | code2Session;web-only 可留空(wx 端点会 502 直到配置) |
| `MYSQL_ROOT_PASSWORD` / `DB_PASSWORD` | dev 固定 | **必填** | MySQL 密码 |
| `ICP_NUMBER` | 可空 | 可空 | 登录页底部备案号(构建期注入) |
| `DOCS_URL` | 可空 | 可空 | 登录页文档链接(设了才显示) |

生产用 `docker-compose.prod.yml`,对 `DB_PASSWORD`/`MYSQL_ROOT_PASSWORD`/`JWT_SECRET`/`ADMIN_PASSWORD` 做 fail-fast(缺则拒启)。

## 各端单独跑

### 后端(Go)

```bash
cd backend
APP_ENV=development go run .     # 或 air 热更
```

需 MySQL 可达(`DB_HOST` 等默认指向 localhost:3306,dev 凭据 ocm/ocm)。`APP_ENV=development` 允许 `DB_HOST` 缺省回落 localhost;生产模式拒绝。

### Web 控制台

```bash
cd web && npm install && npm run dev     # http://localhost:5173
```

`/api` 由 Vite 代理到 `VITE_API_PROXY_TARGET`(默认 `http://localhost:8080`)。

### 小程序

1. 微信开发者工具打开 `miniapp/`(不是仓库根)。
2.「工具 → 构建 npm」生成 `miniprogram_npm/`。
3. 起后端(:8080)。开发者工具里 `envVersion=develop` 自动走 `http` → `http://localhost:8080`,无需改配置。
4.「详情 → 本地设置 → 不校验合法域名」放行 localhost;真机把 `devConfig.baseUrl` 的 `localhost` 换局域网 IP。

详见 [小程序](/guide/miniapp)。

## 生产部署

```bash
docker compose -f docker-compose.prod.yml --env-file .env up -d
```

- 后端 `:8080`(`Dockerfile`,non-root alpine),web `:80`(nginx,`/api` 代理到 `backend:8080`,SPA fallback),mysql 自带。
- Web 静态站可单独部署到腾讯云 EdgeOne Pages:`web/edgeone.json`(SPA 重写)+ `web/edge-functions/api/` 边缘函数反代 `/api` 到后端(设 `API_ORIGIN`)。详见 [Web 端](/guide/web#edgeone-部署)。

## 健康检查

- `GET /healthz` —— 进程存活(不查 DB,开库前即注册)。
- `GET /readyz` —— 就绪(ping DB,不可达 503,开库后注册)。

## 下一步

- [项目结构](/guide/structure) —— 仓库布局。
- [后端](/guide/backend) —— 路由、鉴权、迁移、并发控制。
- [小程序](/guide/miniapp) —— 传输切换、请求层、身份流程。
- [Web 端](/guide/web) —— 技术栈、鉴权、EdgeOne。
- [课表导入](/guide/import) —— xlsx 契约与异步导入流程。
- [API 概述](/api) —— 完整路由表。
