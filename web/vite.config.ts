import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      // Forward API calls (including the websocket upgrade) to the Go server.
      '/api': {
        target: 'http://localhost:8080',
        ws: true,
      },
    },
  },
})
