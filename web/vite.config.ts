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
  // Fonts are never inlined as data: (the Greek faces are under the 4 KB default):
  // inlined, they would sit in the render-blocking CSS of every page; as files the
  // browser fetches them only when a page uses their unicode-range.
  build: { chunkSizeWarningLimit: 600, assetsInlineLimit: (file) => (file.endsWith('.woff2') ? false : undefined) },
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
