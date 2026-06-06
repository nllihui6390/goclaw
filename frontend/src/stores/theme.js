import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'

export const useThemeStore = defineStore('theme', () => {
  // 三种模式：'system' | 'dark' | 'light'
  const saved = localStorage.getItem('goclaw_theme')
  const theme = ref(saved || 'system')

  // 系统主题状态
  const systemIsDark = ref(false)

  // 初始化：检测系统主题
  function init() {
    const media = window.matchMedia('(prefers-color-scheme: dark)')
    systemIsDark.value = media.matches

    // 监听系统主题变化
    media.addEventListener('change', (e) => {
      systemIsDark.value = e.matches
      applyTheme()
    })

    applyTheme()
  }

  // 计算实际应用的主题
  const actualTheme = computed(() => {
    if (theme.value === 'system') {
      return systemIsDark.value ? 'dark' : 'light'
    }
    return theme.value
  })

  // 应用主题到 DOM
  function applyTheme() {
    const html = document.documentElement

    if (actualTheme.value === 'dark') {
      html.classList.add('dark')
      html.setAttribute('data-theme', 'dark')
    } else {
      html.classList.remove('dark')
      html.setAttribute('data-theme', 'light')
    }
  }

  // 设置主题
  function setTheme(newTheme) {
    theme.value = newTheme
    localStorage.setItem('goclaw_theme', newTheme)
    applyTheme()
  }

  // 监听变化自动应用
  watch(actualTheme, applyTheme)

  return {
    theme,
    actualTheme,
    systemIsDark,
    init,
    setTheme
  }
})