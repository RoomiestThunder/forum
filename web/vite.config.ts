import { writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vitest/config'
import type { Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// A build empties dist/, including the tracked dist/.gitkeep that lets
// `go build` (go:embed) work on a fresh checkout before the frontend has been
// built. Put it back so building doesn't leave the working tree dirty.
const keepDistPlaceholder = (): Plugin => ({
  name: 'keep-dist-placeholder',
  apply: 'build',
  closeBundle() {
    writeFileSync(fileURLToPath(new URL('./dist/.gitkeep', import.meta.url)), '')
  },
})

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss(), keepDistPlaceholder()],
  server: {
    proxy: {
      // Forward API calls (including the websocket upgrade) to the Go server.
      '/api': {
        target: 'http://localhost:8080',
        ws: true,
      },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
  },
})
