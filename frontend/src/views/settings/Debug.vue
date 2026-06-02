<script setup>
import { ref, inject, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'

const api = inject('api')
const logs = ref('')
const status = ref({})
const loading = ref(false)
const autoRefresh = ref(false)
let refreshTimer = null

onMounted(async () => {
  await loadLogs()
  await loadStatus()
})

onUnmounted(() => {
  if (refreshTimer) {
    clearInterval(refreshTimer)
  }
})

async function loadLogs() {
  loading.value = true
  try {
    const data = await api.getLogs()
    logs.value = data || '暂无日志'
  } catch (e) {
    logs.value = '加载失败: ' + e.message
  }
  loading.value = false
}

async function loadStatus() {
  try {
    status.value = await api.getStatus()
  } catch (e) {
    console.log('加载状态失败:', e.message)
  }
}

function toggleAutoRefresh() {
  if (autoRefresh.value) {
    refreshTimer = setInterval(loadLogs, 3000)
    ElMessage.success('已开启自动刷新')
  } else {
    if (refreshTimer) {
      clearInterval(refreshTimer)
      refreshTimer = null
    }
    ElMessage.info('已关闭自动刷新')
  }
}

function formatLog(log) {
  // 简单的颜色标记：ERROR 红色，WARN 黄色，INFO 绿色
  return log
    .replace(/ERROR/g, '<span style="color:#f56c6c">ERROR</span>')
    .replace(/WARN/g, '<span style="color:#e6a23c">WARN</span>')
    .replace(/INFO/g, '<span style="color:#67c23a">INFO</span>')
}
</script>

<template>
  <div class="page">
    <el-card class="status-card">
      <template #header>
        <span>系统状态</span>
      </template>
      <el-descriptions :column="2" border size="small">
        <el-descriptions-item label="运行状态">
          <el-tag type="success">{{ status.status || 'running' }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="启动时间">{{ status.uptime || '-' }}</el-descriptions-item>
      </el-descriptions>
    </el-card>

    <el-card class="logs-card">
      <template #header>
        <div class="logs-header">
          <span>运行日志</span>
          <div class="logs-actions">
            <el-switch v-model="autoRefresh" @change="toggleAutoRefresh" active-text="自动刷新" />
            <el-button size="small" @click="loadLogs" :loading="loading">刷新</el-button>
          </div>
        </div>
      </template>
      <div class="logs-container" v-loading="loading">
        <pre class="logs-content" v-html="formatLog(logs)"></pre>
      </div>
    </el-card>
  </div>
</template>

<style scoped>
.page { padding: 24px; display: flex; flex-direction: column; height: calc(100vh - 48px); gap: 16px; box-sizing: border-box; }
.status-card { flex-shrink: 0; }
.logs-card { flex: 1; display: flex; flex-direction: column; min-height: 0; }
.logs-card :deep(.el-card__body) { flex: 1; display: flex; flex-direction: column; min-height: 0; padding: 16px; }
.logs-header { display: flex; justify-content: space-between; align-items: center; flex-shrink: 0; }
.logs-actions { display: flex; gap: 12px; align-items: center; }
.logs-container {
  flex: 1;
  min-height: 0;
  overflow: auto;
  background: #1a1b26;
  border-radius: 4px;
}
.logs-content {
  margin: 0;
  padding: 12px;
  font-family: 'Consolas', 'Monaco', monospace;
  font-size: 12px;
  line-height: 1.5;
  color: #a9b1d6;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>