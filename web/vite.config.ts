/// <reference types="vitest/config" />
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'
import { storyImages } from './build/storyImages.ts'

// https://vite.dev/config/
export default defineConfig({
  // storyImages: the still images of /lab/historia, rendered at build time.
  plugins: [react(), storyImages()],
  // three.js is one lazy chunk of ~530 kB (~130 kB gzip), loaded only by the
  // 3D scenes; scripts/check-bundle.mjs keeps it out of the initial load.
  build: { chunkSizeWarningLimit: 600 },
  server: {
    // In development the Go API runs separately; in production Caddy routes /api.
    proxy: {
      '/api': 'http://127.0.0.1:8080',
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    restoreMocks: true,
    unstubGlobals: true,
  },
})
