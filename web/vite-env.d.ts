/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Browser tab title; inlined at build time (see src/brand). */
  readonly VITE_BRAND_TITLE?: string
  /** ICP filing number shown on the login page footer. */
  readonly VITE_ICP_NUMBER?: string
  /** External docs site origin. */
  readonly VITE_DOCS_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
