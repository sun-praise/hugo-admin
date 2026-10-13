import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: '/admin-ui/',
  build: {
    outDir: '../admin-ui',
    emptyOutDir: true,
    // mermaid (loaded lazily, only when a post contains a ```mermaid block)
    // ships an internal ~660 kB shared chunk that cannot be split further
    // from the app side. Every app-owned chunk stays under the default 500 kB.
    chunkSizeWarningLimit: 700,
  },
  server: {
    // pnpm dev 时把 /api（含 SSE）代理到本地 Go 服务,无需先构建再由 Go 托管
    proxy: {
      '/api': 'http://localhost:5050',
    },
  },
})
