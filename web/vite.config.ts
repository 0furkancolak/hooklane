import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// The dev server proxies API calls so the dashboard and the API share an
// origin. Override the target when the API is not on port 8080.
const apiTarget = process.env.HOOKLANE_API_URL ?? 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/healthz': apiTarget,
      '/readyz': apiTarget,
      '/v1': apiTarget,
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    restoreMocks: true,
  },
})
