---
title: 小程序
---

微信小程序客户端。TypeScript + glass-easel + TDesign 组件库,**默认 WebView 渲染**,Skyline 仅在性能关键页按页开启(当前未开)。通信层抽象了微信云托管与自托管两种传输,身份基于 `wx.login` + 后端 `code2Session` 解析 openid。

## 工程约定

- `app.js` 为纯 JS(`require`),避免 `app.ts`/`app.js` 冲突;`utils/`、`config/` 为 `.ts`(ES 导出,编译为 CommonJS)。
- `componentFramework: "glass-easel"`(全局,`app.json`)。
- `navigationStyle: "custom"`,每页手写自定义导航栏。
- 设计 token 定义在 `app.wxss`(`page` 选择器上的 CSS 变量)。

## 渲染策略

- **不在 `app.json` 设全局 `"renderer": "skyline"`**;`rendererOptions.skyline` 只是声明"开 Skyline 时的选项",不等于开启。
- Skyline 仅在性能关键页按页开启:在该页 `page.json` 加 `"renderer": "skyline"`,**必须用 `scroll-view` 承载滚动内容**(Skyline 无页面级滚动),并保持 `navigationStyle: custom`。
- 当前 6 个页面**均为 WebView**(静态占位数据,无性能压力)。`index`/`ai` 接入真实大列表后再按页评估开启 Skyline。
- 低版本微信/PC 端会自动 fallback 到 WebView,按页开启是安全的。
- TDesign 对 Skyline 支持是部分的(Cascader/Calendar 在 Skyline 下不可用),默认 WebView 可用完整组件库。选型背景见仓库根 `agents/TDesign-Skyline-兼容性调查报告.md`。

## 页面

`app.json` 声明 6 个页面,tabBar 5 个(首页/控制台/AI助手/消息/我的),`login` 不在 tabBar。

| 页 | 路径 | 用途 | 状态 |
|----|------|------|------|
| 首页 | `pages/index/index` | 工作台:扫码、搜索、快捷入口、功能宫格、今日课程 | 静态占位(待接 API) |
| 登录 | `pages/login/login` | 账号绑定(静默登录→用户名密码绑定表单) | 可用 |
| 控制台 | `pages/console/console` | 控制台 | 占位(空) |
| AI 助手 | `pages/ai/ai` | 聊天输入 + 建议提示词 | 占位(仅 toast,无后端) |
| 消息 | `pages/messages/messages` | 消息列表 | 占位(空) |
| 我的 | `pages/profile/profile` | 资料/统计/菜单/清缓存/解绑退出 | 可用 |

## 通信与传输

配置在 `miniapp/config/api.ts`。`ApiConfig` 字段:`transport`(`callContainer` | `http`)、`cloudEnv`(云环境 id,callContainer 用)、`serviceName`(作为 `X-WX-SERVICE` 头,callContainer 用)、`baseUrl`(后端地址,http 用)、`timeout`。

- **devConfig**:`transport: 'http'`,`baseUrl: 'http://localhost:8080'`。
- **prodConfig**:`transport: 'callContainer'`,`cloudEnv`/`serviceName` 填真实云托管值(当前为占位 TODO)。
- **自动切换**:`resolveConfig` 读 `wx.getAccountInfoSync().miniProgram.envVersion`——`develop`(开发者工具)走 `devConfig`,`trial`/`release`(真机)走 `prodConfig`。
- **强制覆盖**:`FORCE_TRANSPORT`(设 `'http'` 或 `'callContainer'`)可无视 `envVersion` 全局强制,便于在工具里测云端传输。

### 自托管(非微信云)

仅改配置、不改代码:把 `FORCE_TRANSPORT` 设为 `'http'`,并把 `devConfig.baseUrl` 改成你的后端地址(无尾斜杠)。`localhost` 需在开发者工具勾选「详情 → 本地设置 → 不校验合法域名」;真实域名需加入微信「request 合法域名」白名单。身份机制(`wx.login`/`code2Session`)非云专属,自托管同样可用。

## 请求层

`miniapp/utils/request.ts` 导出 `request<T>(opts)`,在 `callContainer` 与 `wx.request` 之上做了统一 Promise 封装:

- `opts`: `path`、`method`、`data`(体)、`params`(查询串,自动 URL 编码)、`header`、`auth`(默认 `true`,附 JWT)、`timeout`。
- `auth !== false` 时:从 `getToken()` 取 JWT,设 `Authorization: Bearer <token>`;`callContainer` 分支额外附 `X-WX-SERVICE` 头。默认 `Content-Type: application/json`。
- 网络层失败:reject `{ statusCode: 0, message: '网络异常,请检查网络后重试' }`。
- HTTP 401 且 `auth !== false`:触发已注册的 `onUnauthorized`(由 `auth.ts` 接到 `clearAuth` + `wx.reLaunch('/pages/login/login')`)。
- 非 2xx:抛 `ApiError { statusCode, message, data }`,`message` 取自 `data.error` 或 `data` 或默认 `'请求失败'`。
- 2xx:resolve `data`。

## 身份与鉴权

客户端**只持有 JWT**(storage key `token`)和 `user` 对象(key `user`),**从不接触 openid**——openid 由后端用 AppSecret 调 `code2Session` 解析,AppSecret 只在后端 env。

| 动作 | 流程 | 端点 |
|------|------|------|
| 静默登录 | `wx.login` 取 `code` → POST `{ code }`(`auth: false`)→ 后端 code2Session 解析 openid → 返回 `{ token, user }` | `POST /api/auth/wx-login` |
| 绑定账号 | `wx.login` + 用户名密码 → POST `{ username, password, code }`(`auth: false`)。首次绑定,openid 必须未绑过 | `POST /api/auth/wx-bind` |
| 解绑 | POST(需鉴权) | `POST /api/auth/wx-unbind` |
| 页面守卫 | `ensureAuth()`:有 token 即放行;否则尝试静默登录;遇 404(openid 未绑)→ `reLaunch` 到 `login?notBound=1` | 客户端逻辑 |

安全约束:后端因网页端需要**必须公网暴露**,故 `X-WX-OPENID` 云网关头**可伪造、不可信任**;openid 只信后端 `code2Session` 的结果。绑定规则:账号 `openid` 必须为 NULL 才允许绑定(防劫持),唯一索引防一个 openid 绑多账号。JWT 为 HS256、24h,网页端与小程序共用,`Authorization: Bearer <token>`。

## 设计 token

`miniapp/app.wxss` 在 `page` 上定义全套 CSS 变量:

- 品牌色:`--color-primary: #2B5FF6`,并绑定 `--td-brand-color: #2B5FF6`(TDesign 品牌色与设计 token 一致)。
- 色彩:`--color-primary-light`、`--color-accent`、`--color-cta`(用于解绑确认等)、`--color-bg`、`--color-card`、`--color-border`、`--color-text` 及二级/三级文本色。
- 圆角:`--radius-sm 8px` / `--radius-md 12px` / `--radius-lg 20px`。
- 字号(rpx):`--font-xs` … `--font-xl`(22/24/28/34/44rpx)。
- 间距:`--space-xs` … `--space-lg`(8/12/16/20px)。
- `.safe-padding-top` / `.safe-padding-bottom` 基于 `env(safe-area-inset-*)`,适配刘海/底部横条。

## TDesign 组件库

- 依赖 `tdesign-miniprogram`(npm),按页在 `page.json` 的 `usingComponents` 引入。`lazyCodeLoading: "requiredComponents"` 已开,未用组件自动裁剪。
- 改 `miniapp/package.json` 后,**必须在微信开发者工具「工具 → 构建 npm」生成 `miniprogram_npm/`**,否则组件无法解析。
- TDesign 品牌色用 `--td-brand-color`(已在 `app.wxss` 绑定到 `--color-primary`)。
- TDesign 主要面向 WebView;在 Skyline 页用之前先查兼容性。日期/时间选择用原生 `<picker mode="date|time">`(原生组件不受渲染引擎影响)。

## 本地运行

1. 微信开发者工具打开 `miniapp/` 目录(小程序工程根是 `miniapp/`,不是仓库根)。
2. Appid 在 `project.config.json`(现填 `wx1d0666f34a25b850`;无权限则换成你自己的)。
3. 「工具 → 构建 npm」生成 `miniprogram_npm/`。
4. 起后端(默认 `:8080`;见 [快速开始](/))。
5. 开发者工具里 `envVersion === 'develop'` 自动走 `http` → `http://localhost:8080`,**无需改任何配置**。
6. 「详情 → 本地设置 → 不校验合法域名」放行 localhost;真机 Wi-Fi 调试把 `devConfig.baseUrl` 的 `localhost` 换成局域网 IP。
7. 测云端传输:设 `FORCE_TRANSPORT = 'callContainer'` 并在 `prodConfig` 填真实 `cloudEnv`/`serviceName`。

## 数据状态

无 mock 开关。各页 `data` 内为静态占位值(如 `index.ts` 的 `gridItems`/`todayClasses`、`profile.ts` 的 `stats`、`ai.ts` 的 `suggestions`),带显式 TODO。`index`/`ai` 接入真实 API(尤其大列表)后,再按页评估开启 Skyline。
