# OCM - 智慧教室管理平台

Classroom management system: Go backend + WeChat mini-program (小程序) + web console.

## 小程序渲染策略(重要)

- **默认 WebView 渲染**。不在 app.json 设置全局 `"renderer": "skyline"`。
- **Skyline 仅在性能关键页按页开启**:长列表、重滚动、worklet 动画等场景。开启方式:在该页 page.json 加 `"renderer": "skyline"`(`componentFramework: glass-easel` 已在 app.json 全局设置,继承即可)。
- 开 Skyline 的页面**必须用 `scroll-view` 承载滚动内容**(Skyline 无页面级滚动),并保持 `navigationStyle: custom`。
- 低版本微信/PC 端会自动 fallback 到 WebView,按页开启是安全的。
- **当前所有页面均为 WebView**(mock 数据,无性能压力)。index/ai 接入真实大列表后,再按页评估开启 Skyline。
- 选型背景见 `TDesign-Skyline-兼容性调查报告.md`:TDesign 对 Skyline 支持是部分的,Cascader/Calendar 在 Skyline 下不可用,故默认 WebView 以获得完整组件库。

## TDesign 组件库

- 使用 `tdesign-miniprogram`(npm)。改 `miniapp/package.json` 后,须在微信开发者工具「工具 → 构建 npm」生成 `miniprogram_npm/`,否则组件无法解析。
- 组件按页在 page.json `usingComponents` 引入(`lazyCodeLoading: "requiredComponents"` 已开,未用组件自动裁剪)。
- TDesign 品牌色用 `--td-brand-color`(在 `app.wxss` 绑定到 `--color-primary`),保持与设计 token 一致。
- TDesign 主要面向 WebView;在 Skyline 页用之前先查兼容性。日期/时间选择用原生 `<picker mode="date|time">`(原生组件不受渲染引擎影响)。

## 安全约束(鉴权)

- 后端因网页端需要**必须公网暴露**;因此 `X-WX-OPENID` 云网关头**可伪造、不可信任**。
- 小程序身份通过 `wx.login()` + 后端 `code2Session`(AppSecret 仅在后端 env)解析 openid,绝不取自请求头。
- 绑定:账号 `openid` 必须为 NULL 才允许绑定(防劫持);唯一索引防一个 openid 绑多账号。
- JWT(HS256,24h)网页端与小程序共用;`Authorization: Bearer <token>`。

## 通信与自托管

- 小程序通过 `wx.cloud.callContainer` 访问云托管后端(仅传输)。`config.env` 每次调用必填,`X-WX-SERVICE` 头指定服务名;`wx.cloud.init()` 空参即可。
- 请求层 `miniapp/utils/request.ts` 抽象传输(callContainer / wx.request),`miniapp/config/api.ts` 配置切换。**自托管(非微信云)仅改配置、不改代码**:把 `transport` 改 `'http'` + 填 `baseUrl`。
- 身份机制(wx.login/code2Session)非云专属,自托管同样可用。

## 小程序工程约定

- TypeScript + glass-easel。`app.js` 为纯 JS(require)以避免 app.ts/app.js 冲突;`utils/`、`config/` 为 .ts(ES 导出,编译为 CommonJS)。
- 设计 token 在 `app.wxss`(`page` 上的 CSS 变量,如 `--color-primary: #2B5FF6`)。
- `navigationStyle: custom`,每页手写自定义导航栏(未来可统一为 t-navbar)。

## 后端(Go)

- `net/http` ServeMux(Go 1.22+ 方法路由)。认证在 `backend/internal/auth`。
- MySQL 幂等迁移:ALTER 忽略 1060/1061/1062(见 importer/store.go 模式)。
- 权限链:`auth.Middleware`(JWT→username)→ `user.LoadSubject` → `authz.RequirePermission`。角色 admin(通配)/ user。
