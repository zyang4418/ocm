# TDesign 微信小程序 Skyline 渲染引擎兼容性调查报告

> 调研时间：2026-08-08  
> TDesign 小程序版最新版本：**v1.16.0**（2026-08-06 发布）  
> Skyline 最新版本：**v1.4.20**（2026-06-10 发布）

---

## 一、总体结论

**TDesign 小程序组件库对 Skyline 的支持处于"大部分适配、持续追赶"阶段。** 截至 2026 年 8 月，约 **60%+ 的组件已完成 Skyline 适配**，核心基础组件（Button、Input、Form、Toast、Popup 等）可用，但仍有不少中高级组件（Calendar、Cascader、DateTimePicker 等）存在兼容性问题或尚未适配。**如果你项目主要使用基础表单 + 展示类组件，可以尝试 Skyline；如果重度依赖 Picker/Cascader/Calendar/Dialog 等复杂交互组件，建议谨慎评估。**

TDesign 团队在持续投入 Skyline 适配，每版 release 基本都有 Skyline 相关的修复，最新 v1.16.0（仅 2 天前发布）刚完成了 **Form 组件的 Skyline 适配**以及 **glass-easel 组件关系迁移**。

---

## 二、适配历史与进度

### 2.1 里程碑

| 时间 | 版本 | 事件 |
|------|------|------|
| 2024-05 | v1.4.0 | **首批 33 个组件完成 Skyline 适配**（PR #2659，占全体 58%） |
| 2024-06 | v1.4.1 | 修复 Tabs Skyline 下无法使用 |
| 2024-12 | v1.6.2 | 修复 Tabs Skyline 下无法滚动 |
| 2025-08 | v1.10.1 | Textarea 新增 Skyline 专有属性 `cursorColor` |
| 2025-09 | v1.11.0 | 修复 Input（Skyline + nickname）、TabBar Skyline change 事件 |
| 2026-04 | v1.14.0 | 新增 Segmented、Table 组件 |
| 2026-08 | **v1.16.0** | **Form 组件 Skyline 适配**；Cell/DropdownMenu/Indexes/Layout/Picker/SideBar/Steps/Swiper 组件关系迁移为 `ancestor/descendant`（glass-easel 兼容） |

### 2.2 官方追踪 Issue

GitHub Issue [#3149](https://github.com/Tencent/tdesign-miniprogram/issues/3149) `🚀 Skyline：组件适配进度` 是官方公开的适配追踪入口，由核心维护者 **@anlyyao** 管理。该 issue 最近一次更新标注为 **累计 35 个组件，占全体组件的 61%（35/57）**，但实际随 v1.16.0 Form 组件完成适配，已完成组件数已超过此数字。

---

## 三、已适配组件清单（首批 33 个 + 后续增量）

### ✅ 已确认适配（基础类 7 个）

| 组件 | 备注 |
|------|------|
| **Button** 按钮 | 早期已适配。已知问题：Skyline 不支持 `inline-flex`，按钮图标可能错位（#2912），需手动改为 `display: flex` |
| **Divider** 分割线 | 正常 |
| **Fab** 悬浮按钮 | 正常 |
| **Icon** 图标 | 正常 |
| **Layout** 布局 | ⚠️ **已知：Skyline 下有精度问题，可能出现非预期换行** |
| **Link** 链接 | 正常 |
| **Typography** 排版 | v1.15.0 新增，待确认 |

### ✅ 已确认适配（导航类 8 个）

| 组件 | 备注 |
|------|------|
| **BackTop** 返回顶部 | 正常 |
| **Drawer** 抽屉 | 正常 |
| **Navbar** 导航栏 | 正常（Skyline 不支持原生导航栏，必须用自定义导航栏） |
| **Steps** 步骤条 | 正常；v1.16.0 完成 glass-easel 关系迁移 |
| **TabBar** 标签栏 | 正常；v1.11.0 修复 change 事件；Skyline 需补充 `pointer-events: auto` 样式 |
| **Tabs** 选项卡 | 已修复多次（v1.4.1、v1.6.2），基本可用 |
| **Indexes** 索引 | v1.16.0 完成 glass-easel 关系迁移 |
| **SideBar** 侧边导航栏 | v1.16.0 完成 glass-easel 关系迁移 |

### ✅ 已确认适配（输入类 16 个）

| 组件 | 备注 |
|------|------|
| **CheckBox** 多选框 | 正常 |
| **Input** 输入框 | 正常；v1.11.0 修复 Skyline + nickname 场景 |
| **Radio** 单选框 | 正常 |
| **Stepper** 步进器 | 正常 |
| **Textarea** 多行文本框 | 正常；v1.10.1 新增 Skyline 专有 `cursorColor` |
| **Switch** 开关 | 基本可用 |
| **Slider** 滑动选择器 | 基本可用 |
| **Rate** 评分 | 待确认 |
| **Search** 搜索框 | 待确认 |
| **Upload** 上传 | 基本可用 |
| **Calendar** 日历 | ❓ **未明确适配，社区反馈有问题** |
| **Cascader** 级联选择器 | ❌ **确认未适配**（#3854），二次打开无法选值 |
| **ColorPicker** 颜色选择器 | 待确认 |
| **DateTimePicker** 日期选择器 | 待确认 |
| **Picker** 选择器 | v1.16.0 完成 glass-easel 关系迁移，功能可用性待验证 |
| **TreeSelect** 树形选择器 | 待确认 |

### ✅ 已确认适配（数据展示类 20 个）

| 组件 | 备注 |
|------|------|
| **Avatar** 头像 | 正常 |
| **Badge** 徽标 | 正常 |
| **Cell** 单元格 | 正常；v1.16.0 完成 glass-easel 关系迁移 |
| **CountDown** 倒计时 | 正常 |
| **Empty** 空状态 | 正常 |
| **Footer** 页脚 | 正常 |
| **Image** 图片 | 正常 |
| **ImageViewer** 图片预览 | 正常 |
| **Progress** 进度条 | 正常 |
| **Result** 结果 | 正常 |
| **Skeleton** 骨架屏 | 正常 |
| **Swiper** 轮播图 | 正常；v1.16.0 完成 glass-easel 关系迁移 |
| **Tag** 标签 | 正常 |
| **Collapse** 折叠面板 | ⚠️ **动画在 Skyline 下有问题**（展开/收起卡顿，高度计算异常） |
| **Grid** 宫格 | 待确认 |
| **QRCode** 二维码 | 基本可用 |
| **Segmented** 分段控制器 | v1.14.0 新增 |
| **Sticky** 吸顶容器 | 待确认 |
| **Table** 表格 | v1.14.0 新增，有 Skyline label 的 open issue |
| **Watermark** 水印 | v1.11.0 新增，待确认 |

### ✅ 已确认适配（反馈类 12 个）

| 组件 | 备注 |
|------|------|
| **Loading** 加载 | 正常 |
| **Overlay** 遮罩层 | 正常 |
| **Popup** 弹出层 | 正常 |
| **Toast** 轻提示 | ⚠️ **circular loading 动画在 Skyline 下不转圈**；`fit-content` 不支持 |
| **ActionSheet** 动作面板 | 待确认 |
| **Dialog** 对话框 | 基本可用 |
| **DropdownMenu** 下拉菜单 | v1.16.0 完成 glass-easel 关系迁移 |
| **Guide** 引导 | 待确认 |
| **Message** 全局提示 | 基本可用 |
| **NoticeBar** 消息提醒 | 基本可用 |
| **Popover** 弹出气泡 | 基本可用 |
| **PullDownRefresh** 下拉刷新 | 基本可用 |
| **SwipeCell** 滑动操作 | 待确认 |

### 🆕 v1.16.0 新增适配

| 组件 | 变更内容 |
|------|----------|
| **Form** 表单 | 🎉 **首次完成 Skyline 适配**（#4583） |
| Cell / DropdownMenu / Indexes / Layout / Picker / SideBar / Steps / Swiper | 统一组件关系类型为 `ancestor/descendant`，支持 glass-easel 复合嵌套场景（#4589） |

---

## 四、已知问题汇总

### 4.1 CSS 兼容性根本差异

Skyline 的 WXSS 支持是 WebView 的**子集**，以下 CSS 特性在 Skyline 中**不支持**：

| CSS 特性 | 影响 |
|----------|------|
| `display: inline-flex` | Button 组件图标文字错位（#2912） |
| `display: inline` | 多段文本无法内联，需改用 flex 布局 |
| `fit-content` | Toast 最大宽度不生效（#3813） |
| `position: fixed` | 行为变化，需额外处理层级 |
| 原生导航栏 | 不支持，必须用自定义导航栏 |
| 页面全局滚动 | 不支持，必须改用 `scroll-view` |

### 4.2 组件特定已知 Bug

| 组件 | 问题 | 状态 | Issue |
|------|------|------|-------|
| **Button** | `inline-flex` 不支持，图标文字错位 | 等 iOS 微信更新 | #2912 |
| **Toast** | circular loading 动画真机不转圈 | 有替代方案（改用 `<t-loading>` 组件） | CSDN 讨论 |
| **Toast** | `fit-content` 不支持，最大宽度失效 | 未完全修复 | #3813 |
| **Collapse** | 展开/收起动画卡顿，高度计算异常 | 社区有深度兼容方案（CSDN） | — |
| **Cascader** | 二次打开无法重新选值 | **仍未适配** | #3854 |
| **Calendar** | 未适配，community 反馈 CSS 改造困难 | 未修复 | #3149 讨论 |
| **TabBar** | 真机 change 事件失效（已修复于 v1.11.0） | ✅ 已修复 | #3924 |
| **Input** | Skyline + nickname 类型 change 事件失效（已修复） | ✅ 已修复 | #3855 |
| **Layout** | 精度问题导致非预期换行 | 已知但未修复 | PR #2659 备注 |
| **Tabs** | 多次修复历程（v1.4.1, v1.6.2） | 基本可用 | — |
| **Table** | 单元格内容无法选中复制、省略号不显示 tip | Open | #4452 |

### 4.3 动画与交互差异

- **`wx.createAnimation`** API 在 Skyline 下行为不同：不支持 `height: auto` 动画过渡，连续 `step()` 调用可能混乱
- Skyline 推荐使用 **worklet 动画机制** 替代传统动画 API
- 离屏渲染模式下，`getRect`（获取元素尺寸）可能返回 0 或错误值

### 4.4 glass-easel 组件框架兼容

Skyline 强制使用 `glass-easel` 组件框架，组件间关系必须使用 `ancestor/descendant` 类型，而非旧的 `parent/child`。v1.16.0 刚刚完成了一批组件的迁移。

---

## 五、社区实践案例

### 5.1 真实项目参考

| 项目 | 技术栈 | 说明 |
|------|--------|------|
| [wzkris-mini](https://gitee.com/wzkris/wzkris-mini) | TypeScript + SCSS + TDesign v1.11.2 + Skyline | 企业级小程序，含认证体系、请求封装、状态管理，实际运行在 Skyline 上 |
| [weapp-template](https://cnb.cool/lihejia/weapp-template) | TypeScript + Less + TDesign + Skyline + 云开发 | 开源脚手架模板，标榜"双架构设计"，Skyline 开箱即用 |

### 5.2 社区提供的兼容方案

TDesign 仓库 `example/behaviors/skyline.js` 提供了运行时检测机制：

```javascript
module.exports = Behavior({
  data: { skylineRender: false },
  lifetimes: {
    created() {
      this.setData({
        skylineRender: this.renderer === 'skyline'
      });
    }
  }
});
```

社区（CSDN 等）有开发者分享了针对 Collapse 等组件的深度兼容补丁方案，通过检测 `skylineRender` 变量走不同的动画实现路径。

---

## 六、是否适合在 Skyline 中使用 TDesign？

### ✅ 适合的场景

- 以 **表单 + 列表展示** 为主的 B 端/C 端应用
- 使用了上述已适配组件，且不依赖 Cascader、Calendar 等复杂交互组件
- 愿意接受渐进式迁移（先 WebView 后逐页切 Skyline）
- 目标是利用 Skyline 的性能优势（长列表、worklet 动画、更低内存占用）

### ⚠️ 需要谨慎的场景

- 重度使用 **Cascader（级联选择器）、Calendar（日历）、DateTimePicker（日期选择器）** 的项目
- 对 **Toast loading 动画、Collapse 折叠动画** 有较高体验要求的项目
- 需要在 iOS 低版本微信上运行（官方建议 Android + 微信 8.0.49 体验最佳）
- 项目已经深度定制了 TDesign 组件样式和动画

### 💡 推荐策略：渐进式迁移

微信官方和 TDesign 都推荐**渐进式迁移**策略：

1. **项目全局保留 WebView 为主**，不对 app.json 设置全局 `"renderer": "skyline"`
2. **逐页迁移**：在 page.json 中单独为页面开启 Skyline
3. **优先迁移关键路径页面**，让大多数用户获得性能收益
4. 低版本微信/PC 端会自动 **fallback 到 WebView**，无需担心兼容性

```json
// page.json — 单个页面开启 Skyline
{
  "renderer": "skyline",
  "navigationStyle": "custom",
  "disableScroll": true,
  "componentFramework": "glass-easel"
}
```

---

## 七、Skyline 开启的完整配置参考

```json
// app.json
{
  "rendererOptions": {
    "skyline": {
      "defaultDisplayBlock": true,
      "defaultContentBox": true,
      "tagNameStyleIsolation": "legacy",
      "enableScrollViewAutoSize": true
    }
  },
  "componentFramework": "glass-easel",
  "lazyCodeLoading": "requiredComponents"
}
```

配合全局样式 Reset（兼容 WebView）：

```css
page, view, text, image, button, video, map,
scroll-view, swiper, input, textarea, navigator {
  position: relative;
  background-origin: border-box;
  isolation: isolate;
}
page {
  height: 100%;
}
```

---

## 八、关键资源

| 资源 | 链接 |
|------|------|
| TDesign 小程序官网 | https://tdesign.tencent.com/miniprogram/overview |
| GitHub 仓库 | https://github.com/Tencent/tdesign-miniprogram |
| Skyline 适配追踪 Issue | https://github.com/Tencent/tdesign-miniprogram/issues/3149 |
| 首批适配 PR #2659 | https://github.com/Tencent/tdesign-miniprogram/pull/2659 |
| 微信 Skyline 迁移文档 | https://developers.weixin.qq.com/miniprogram/dev/framework/runtime/skyline/migration/ |
| Skyline 兼容问题官方文档 | https://developers.weixin.qq.com/miniprogram/dev/framework/runtime/skyline/migration/compatibility.html |
| Skyline 更新日志 | https://developers.weixin.qq.com/miniprogram/dev/framework/runtime/skyline/changelog.html |
| wzkris-mini（Skyline + TDesign 实践） | https://gitee.com/wzkris/wzkris-mini |

---

## 九、总结

| 维度 | 评价 |
|------|------|
| **基础组件可用性** | ⭐⭐⭐⭐ 良好 — Button、Input、Form、Toast、Popup 等核心组件可用 |
| **高级组件覆盖** | ⭐⭐ 不足 — Cascader、Calendar 等关键组件仍未适配 |
| **维护活跃度** | ⭐⭐⭐⭐⭐ 积极 — 每版 release 都有 Skyline 相关修复，v1.16.0 刚发布 |
| **社区生态** | ⭐⭐⭐ 一般 — 有项目实践案例，但公开分享的完整方案不多 |
| **官方文档** | ⭐⭐ 不足 — 官网未公开标注各组件的 Skyline 兼容状态（建议查阅 GitHub issue） |
| **总体可用性** | ⭐⭐⭐ 谨慎可用 — 需要评估项目具体用到的组件，不可盲目全面切换 |

**一句话建议**：如果你能接受目前只使用已适配组件（基础类 + 大部分导航/数据展示/反馈类），避开 Cascader/Calendar，**现在就可以用 TDesign + Skyline 开始开发**；如果需要全量组件，**建议等官方完成更多适配（关注 #3149），先用 WebView 开发，逐步迁移。**
