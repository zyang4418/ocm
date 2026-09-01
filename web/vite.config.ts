import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [
    react(),
    {
      // Brand title placeholder in index.html: Vite's built-in HTML env
      // replacement only substitutes %VITE_*% when the variable is actually
      // set — otherwise the literal placeholder would ship to production.
      // Default it to "OCM" here (unless the build passed a real value), so
      // dev and unset-env builds render a sane title. See src/brand.
      name: 'brand-title-default',
      transformIndexHtml: {
        order: 'pre',
        handler(html: string) {
          const title = process.env.VITE_BRAND_TITLE || 'OCM'
          return html.replaceAll('%VITE_BRAND_TITLE%', title)
        },
      },
    },
  ],
  server: {
    proxy: {
      '/api': {
        target: process.env.VITE_API_PROXY_TARGET || 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
