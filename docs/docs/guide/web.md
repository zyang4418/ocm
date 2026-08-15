---
title: Web 端
---

Web 控制台。React 19 + Vite + IBM **Carbon Design System**,纯 JSX(无 TypeScript),无状态库。前端一律用**同源相对路径** `/api/...` 调后端——开发由 Vite proxy 转发,生产由 nginx 转发,EdgeOne 由边缘函数转发,因此**没有客户端 API base URL**。

## 技术栈

- React 19 + `react-router-dom` 7 + Vite 8 + `@vitejs/plugin-react`
- UI 库:`@carbon/react`(IBM Carbon Design System)+ `@carbon/icons-react`,字体 IBM Plex Sans
- Sass 样式(`src/app.scss` 全局),Carbon 主题 `g10`
- **无 TypeScript**(全部 `.jsx`);**无外部状态库**(仅 `AuthContext` + 各页 `useState`);无 i18n(字符串硬编码)
- Node ≥ 20.19

## 脚本

`web/package.json` 只有三个脚本,无 lint/test:

| 脚本 | 命令 | 用途 |
|------|------|------|
| `dev` | `vite` | 开发服务器(:5173) |
| `build` | `vite build` | 生产构建 |
| `preview` | `vite preview` | 预览构建产物 |

## 本地运行

```bash
cd web
npm install
npm run dev          # http://localhost:5173
```

`/api` 由 Vite dev-server 代理(`vite.config.js`),目标取 `VITE_API_PROXY_TARGET`,默认 `http://localhost:8080`。在 `docker compose` 里指向 `http://backend:8080`。

## 目录结构(`web/src/`)

- `main.jsx` — 入口,挂载 `<App/>`,引入 IBM Plex Sans 字重与全局 `app.scss`。
- `App.jsx` — `BrowserRouter` + `RequireAuth`(未登录重定向到 `/login`)+ `AppShell` 布局;路由表见下。
- `auth/api.js` — 三个助手:`apiFetch`(JSON,每次按调用附 `Authorization: Bearer`)、`apiUpload`(multipart,不设 Content-Type 让浏览器加 boundary)、`apiDownload`(解析 `Content-Disposition` 触发下载)。**同源相对路径**,无全局 fetch 拦截器。
- `auth/AuthContext.jsx` — 唯一状态层,提供 `token`/`user`/`login`/`logout`/`bootstrapping`/`can`。
- `components/AppShell.jsx` — Carbon UI Shell(顶栏 + 侧边栏 + 内容区)。
- `pages/` — 各业务页面。

## 路由

| 路径 | 页面 | 说明 |
|------|------|------|
| `/login` | LoginPage | 公开;两步登录(用户名 → 密码) |
| `/` | DashboardPage | 仪表盘(需登录) |
| `/classrooms` | ClassroomsPage | 教室管理 |
| `/bookings` | BookingsPage | 教室预约 |
| `/courses` | CourseManagementPage | 课程管理 |
| `/schedule-config` | ScheduleConfigPage | 作息制度配置 |
| `/timetable` | TimetablePage | 课表 |
| `/imports` | ImportsPage | 数据导入(持有 `course:manage` 等导入权限者可见) |
| `/users` | UsersPage | 用户管理(持有 `user:read` 可见、`user:manage` 可操作) |
| `/roles` | RolesPage | 角色管理(`role:manage`) |
| `/groups` | GroupsPage | 用户组管理(`group:manage`) |
| `/admin-classes` | AdminClassesPage | 行政班 |
| `/teaching-classes` | TeachingClassesPage | 教学班 |

## 鉴权

- 登录:`POST /api/auth/login` `{username, password}` → 后端返回 `{token, user}`,存入 `localStorage`(键 `ocm.token`)。
- 启动水合:若 `localStorage` 有 token,调 `GET /api/auth/me` 取回 `user`;401 弃 token,5xx/网络错误**保留** token 以便刷新重试。
- **无 refresh token**;会话中遇 401 **不自动登出**,页面直接显示错误文案。
- token 按**每次请求**附 `Authorization: Bearer <token>`(`api.js`),无全局拦截器。

## 权限(重要)

客户端**只做可见性控制**,不做安全边界:

- 登录/水化响应携带 `user.permissions`(有效权限数组,通配为 `["*"]`),`AuthContext` 暴露 `can(perm)`(通配感知)。侧边栏与各页写操作按钮按 `can(...)` 门控:教室→`classroom:manage`、预约审批→`booking:approve`、课程/作息/课表/导入→`course:manage`、用户→`user:read`/`user:manage`、角色→`role:manage`、用户组→`group:manage`、行政班→`admin_class:manage`、教学班→`teaching_class:manage`。
- **没有路由级权限守卫**:任意已登录用户都能渲染路由组件,页面按权限收敛按钮,后端 403 时显示错误文案。
- **后端才是鉴权权威**——每个写操作端点都由后端校验权限。贡献者不要以为路由能保护管理页;新增管理功能必须在后端加权限校验。
- 权限/角色/用户组是数据而非代码:管理员可在控制台创建自定义角色、给用户或用户组授权(可带有效期),无需改前端。

## 数据导入(`ImportsPage`,路由 `/imports`)

- 支持 **8 种**导入类型:`sessions`(课次,默认)、`classrooms`、`admin_classes`、`teaching_classes`、`catalog`、`offerings`、`regimes`、`bookings`。
- **异步预览→提交流程**:上传 → 后端建 job → `preview` 态弹出明细 →「确认导入」(`POST /api/imports/{id}/commit`)或「取消」。
- **轮询**:仅当存在 `pending`/`processing` 的 job 时,每 **3 秒**轮询一次,无活动则停。
- 结果展示:成功/失败/总行数;`failedRows > 0` 或 `failed` 时可「查看明细」,逐行列出错误。
- **教务处课表拆分**(`jwc_split`):上传一个聚合 `.xlsx` + `semester` + `week1_monday` → `POST /api/imports/jwc_split`,后端拆成 6 个 job,按依赖顺序提交:**教室 → 课程库 → 行政班 → 教学班 → 开课 → 课次**。

## EdgeOne 部署

两份产物,用于腾讯云 EdgeOne Pages 托管:

- `web/edgeone.json` — SPA 重写规则 `/* → /index.html`,让客户端路由在静态部署上可用。
- `web/edge-functions/api/[[default]].js` — 边缘函数,反向代理 `/api/*` 到后端,使前端能用同源相对路径、免 CORS。后端 origin 取自 `context.env.API_ORIGIN`(在 EdgeOne 项目设置里配,非仓库内);未设返回 `502 {"error":"后端地址未配置(API_ORIGIN 未设置)"}`。函数会规范化 origin 尾斜杠(避免 ServeMux 301 死循环),并删除 `host` 头让上游用目标 host。

## Docker

- **`Dockerfile`**(生产,多阶段):`node:24-alpine` 跑 `npm run build`,`nginx:stable-alpine` 运行;`:80`;`/api/` 代理到 `http://backend:8080`,`location /` 用 `try_files $uri $uri/ /index.html` 做 SPA fallback。构建期注入 `VITE_ICP_NUMBER`/`VITE_DOCS_URL`。
- **`Dockerfile.dev`**:`node:24-alpine`,`:5173`,`npm run dev -- --host 0.0.0.0`;源码由 compose 挂载。

## 环境变量

| 变量 | 读取处 | 用途 | 默认 |
|------|--------|------|------|
| `VITE_API_PROXY_TARGET` | `vite.config.js`(Node `process.env`,**非** `import.meta.env`) | dev 下 `/api` 代理目标 | `http://localhost:8080` |
| `VITE_ICP_NUMBER` | `import.meta.env`(构建期内联) | 登录页底部备案号(链 `beian.miit.gov.cn`) | 空(不显示) |
| `VITE_DOCS_URL` | `import.meta.env`(构建期内联) | 外部文档站地址;设了才在登录页显示「Docs」链接 | 空(隐藏) |
| `API_ORIGIN` | EdgeOne 边缘函数 `context.env` | 边缘函数反代的后端 origin | 无(未设返回 502) |

注意区分:`VITE_API_PROXY_TARGET` 是 **Node 侧**变量(在 `vite.config.js` 里用 `process.env`);`VITE_ICP_NUMBER`/`VITE_DOCS_URL` 是**客户端**变量(用 `import.meta.env`,由 `vite build` 内联进 bundle);`API_ORIGIN` 是 EdgeOne 服务端变量。
