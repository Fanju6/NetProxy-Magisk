import { fileURLToPath, URL } from 'node:url'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vite'

export default defineConfig(({ mode }) => ({
  plugins: [
    react(),
    tailwindcss(),
    ...(mode === 'native'
      ? [
          {
            name: 'native-ipc-csp',
            transformIndexHtml: (html: string) =>
              html.replace("connect-src 'self'", "connect-src 'self' ipc: http://ipc.localhost"),
          },
        ]
      : []),
    {
      name: 'local-dev-csp',
      apply: 'serve',
      transformIndexHtml: (html) =>
        html.replace("connect-src 'self'", "connect-src 'self' ws://127.0.0.1:*"),
    },
  ],
  base: './',
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  server: { host: '127.0.0.1' },
  build: { outDir: mode === 'native' ? 'dist-native' : 'dist', sourcemap: false },
}))
