import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    // In development the Go API runs separately; in production Caddy routes /api.
    proxy: {
      '/api': 'http://127.0.0.1:8080',
    },
  },
})
