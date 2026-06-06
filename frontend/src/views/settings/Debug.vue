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

function startRefresh() { stopRefresh(); refreshTimer = setInterval(loadLogs, 3000) }
function stopRefresh() { if (refreshTimer) { clearInterval(refreshTimer); refreshTimer = null } }

function toggleAutoRefresh() {
  if (autoRefresh.value) { startRefresh(); ElMessage.success('已开启自动刷新') }
  else { stopRefresh(); ElMessage.info('已关闭自动刷新') }
}

async function handleRestart() {
  try {
    await ElMessageBox.confirm('确定要重启系统吗？', '重启确认', {
      confirmButtonText: '确定', cancelButtonText: '取消', type: 'warning'
    })
    restarting.value = true
    const result = await api.restart()
    restarting.value = false
    if (result.error) { ElMessage.error('重启失败: ' + result.error) }
    else { ElMessage.success('重启成功'); await loadStatus(); await loadLogs() }
  } catch { restarting.value = false }
}

function highlightLog(text) {
  if (!text) return ''
  // Escape HTML entities first
  const esc = text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')

  // Highlight rules: [regex, cssClass, group]
  const rules = [
    [/(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}(?:\.\d+)?)/g, 'hl-time'],
    [/"([^"]+)"/g, 'hl-string'],
    [/\b(ERROR|ERRO|FATAL|PANIC)\b/g, 'hl-error'],
    [/\b(WARN|WARNING)\b/g, 'hl-warn'],
    [/\b(INFO)\b/g, 'hl-info'],
    [/\b(DEBUG|TRACE)\b/g, 'hl-debug'],
    [/\b(\d+(?:\.\d+)?\s*(?:ms|MB|KB|GB|bps|s|%))\b/g, 'hl-num'],
    [/\b(\w[\w-]*)=/g, 'hl-key'],
    [/(https?:\/\/[^\s"]+)/g, 'hl-path'],
  ]

  return esc.split('\n').map(line => {
    // Collect all matches with positions (from original line text)
    const matches = []
    for (const [regex, cls] of rules) {
      let m
      const re = new RegExp(regex.source, regex.flags) // clone
      while ((m = re.exec(line)) !== null) {
        matches.push({ start: m.index, end: m.index + m[0].length, cls, text: m[0] })
      }
    }
    // Sort by start, longer matches first for ties
    matches.sort((a, b) => a.start - b.start || b.end - a.end)

    // Build highlighted line, skipping overlaps
    let out = ''
    let pos = 0
    for (const m of matches) {
      if (m.start < pos) continue // overlaps with previous match
      out += line.slice(pos, m.start)
      out += `<span class="${m.cls}">${m.text}</span>`
      pos = m.end
    }
    out += line.slice(pos)
    return out
  }).join('\n')
}
</script>

<template>
  <div class="page">
    <!-- Status -->
    <div class="status-section">
      <div class="status-header">
        <div class="header-title">
          <span class="title-symbol">◉</span>
          <span>系统状态</span>
        </div>
        <el-button type="warning" size="small" @click="handleRestart" :loading="restarting">重启</el-button>
      </div>
      <div class="status-grid">
        <div class="status-item">
          <span class="status-label">运行状态</span>
          <div class="status-value">
            <span class="status-dot running"></span>
            <span>{{ status.status || 'running' }}</span>
          </div>
        </div>
        <div class="status-item">
          <span class="status-label">启动时间</span>
          <span class="status-value mono">{{ status.uptime || '—' }}</span>
        </div>
      </div>
    </div>

    <!-- Logs -->
    <div class="logs-section">
      <div class="logs-header">
        <div class="header-title">
          <span class="title-symbol">⊡</span>
          <span>运行日志</span>
        </div>
        <div class="logs-actions">
          <el-switch v-model="autoRefresh" @change="toggleAutoRefresh" active-text="自动刷新" />
          <el-button size="small" @click="loadLogs" :loading="loading">刷新</el-button>
        </div>
      </div>
      <div class="logs-container" v-loading="loading">
        <pre class="logs-content" v-html="highlightLog(logs)"></pre>
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
@use '@/styles/variables.scss' as *;

.page {
  padding: 32px; display: flex; flex-direction: column;
  height: calc(100vh - $header-height); gap: 16px; box-sizing: border-box;
}

.status-section { @include glass-panel; border-radius: $radius-lg; padding: 20px; flex-shrink: 0; }
.status-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.header-title { display: flex; align-items: center; gap: 8px; font-size: $font-size-lg; font-weight: 600; color: $text-primary; }
.status-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.status-item { display: flex; flex-direction: column; gap: 6px; padding: 12px 16px; background: $bg-elevated; border-radius: $radius-md; border: 1px solid $border-default; }
.status-label { font-size: $font-size-xs; color: $text-muted; font-family: $font-display; text-transform: uppercase; letter-spacing: 0.5px; }
.status-value { font-size: $font-size-base; color: $text-primary; display: flex; align-items: center; gap: 8px; }
.mono { font-family: $font-display; }
.status-dot { width: 8px; height: 8px; border-radius: 50%; &.running { background: $accent-emerald; animation: pulse-glow 2s ease-in-out infinite; } }

.logs-section { @include glass-panel; border-radius: $radius-lg; flex: 1; display: flex; flex-direction: column; min-height: 0; overflow: hidden; }
.logs-header { display: flex; justify-content: space-between; align-items: center; padding: 20px; flex-shrink: 0; border-bottom: 1px solid $border-subtle; }
.logs-actions { display: flex; gap: 12px; align-items: center; }
.logs-container { flex: 1; min-height: 0; overflow: auto; padding: 16px; }

.logs-content {
  margin: 0; font-family: $font-display; font-size: $font-size-sm; line-height: 1.7;
  color: $text-secondary; white-space: pre-wrap; word-break: break-all;

  :deep(.hl-error)  { color: #f43f5e; font-weight: 600; }
  :deep(.hl-warn)   { color: #f59e0b; font-weight: 600; }
  :deep(.hl-info)   { color: #10b981; font-weight: 600; }
  :deep(.hl-debug)  { color: #8b5cf6; font-weight: 600; }
  :deep(.hl-time)   { color: #6b7280; }
  :deep(.hl-num)    { color: #0891b2; }
  :deep(.hl-string) { color: #d97706; }
  :deep(.hl-path)   { color: #3b82f6; }
  :deep(.hl-key)    { color: #a855f7; }
}

// Dark mode - toned down colors
:global([data-theme="dark"]) {
  .logs-content {
    :deep(.hl-error)  { color: #fb7185; }
    :deep(.hl-warn)   { color: #fbbf24; }
    :deep(.hl-info)   { color: #34d399; }
    :deep(.hl-debug)  { color: #a78bfa; }
    :deep(.hl-num)    { color: #22d3ee; }
    :deep(.hl-string) { color: #fbbf24; }
    :deep(.hl-path)   { color: #60a5fa; }
    :deep(.hl-key)    { color: #c084fc; }
  }
}
</style>