// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// In development the API is proxied to the Go server on :8080.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
      '/calendar': 'http://localhost:8080',
      '/healthz': 'http://localhost:8080',
    },
  },
  build: { outDir: 'dist', emptyOutDir: true, sourcemap: false },
})
