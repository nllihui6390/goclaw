<script setup>
import { useRoute } from 'vue-router'
import { computed, inject, onMounted } from 'vue'
import { useAgentStore } from '@/stores/agent'
import { useThemeStore } from '@/stores/theme'

const route = useRoute()
const api = inject('api')
const agentStore = useAgentStore()
const themeStore = useThemeStore()
const isMobile = inject('isMobile')
const toggleMobile = inject('toggleMobile')
const toggleCollapse = inject('toggleCollapse')

async function loadAgentList() {
  try {
    const list = await api.getAgents() || []
    agentStore.setAgentList(list)
  } catch { /* degrade */ }
}

onMounted(loadAgentList)

const agentNameMap = computed(() => {
  const map = {}
  agentStore.agentList.forEach(a => { map[a.name] = a.display_name || a.name })
  return map
})

const agentOptions = computed(() => {
  const names = Object.keys(agentNameMap.value)
  if (names.length === 0) return [{ label: agentStore.selectedAgent, value: agentStore.selectedAgent }]
  return names.map(name => ({ label: agentNameMap.value[name], value: name }))
})

const pageTitle = computed(() => {
  const pathMap = {
    '/': '聊天', '/inbox': '收件箱', '/channels': '渠道管理',
    '/sessions': '会话管理', '/cron-jobs': '定时任务',
    '/agent-config': 'Agent 配置', '/workspace': '工作空间',
    '/skills': '技能管理', '/tools': '工具列表', '/files': '文件管理', '/mcp': 'MCP 集成',
    '/models': '模型供应商', '/security': '安全设置', '/debug': '调试日志'
  }
  return pathMap[route.path] || 'go-claw'
})

const agentModel = computed({
  get: () => agentStore.selectedAgent,
  set: (val) => { agentStore.setAgent(val) }
})

const themeIcon = computed(() => {
  if (themeStore.actualTheme === 'dark') return 'Moon'
  return 'Sunny'
})

function setTheme(mode) {
  themeStore.setTheme(mode)
}
</script>

<template>
  <header class="header">
    <div class="header-left">
      <button v-if="isMobile" class="menu-btn" @click="toggleMobile()">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 12h18M3 6h18M3 18h18"/></svg>
      </button>
      <button v-else class="menu-btn" @click="toggleCollapse()">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 12h18M3 6h18M3 18h18"/></svg>
      </button>
      <h1 class="page-title">{{ pageTitle }}</h1>
    </div>

    <div class="header-right">
      <!-- Agent selector -->
      <div class="agent-selector">
        <span class="selector-label">Agent</span>
        <el-select v-model="agentModel" placeholder="Agent" size="small" class="agent-select">
          <el-option v-for="a in agentOptions" :key="a.value" :label="a.label" :value="a.value" />
        </el-select>
      </div>

      <!-- Theme switch -->
      <el-dropdown trigger="click" @command="setTheme">
        <button class="icon-btn" title="切换主题">
          <el-icon :size="18"><component :is="themeIcon" /></el-icon>
        </button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="system" :class="{ checked: themeStore.theme === 'system' }">
              <el-icon :size="14"><Monitor /></el-icon>
              <span>跟随系统</span>
            </el-dropdown-item>
            <el-dropdown-item command="dark" :class="{ checked: themeStore.theme === 'dark' }">
              <el-icon :size="14"><Moon /></el-icon>
              <span>深色</span>
            </el-dropdown-item>
            <el-dropdown-item command="light" :class="{ checked: themeStore.theme === 'light' }">
              <el-icon :size="14"><Sunny /></el-icon>
              <span>亮色</span>
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>

      <!-- GitHub link -->
      <a href="https://github.com/nllihui6390/goclaw" target="_blank" rel="noopener noreferrer" class="icon-btn" title="GitHub">
        <svg width="18" height="18" viewBox="0 0 16 16" fill="currentColor"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"/></svg>
      </a>
    </div>
  </header>
</template>

<style lang="scss" scoped>
@use '@/styles/variables.scss' as *;

.header {
  height: $header-height;
  background: $bg-surface;
  border-bottom: 1px solid $border-subtle;
  display: flex; align-items: center; justify-content: space-between;
  padding: 0 24px; flex-shrink: 0; z-index: 10;
  transition: background 0.3s, border-color 0.3s;
}

.header-left { display: flex; align-items: center; gap: 16px; }

.menu-btn {
  width: 36px; height: 36px; border: none; border-radius: $radius-md;
  background: transparent; cursor: pointer; display: flex; align-items: center; justify-content: center;
  color: $text-secondary; transition: all 0.2s;
  &:hover { background: $bg-glass-light; color: $text-primary; }
}

.page-title { font-size: $font-size-lg; font-weight: 600; color: $text-primary; margin: 0; }

.header-right { display: flex; align-items: center; gap: 8px; }

.agent-selector { display: flex; align-items: center; gap: 8px; }
.selector-label { font-size: $font-size-sm; color: $text-secondary; }
.agent-select { width: 130px;
  :deep(.el-select__wrapper) {
    background: $bg-elevated; border: 1px solid $border-default; border-radius: $radius-md;
    box-shadow: none; min-height: 32px;
    &:hover { border-color: $accent-cyan; }
    &.is-focused { border-color: $accent-cyan; }
  }
}

.icon-btn {
  width: 36px; height: 36px; border: none; border-radius: $radius-md;
  background: transparent; cursor: pointer; display: flex; align-items: center; justify-content: center;
  color: $text-secondary; transition: all 0.2s; text-decoration: none;
  &:hover { background: $bg-glass-light; color: $text-primary; }
}

.checked {
  color: $accent-cyan;
}
</style>