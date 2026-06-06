import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import App from './App.vue'
import router from './router'
import './styles/global.scss'

async function init() {
  let api

  const isWails = !!(
    window._wails?.environment ||
    window.chrome?.webview?.postMessage ||
    window.webkit?.messageHandlers?.external ||
    window.wails?.invoke
  )

  if (isWails) {
    console.log('[go-claw] 桌面模式 (Wails3)')
    const { WailsAdapter } = await import('./api/adapters/WailsAdapter.js')
    api = new WailsAdapter()
  } else {
    console.log('[go-claw] Web 模式 (HTTP)')
    const { default: HttpAdapter } = await import('./api/adapters/HttpAdapter.js')
    api = HttpAdapter
  }

  const app = createApp(App)
  app.use(createPinia())
  app.use(ElementPlus)
  app.use(router)
  app.provide('api', api)

  for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
    app.component(key, component)
  }
  app.mount('#app')
}

init()