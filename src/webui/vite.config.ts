/**
 * @file vite.config.ts
 * @description Vite 构建配置：启用 Vue 插件并把 `md-` 前缀标签视为自定义元素（适配 @material/web），
 *   构建产物输出到 src/module/webroot/netproxy，避免覆盖统一入口与第三方面板。
 */
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

// https://vite.dev/config/
export default defineConfig({
  base: './',
  plugins: [
    vue({
      template: {
        compilerOptions: {
          // 把所有以 'md-' 开头的标签视为自定义元素（供 @material/web 使用）
          isCustomElement: (tag) => tag.startsWith('md-')
        }
      }
    })
  ],
  build: {
    // 仅清理 NetProxy WebUI 子目录，保留根入口、zashboard 与 Service Dashboard
    outDir: resolve(__dirname, '../module/webroot/netproxy'),
    emptyOutDir: true
  }
})
