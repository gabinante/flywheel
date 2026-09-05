import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// https://vite.dev/config/
const apiTarget = process.env.VITE_API_PROXY ?? 'http://127.0.0.1:8090'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    proxy: {
      '^/(orgs|projects|tickets|agents|mcp|api|invites|worker-config|\\.well-known|healthz|readyz|metrics|me)(/|$)': {
        target: apiTarget,
        changeOrigin: false,
      },
    },
  },
})
