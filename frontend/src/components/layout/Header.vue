<script setup>
import { useRoute } from 'vue-router'
import { computed, inject } from 'vue'

const route = useRoute()
const selectedAgent = inject('selectedAgent')

const agentOptions = [
  { label: 'default', value: 'default' },
  { label: 'local', value: 'local' },
]

const pageTitle = computed(() => {
  const pathMap = {
    '/': '聊天', '/inbox': '收件箱', '/channels': '渠道管理',
    '/sessions': '会话管理', '/cron-jobs': '定时任务',
    '/agent-config': 'Agent 配置', '/workspace': '工作空间',
    '/skills': '技能管理', '/tools': '工具列表', '/mcp': 'MCP 集成',
    '/models': '模型供应商', '/security': '安全设置', '/debug': '调试日志'
  }
  return pathMap[route.path] || 'go-claw'
})

// computed with getter/setter for v-model
const agentModel = computed({
  get: () => selectedAgent.value,
  set: (val) => { selectedAgent.value = val }
})
</script>

<template>
  <header class="header">
    <div class="header-left">
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
.header-title { font-size: 16px; font-weight: 500; color: #303133; margin: 0; }
</style>