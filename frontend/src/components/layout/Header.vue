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
      <el-select v-model="agentModel" placeholder="选择 Agent" size="small" style="width: 140px">
        <el-option v-for="a in agentOptions" :key="a.value" :label="a.label" :value="a.value" />
      </el-select>
    </div>
  </header>
</template>

<style scoped>
.header { height: 48px; background: #fff; border-bottom: 1px solid #e4e7ed; display: flex; align-items: center; justify-content: space-between; padding: 0 20px; flex-shrink: 0; }
.header-left { display: flex; align-items: center; gap: 10px; }
.header-title { font-size: 16px; font-weight: 500; color: #303133; margin: 0; }
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