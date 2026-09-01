// Brand is the single injection point for deployment branding on the web.
//
// Rule: with injection, the injected config wins; without injection, the OCM
// defaults apply. Two injection mechanisms coexist:
//
//   1. Injection file (file DI): create `src/brand/brand.override.ts` next to
//      this module, e.g.
//
//        import type { BrandConfig } from './index'
//        export const brand: BrandConfig = {
//          name: '示例大学',
//          i18n: { 'zh-CN': { common: { app: { title: '示例大学智慧教室管理平台' } } } },
//        }
//
//      Vite resolves the glob below at build time — when the file is absent
//      the glob is empty and the override tree-shakes away; when present the
//      values merge over the defaults.
//
//   2. Build-time env: `VITE_BRAND_TITLE` inlines into index.html's <title>
//      (see vite.config.ts) and serves as the tab-title fallback here.

/** Deep i18n overrides: language -> namespace -> patch object. */
export type BrandI18n = Partial<Record<string, Partial<Record<string, Record<string, unknown>>>>>

export interface BrandConfig {
  /** Brand word: login page brand + tab-title fallback. */
  name?: string
  /** Login page cloud-style title fragment; defaults to `name`. */
  titleCloud?: string
  /** Browser tab title; defaults to `name`. */
  tabTitle?: string
  /** Per-language i18n overrides, deep-merged over the bundled resources. */
  i18n?: BrandI18n
}

const defaults: Required<Pick<BrandConfig, 'name' | 'titleCloud'>> = {
  name: 'OCM',
  titleCloud: 'OCM',
}

const modules = import.meta.glob<{ brand?: BrandConfig }>('./brand.override.ts', {
  eager: true,
})
const injected: BrandConfig = Object.values(modules)[0]?.brand ?? {}

/** Effective brand: injected values win over the OCM defaults. */
export const brand = {
  name: injected.name ?? defaults.name,
  titleCloud: injected.titleCloud ?? injected.name ?? defaults.titleCloud,
  i18n: injected.i18n ?? {},
}

/** Browser tab title. Priority: tabTitle > name > VITE_BRAND_TITLE env > OCM. */
export const brandTitle: string =
  injected.tabTitle ?? injected.name ?? import.meta.env.VITE_BRAND_TITLE ?? defaults.name
