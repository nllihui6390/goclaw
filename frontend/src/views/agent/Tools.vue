<script setup>
import { ref, inject, onMounted, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useAgentStore } from '@/stores/agent'
import { useToolsStore } from '@/stores/tools'

const api = inject('api')
const agentStore = useAgentStore()
const toolsStore = useToolsStore()

const loading = ref(false)
const agents = ref([])
const agentConfig = ref(null)

const agentName = computed(() => agentStore.selectedAgent || 'default')
const allTools = computed(() => toolsStore.allToolNames)

function toolMeta(name) {
  return toolsStore.getTool(name)
}

const currentAgent = computed(() => {
  return agents.value.find(a => a.name === agentName.value) || null
})

function isToolEnabled(toolName) {
  return currentAgent.value?.tools?.includes(toolName) || false
}

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
    await api.updateAgent(agentName.value, agent)
  } catch (e) {
    ElMessage.error('保存失败: ' + e.message)
  }
}

async function selectAll() {
  const agent = currentAgent.value
  if (!agent) return
  agent.tools = [...allTools.value]
  try {
    await api.updateAgent(agentName.value, agent)
    ElMessage.success('已启用全部工具')
  } catch (e) {
    ElMessage.error('保存失败: ' + e.message)
  }
}

async function selectNone() {
  const agent = currentAgent.value
  if (!agent) return
  agent.tools = []
  try {
    await api.updateAgent(agentName.value, agent)
    ElMessage.success('已禁用全部工具')
  } catch (e) {
    ElMessage.error('保存失败: ' + e.message)
  }
}

async function loadConfig() {
  loading.value = true
  try {
    const list = await api.getAgents() || []
    agents.value = list
  } catch (e) {
    ElMessage.error('加载配置失败: ' + e.message)
  }
  loading.value = false
}

onMounted(loadConfig)
watch(agentName, loadConfig)
</script>

<template>
  <div class="page" v-loading="loading">
    <div class="page-header">
      <div class="header-left">
        <h2>工具管理</h2>
        <div class="header-info">
          <el-tag size="small">{{ agentName }}</el-tag>
          <span class="tool-count">{{ allTools.length }} 个工具</span>
        </div>
      </div>
      <div class="header-actions">
        <el-button size="small" @click="selectAll">全选</el-button>
        <el-button size="small" @click="selectNone">全不选</el-button>
      </div>
    </div>

    <div class="tools-grid">
      <div
        v-for="toolName in allTools"
        :key="toolName"
        class="tool-card"
        :class="{ enabled: isToolEnabled(toolName) }"
      >
        <div class="tool-status-badge" v-if="isToolEnabled(toolName)">
          <span class="status-dot"></span>已启用
        </div>
        <div class="tool-header">
          <div class="tool-icon">{{ toolMeta(toolName).icon || '🔧' }}</div>
          <div class="tool-info">
            <span class="tool-name">{{ toolName }}</span>
            <span class="tool-desc">{{ toolMeta(toolName).desc || '—' }}</span>
          </div>
        </div>
        <el-switch
          class="tool-switch"
          :model-value="isToolEnabled(toolName)"
          @change="val => toggleTool(toolName, val)"
          size="small"
        />
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
@use '@/styles/variables.scss' as *;

.page { padding: 32px; }

.page-header {
  display: flex; justify-content: space-between; align-items: center;
  margin-bottom: 28px;
}

.header-left { display: flex; align-items: center; gap: 12px; }

.header-left h2 { margin: 0; font-size: $font-size-xl; font-weight: 600; color: $text-primary; }
.header-info { display: flex; align-items: center; gap: 8px; }
.tool-count { font-size: $font-size-sm; color: $text-muted; font-family: $font-display; }

.header-actions { display: flex; gap: 6px; }

.tools-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 12px; }

.tool-card {
  @include glass-panel; border-radius: $radius-md; padding: 16px;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  position: relative;
  @include stagger-entrance(12, 0.04s);

  &:hover {
    border-color: $border-default;
    transform: translateY(-2px);
    box-shadow: $shadow-soft;
  }

  &.enabled {
    border-color: rgba(103, 194, 58, 0.35);
    background: $accent-emerald-dim;

    &:hover {
      box-shadow: $shadow-glow-emerald;
    }
  }
}

.tool-status-badge {
  position: absolute;
  top: 10px;
  right: 10px;
  @include status-badge($accent-emerald, $accent-emerald-dim);
}

.tool-header {
  display: flex;
  align-items: center;
  gap: 12px;
}

.tool-icon {
  font-size: 24px; width: 40px; height: 40px;
  display: flex; align-items: center; justify-content: center;
  background: $bg-elevated; border-radius: $radius-md;
  border: 1px solid $border-default; flex-shrink: 0;
  .enabled & {
    background: $accent-emerald-dim;
    border-color: rgba(103, 194, 58, 0.2);
  }
}

.tool-info { flex: 1; min-width: 0; }
.tool-name { font-size: $font-size-sm; font-weight: 600; color: $text-primary; font-family: $font-display; display: block; margin-bottom: 4px; }
.tool-desc { font-size: $font-size-xs; color: $text-muted; display: block; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

.tool-switch {
  position: absolute;
  bottom: 12px;
  right: 12px;
}
</style>