<script setup>
import { ref, inject, onMounted, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useAgentStore } from '@/stores/agent'
import { useToolsStore } from '@/stores/tools'

const api = inject('api')
const agentStore = useAgentStore()
const toolsStore = useToolsStore()

const loading = ref(false)
const config = ref({})

// 当前选中的 Agent 名称
const agentName = computed(() => agentStore.selectedAgent || 'default')

// 所有工具名
const allTools = computed(() => toolsStore.allToolNames)

// 获取工具元信息
function toolMeta(name) {
  return toolsStore.getTool(name)
}

// 当前选中的 agent 配置
const currentAgent = computed(() => {
  return config.value.agents?.find(a => a.name === agentName.value)
})

// 工具是否对当前 agent 启用
function isToolEnabled(toolName) {
  return currentAgent.value?.tools?.includes(toolName) || false
}

// 切换工具开关（自动保存）
async function toggleTool(toolName, enabled) {
  const agent = currentAgent.value
  if (!agent) return
  if (!agent.tools) agent.tools = []
  if (enabled) {
    if (!agent.tools.includes(toolName)) agent.tools.push(toolName)
  } else {
    agent.tools = agent.tools.filter(t => t !== toolName)
  }
  try {
    await api.saveConfig(config.value)
  } catch (e) {
    ElMessage.error('保存失败: ' + e.message)
  }
}

// 加载配置
async function loadConfig() {
  loading.value = true
  try {
    config.value = await api.getConfig() || {}
  } catch (e) {
    ElMessage.error('加载配置失败: ' + e.message)
  }
  loading.value = false
}

onMounted(loadConfig)
// agent 切换时重新加载
watch(agentName, loadConfig)
</script>

<template>
  <div class="page" v-loading="loading">
    <div class="toolbar">
      <span class="toolbar-title">
        工具管理 — <span class="current-agent">{{ agentName }}</span>
      </span>
    </div>

    <div class="tools-grid">
      <el-card v-for="toolName in allTools" :key="toolName" class="tool-card">
        <div class="tool-header">
          <div class="tool-icon">{{ toolMeta(toolName).icon || '🔧' }}</div>
          <div class="tool-info">
            <span class="tool-name">{{ toolName }}</span>
            <span class="tool-desc">{{ toolMeta(toolName).desc || '' }}</span>
          </div>
          <el-switch
            :model-value="isToolEnabled(toolName)"
            @change="val => toggleTool(toolName, val)"
            size="small"
          />
        </div>
      </el-card>

      <el-empty v-if="!allTools.length && !loading" description="暂无工具" />
    </div>
  </div>
</template>

<style scoped>
.page { padding: 24px; }
.toolbar { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
.toolbar-title { font-size: 18px; font-weight: 500; }
.current-agent { color: #409eff; font-weight: 600; }

.tools-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
}

.tool-card { transition: all .2s; }
.tool-card:hover { box-shadow: 0 2px 12px rgba(0,0,0,.1); }

.tool-header { display: flex; align-items: center; gap: 12px; }
.tool-icon { font-size: 24px; }
.tool-info { flex: 1; min-width: 0; }
.tool-name { font-size: 15px; font-weight: 600; }
.tool-desc { font-size: 12px; color: #909399; display: block; margin-top: 2px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
</style>