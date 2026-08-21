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
- 权限链:`auth.Middleware`(JWT→username)→ `user.LoadSubject`(每请求重算)→ `authz.RequirePermission`/`RequireAny`。RBAC 数据(`roles`/`user_roles`/`user_permissions`/`user_groups` 等)在 `internal/iam`;**有效权限 = 直接角色授权 ∪ 组角色授权 ∪ 直接权限授权**,授权可带 `expires_at` 过期;`*` 通配仅系统 admin 角色持有,不在权限目录(API 无法授予);授权 admin 角色需操作者持有 `*`。权限目录在代码(`authz.Catalog`),DB 只存权限字符串。handler 检查 permission 字符串,不检查角色名。

## OpenAPI 契约(swaggo)

- handler 注释用 swaggo 注解(`@Summary/@Param/@Success/@Router`,试点见 `backend/internal/attendance/handler.go`);`backend/docs/` 为生成产物勿手改。**全部业务模块已注解**(导出 xlsx/docx 的端点除外——二进制响应无 schema 意义)。
- 改 handler/model 后须 `go -C backend tool swag init -o docs --parseDependency --parseInternal --requiredByDefault` 并提交;前端 `npm --prefix web run gen:types` 重生成 `web/src/types/api.d.ts`(链路:swagger 2.0 -> swagger2openapi 转 OpenAPI 3 -> openapi-typescript)。Swagger UI 仅在 `APP_ENV=development`(或显式设 `SWAGGER_ENABLED=true`)时挂载在 `/swagger/`,生产默认关闭(未设 APP_ENV 按 production 处理)。CI 有双侧漂移门禁(backend.yml 重生成 `docs/` 无 diff;web.yml 重生成 `api.d.ts` 无 diff)。
- `--requiredByDefault` 把非 omitempty 字段标为必填(TS 端必到字段不再是全可选);**可空指针字段**(如 `*time.Time`)须加 `validate:"optional"` 标签豁免(后端无 validator 库,该标签仅 swaggo 消费),否则会被误标必填。
- `json.RawMessage` 字段须加 `swaggertype` 覆盖,否则 swaggo 渲染成 `number[]`(底层 []byte):任意 JSON 对象用 `swaggertype:"object"`,字符串数组用 `swaggertype:"array,string"`(见 observation/model.go、classroom/repair.go)。
- 注解引用的请求/响应类型须为**导出类型**(swaggo 限制);未导出的先导出化(先例:auth 的 `LoginRequest/UserView/LoginResponse`、ai/mail/storage 的 `MaskedSettings`),或新建命名类型(先例:attendance `ScanRequest`、user `UserRolesInput`、importer `SplitResult`)。

## 网页端(web/)

- **TypeScript strict 全量落地**:src 下无 `.js/.jsx`(`allowJs` 已关),新文件一律 `.ts/.tsx`。
- 页面数据类型一律引 `web/src/types/api.ts` 的导出别名(生成 schema 的 re-export);手写补集仅限 `ApiError`/`Paged<T>`/`Permission`(前端 `can()` 的联合类型,比生成的 `string[]` 严)与极少数非 Go 源头的类型(如 observations 模板 schema——Python 衍生,见 ObservationsPage 本地类型)。
- Carbon DataTable 行必须有 string `id`:wire 无 id 的记录显式派生(`{...r, id: String(r.userId)}`),这是类型锁死的模式。`getHeaderProps`/`getRowProps` spread 自带 key,勿再传显式 `key`(TS2783)。
- Carbon 组件常见坑:Tag 无 `yellow` 色;`DatePickerInput` 不收 `value`(受控值放父 `DatePicker`);`MultiSelect` 无 `selection` prop,弹窗内表单回显必须用受控 `selectedItems`——Carbon Modal 的 children 常驻挂载(仅切 `is-visible` class,不随 open 重挂载),`initialSelectedItems` 仅组件挂载时生效,弹窗打开后异步加载的数据它不感知;`Dropdown` 的 `titleText` 为必填 prop,非空会渲染可见 `<label>`(页面已有自定义 label 时传 `""`,组件默认即空串、空值短路不渲染);`InlineNotification` 无 `actions` prop(用 `ActionableNotification`);charts 的 `scaleType` 须用 `ScaleTypes` 枚举(字符串字面量不过检)。
- `npm --prefix web run typecheck` 为类型门禁;`build` 已含 `tsc --noEmit`。
- 本地无 MySQL:后端业务路由等 DB 连通后才注册,冒烟看 `/healthz` 与 `/swagger/`;本地启动后端需 `APP_ENV=development`(注意不是 dev)。
