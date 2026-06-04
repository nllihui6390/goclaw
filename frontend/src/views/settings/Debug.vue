<script setup>
import { ref, inject, onMounted, onUnmounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useDebugStore } from '@/stores/debug'

const api = inject('api')
const debugStore = useDebugStore()
const logs = ref('')
const status = ref({})
const loading = ref(false)
const restarting = ref(false)
let refreshTimer = null

const autoRefresh = computed({
  get: () => debugStore.autoRefresh,
  set: (val) => { debugStore.autoRefresh = val }
})

onMounted(async () => {
  await loadLogs()
  await loadStatus()
  if (autoRefresh.value) startRefresh()
})

onUnmounted(() => stopRefresh())

async function loadLogs() {
  loading.value = true
  try {
    const data = await api.getLogs()
    logs.value = data || '暂无日志'
  } catch (e) {
    logs.value = '加载失败: ' + e.message
  }
  loading.value = false
  setTimeout(scrollLogBottom, 100)
}

function scrollLogBottom() {
  const el = document.querySelector('.logs-container')
  if (el) el.scrollTop = el.scrollHeight
}

async function loadStatus() {
  try {
    status.value = await api.getStatus()
  } catch (e) {
    console.log('加载状态失败:', e.message)
  }
}

function startRefresh() {
  stopRefresh()
  refreshTimer = setInterval(loadLogs, 3000)
}
function stopRefresh() {
  if (refreshTimer) {
    clearInterval(refreshTimer)
    refreshTimer = null
  }
}

function toggleAutoRefresh() {
  if (autoRefresh.value) {
    startRefresh()
    ElMessage.success('已开启自动刷新')
  } else {
    stopRefresh()
    ElMessage.info('已关闭自动刷新')
  }
}

async function handleRestart() {
  try {
    await ElMessageBox.confirm('确定要重启系统吗？将重新加载配置并同步所有 Agent 和 Channel。', '重启确认', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })
    restarting.value = true
    const result = await api.restart()
    restarting.value = false
    if (result.error) {
      ElMessage.error('重启失败: ' + result.error)
    } else {
      ElMessage.success('重启成功')
      await loadStatus()
      await loadLogs()
    }
  } catch {
    // 用户取消
    restarting.value = false
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
        <div class="status-header">
          <span>系统状态</span>
          <el-button type="warning" size="small" @click="handleRestart" :loading="restarting">
            重启
          </el-button>
        </div>
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
.status-header { display: flex; justify-content: space-between; align-items: center; }
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