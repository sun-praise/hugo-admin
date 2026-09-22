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
})
