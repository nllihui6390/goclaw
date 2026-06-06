<script setup>
import { ref, inject, onMounted, computed, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRouter } from 'vue-router'
import { useAgentStore } from '@/stores/agent'

const api = inject('api')
const router = useRouter()
const agentStore = useAgentStore()

const loading = ref(false)
const sessions = ref([])

const filteredSessions = computed(() => {
  return sessions.value.filter(s => s.agent === agentStore.selectedAgent)
})

async function loadSessions() {
  loading.value = true
  try {
    sessions.value = await api.getSessions() || []
  } catch (e) {
    ElMessage.error('加载失败: ' + e.message)
  }
  loading.value = false
}

function viewSession(session) {
  router.push({ path: '/', query: { session: session.id, agent: session.agent } })
}

async function deleteSession(session) {
  try {
    await ElMessageBox.confirm(`确定删除会话 "${session.name || session.id}"？`, '确认删除', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消'
    })
    await api.deleteSession(session.id)
    ElMessage.success('删除成功')
    await loadSessions()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error('删除失败: ' + e.message)
  }
}

function formatTime(ts) {
  if (!ts) return '-'
  return new Date(ts).toLocaleString('zh-CN')
}

function formatTimeShort(ts) {
  if (!ts) return '-'
  const d = new Date(ts)
  return `${d.getMonth() + 1}/${d.getDate()} ${d.getHours()}:${String(d.getMinutes()).padStart(2, '0')}`
}

function getChannelIcon(channel) {
  const icons = {
    console: 'Monitor',
    wechat: 'ChatDotRound',
    dingtalk: 'ChatLineSquare',
    lark: 'ChatDotSquare',
    wecom: 'ChatRound',
    telegram: 'Promotion',
    slack: 'Connection'
  }
  return icons[channel] || 'ChatLineSquare'
}

function getChannelType(channel) {
  const types = {
    console: 'success',
    wechat: 'info',
    dingtalk: 'primary',
    lark: 'success',
    wecom: 'warning',
    telegram: 'info',
    slack: 'primary'
  }
  return types[channel] || 'info'
}

function getChannelLabel(channel) {
  const labels = {
    console: 'Console',
    wechat: 'WeChat',
    dingtalk: 'DingTalk',
    lark: 'Lark',
    wecom: 'WeCom',
    telegram: 'Telegram',
    slack: 'Slack'
  }
  return labels[channel] || channel
}

onMounted(loadSessions)
watch(() => agentStore.selectedAgent, loadSessions)
</script>

<template>
  <div class="page" v-loading="loading">
    <!-- Page header -->
    <div class="page-header">
      <div class="header-title">
        <h2>会话管理</h2>
        <span class="session-count">{{ filteredSessions.length }} 条记录</span>
      </div>
      <div class="header-actions">
        <el-button type="primary" @click="loadSessions">
          <el-icon><Refresh /></el-icon>刷新
        </el-button>
      </div>
    </div>

    <!-- Sessions cards grid -->
    <div class="sessions-grid" v-if="filteredSessions.length">
      <div v-for="session in filteredSessions" :key="session.id" class="session-card">
        <!-- Card top: channel icon + session name -->
        <div class="card-top">
          <div class="channel-icon-wrap">
            <el-icon :size="18"><component :is="getChannelIcon(session.channel)" /></el-icon>
          </div>
          <div class="session-info">
            <span class="session-name">{{ session.name || session.id.slice(0, 8) }}</span>
            <span class="session-id">{{ session.id }}</span>
          </div>
        </div>

        <!-- Card body: agent + channel + user -->
        <div class="card-body">
          <div class="session-meta">
            <div class="meta-item">
              <span class="meta-label">Agent</span>
              <el-tag size="small" effect="plain">{{ session.agent }}</el-tag>
            </div>
            <div class="meta-item">
              <span class="meta-label">渠道</span>
              <el-tag size="small" :type="getChannelType(session.channel)">
                {{ getChannelLabel(session.channel) }}
              </el-tag>
            </div>
            <div class="meta-item" v-if="session.user_id">
              <span class="meta-label">用户</span>
              <span class="meta-value">{{ session.user_id }}</span>
            </div>
          </div>
        </div>

        <!-- Card footer: time + actions -->
        <div class="card-footer">
          <div class="time-info">
            <div class="time-item">
              <span class="time-label">创建</span>
              <span class="time-value">{{ formatTimeShort(session.created_at) }}</span>
            </div>
            <div class="time-item">
              <span class="time-label">更新</span>
              <span class="time-value">{{ formatTimeShort(session.updated_at) }}</span>
            </div>
          </div>
          <div class="card-actions">
            <el-button size="small" type="primary" @click="viewSession(session)">查看</el-button>
            <el-button size="small" type="danger" @click="deleteSession(session)">删除</el-button>
          </div>
        </div>
      </div>
    </div>

    <el-empty v-if="!filteredSessions.length && !loading" description="暂无会话记录" />
  </div>
</template>

<style lang="scss" scoped>
@use '@/styles/variables.scss' as *;

.page {
  padding: 32px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 28px;
}

.header-title {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-title h2 {
  margin: 0;
  font-size: $font-size-xl;
  font-weight: 600;
  color: $text-primary;
}

.session-count {
  font-size: $font-size-sm;
  color: $text-muted;
  font-family: $font-display;
  padding: 4px 10px;
  background: $bg-elevated;
  border-radius: $radius-sm;
  border: 1px solid $border-default;
}

.header-actions {
  display: flex;
  gap: 8px;
}

// ──── Sessions cards grid ────
.sessions-grid {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.session-card {
  @include glass-panel;
  border-radius: $radius-lg;
  padding: 14px 20px;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 16px;

  @media (max-width: 768px) {
    flex-direction: column;
    align-items: stretch;
  }

  &:hover {
    border-color: $accent-cyan-dim;
    box-shadow: $shadow-glow-cyan;
    transform: translateY(-2px);
  }
}

// ──── Card top ────
.card-top {
  display: flex;
  align-items: center;
  gap: 14px;
}

.channel-icon-wrap {
  width: 40px;
  height: 40px;
  border-radius: $radius-md;
  background: $accent-cyan-dim;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  border: 1px solid rgba(0, 212, 255, 0.2);

  .el-icon {
    color: $accent-cyan;
  }
}

.session-info {
  flex: 1;
  min-width: 0;
}

.session-name {
  font-size: $font-size-base;
  font-weight: 600;
  color: $text-primary;
  display: block;
  margin-bottom: 4px;
}

.session-id {
  font-size: $font-size-xs;
  color: $text-muted;
  font-family: $font-display;
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

// ──── Card body ────
.card-body {
  flex: 1;
  display: flex;
  align-items: center;
}

.session-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  align-items: center;
}

.meta-item {
  display: flex;
  align-items: center;
  gap: 6px;
}

.meta-label {
  font-size: $font-size-xs;
  color: $text-muted;
}

.meta-value {
  font-size: $font-size-sm;
  color: $text-secondary;
  font-family: $font-display;
}

// ──── Card footer ────
.card-footer {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-left: auto;

  @media (max-width: 768px) {
    margin-left: 0;
    justify-content: flex-end;
  }
}

.time-info {
  display: flex;
  gap: 16px;
  flex-shrink: 0;
}

.time-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.time-label {
  font-size: $font-size-xs;
  color: $text-muted;
}

.time-value {
  font-size: $font-size-sm;
  color: $text-secondary;
  font-family: $font-display;
}

.card-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

// ──── Mobile ────
@media (max-width: 768px) {
  .page { padding: 16px; }
  .card-actions { flex-shrink: 0; }
}
</style>