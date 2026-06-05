<script setup>
import { useRoute } from 'vue-router'
import { computed, inject, onMounted, watch } from 'vue'
import { useAgentStore } from '@/stores/agent'

const route = useRoute()
const api = inject('api')
const agentStore = useAgentStore()
const isMobile = inject('isMobile')
const toggleMobile = inject('toggleMobile')
const toggleCollapse = inject('toggleCollapse')

// 加载 agent 列表到共享 store
async function loadAgentList() {
  try {
    const list = await api.getAgents() || []
    agentStore.setAgentList(list)
  } catch { /* 降级 */ }
}

onMounted(loadAgentList)

// 构建 agent ID → display_name 映射
const agentNameMap = computed(() => {
  const map = {}
  agentStore.agentList.forEach(a => {
    map[a.name] = a.display_name || a.name
  })
  return map
})

const agentOptions = computed(() => {
  const names = Object.keys(agentNameMap.value)
  if (names.length === 0) {
    // 降级：从 store 当前值生成选项
    return [{ label: agentStore.selectedAgent, value: agentStore.selectedAgent }]
  }
  return names.map(name => ({
    label: agentNameMap.value[name],
    value: name
  }))
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
</script>

<template>
  <header class="header">
    <div class="header-left">
      <!-- 手机菜单按钮 -->
      <button v-if="isMobile" class="menu-btn" @click="toggleMobile()">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M3 12h18M3 6h18M3 18h18"/>
        </svg>
      </button>
      <!-- 桌面折叠按钮 -->
      <button v-else class="menu-btn" @click="toggleCollapse()">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M3 12h18M3 6h18M3 18h18"/>
        </svg>
      </button>
      <h1 class="header-title">{{ pageTitle }}</h1>
    </div>
    <div class="header-right">
      <label class="label-agent">当前Agent</label>
      <el-select v-model="agentModel" placeholder="选择 Agent" size="small" style="width: 140px">
        <el-option v-for="a in agentOptions" :key="a.value" :label="a.label" :value="a.value" />
      </el-select>
      <a href="https://github.com/nllihui6390/goclaw" target="_blank" rel="noopener noreferrer" class="github-link" title="GitHub 开源项目">
        <svg width="20" height="20" viewBox="0 0 16 16" fill="currentColor">
          <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"/>
        </svg>
      </a>
    </div>
  </header>
</template>

<style scoped>
.header { height: 48px; background: #fff; border-bottom: 1px solid #e4e7ed; display: flex; align-items: center; justify-content: space-between; padding: 0 20px; flex-shrink: 0; }
.header-left { display: flex; align-items: center; gap: 10px; }
.header-title { font-size: 16px; font-weight: 500; color: #303133; margin: 0; }
.header-right { display: flex; align-items: center; gap: 12px; }
.label-agent { font-size: 12px; }
.github-link {
  display: flex; align-items: center; justify-content: center;
  color: #606266;
  transition: color .15s;
  &:hover { color: #303133; }
}
.menu-btn {
  width: 32px; height: 32px;
  border: none; border-radius: 6px;
  background: transparent;
  cursor: pointer;
  display: flex; align-items: center; justify-content: center;
  color: #606266;
  transition: all .15s;
  &:hover { background: #f0f2f5; }
}
</style>