import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'

export const useThemeStore = defineStore('theme', () => {
  // 亮度模式：'system' | 'dark' | 'light'
  const saved = localStorage.getItem('goclaw_theme')
  const theme = ref(saved || 'system')

  // 风格皮肤：'default' | 'ink' | 'aurora' | 'warm'
  const savedSkin = localStorage.getItem('goclaw_skin')
  const skin = ref(savedSkin || 'default')

  // 系统主题状态
  const systemIsDark = ref(false)

  const skins = [
    { value: 'default', label: '默认', icon: 'Cpu' },
    { value: 'ink', label: '墨韵', icon: 'EditPen' },
    { value: 'aurora', label: '极光', icon: 'MagicStick' },
    { value: 'warm', label: '暖调', icon: 'Coffee' },
  ]

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
    applySkin()
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

  // 应用皮肤到 DOM（在 html 上切换 theme-xxx class）
  function applySkin() {
    const html = document.documentElement
    // 移除所有皮肤 class
    html.classList.remove('theme-ink', 'theme-aurora', 'theme-warm')
    // 添加当前皮肤 class（default 不需要额外 class）
    if (skin.value !== 'default') {
      html.classList.add('theme-' + skin.value)
    }
  }

  // 设置亮度模式
  function setTheme(newTheme) {
    theme.value = newTheme
    localStorage.setItem('goclaw_theme', newTheme)
    applyTheme()
  }

  // 设置风格皮肤
  function setSkin(newSkin) {
    skin.value = newSkin
    localStorage.setItem('goclaw_skin', newSkin)
    applySkin()
  }

  // 监听变化自动应用
  watch(actualTheme, applyTheme)

  return {
    theme,
    skin,
    skins,
    actualTheme,
    systemIsDark,
    init,
    setTheme,
    setSkin
  }
})