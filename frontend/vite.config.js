import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'path'

export default defineConfig(({ command, mode }) => {
    // 检测是否为 Wails 构建
  const isWails = process.env.WAILS === 'true' || mode === 'wails'
  return {
    base: isWails ? './' : '/', // 设置 base 路径为根目录
    plugins: [vue()],
    resolve: {
      alias: {
        '@': path.resolve(__dirname, 'src')
      }
    },
    build: {
      outDir: 'dist',
      assetsDir: 'assets',
      rollupOptions: {
        output: {
        manualChunks: undefined,
        },
        modulePreload: false,
      },
    },
    server: {
      proxy: {
        '/api': 'http://localhost:8080',
        '/ws': { target: 'ws://localhost:8080', ws: true }
      }
    },
    // Wails 3 需要的配置
    define: {
      'process.env.NODE_ENV': JSON.stringify(mode),
    },
  }
})
