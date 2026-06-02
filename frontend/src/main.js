import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import App from './App.vue'
import router from './router'
import './styles/global.scss'

async function init() {
  let api

  // 桌面模式：Wails3 bridge 直接调用 Go 函数
  if (window.go?.main?.ChatService) {
    console.log('[go-claw] 桌面模式 (Wails3)')
    const { WailsAdapter } = await import('./api/adapters/WailsAdapter.js')
    api = new WailsAdapter()
  } else {
    // Web 模式：HTTP API
    console.log('[go-claw] Web 模式 (HTTP)')
    const { default: HttpAdapter } = await import('./api/adapters/HttpAdapter.js')
    api = HttpAdapter
  }

  const app = createApp(App)
  app.use(ElementPlus)
  app.use(router)
  app.provide('api', api)
  for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
    app.component(key, component)
  }
  app.mount('#app')
}

init()
