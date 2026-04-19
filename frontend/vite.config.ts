import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

export default defineConfig({
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: [],
    alias: [{ find: '@', replacement: path.resolve(__dirname, './src') }],
  },
  plugins: [
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    // maplibre-gl (~1 MB) and echarts (~600 kB) are irreducible third-party
    // libraries that ship as monolithic bundles. They are lazy-loaded via
    // route-based code splitting, so their size only affects pages that use
    // maps or charts. Gzip sizes are reasonable (277 kB / 202 kB).
    chunkSizeWarningLimit: 1100,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules/maplibre-gl')) {
            return 'vendor-maplibre'
          }
          if (id.includes('node_modules/echarts')) {
            return 'vendor-echarts'
          }
          if (id.includes('node_modules/zrender')) {
            return 'vendor-echarts'
          }
          if (id.includes('node_modules/react-pdf') || id.includes('node_modules/pdfjs-dist')) {
            return 'vendor-react-pdf'
          }
          if (
            id.includes('node_modules/@tanstack/react-query') ||
            id.includes('node_modules/react-router-dom') ||
            id.includes('node_modules/react-router') ||
            id.includes('node_modules/date-fns')
          ) {
            return 'vendor-core'
          }
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/tiles': {
        target: 'http://localhost:3000',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/tiles/, ''),
        configure: (proxy) => {
          proxy.on('proxyRes', (proxyRes) => {
            // Prevent ALL caching of tiles — no-store prevents disk/memory cache
            proxyRes.headers['cache-control'] = 'no-store, must-revalidate'
            // Remove ETag so browser can't use conditional requests for stale data
            delete proxyRes.headers['etag']
          })
        },
      },
    },
  },
})
