import { fileURLToPath, URL } from 'node:url'
import vue from '@vitejs/plugin-vue'
import { defineConfig, loadEnv } from 'vite'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const target = env.VITE_DEV_PROXY_TARGET || 'http://127.0.0.1:7863'

  return {
    plugins: [vue()],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    build: {
      outDir: '../internal/web/dist',
      emptyOutDir: false,
      sourcemap: false,
    },
    server: {
      host: '0.0.0.0',
      port: Number(env.VITE_DEV_PORT || 4173),
      proxy: {
        '/api': { target, changeOrigin: true },
        '/v1': { target, changeOrigin: true },
        '/status': { target, changeOrigin: true },
        '/healthz': { target, changeOrigin: true },
      },
    },
    test: {
      environment: 'jsdom',
      globals: true,
    },
  }
})
