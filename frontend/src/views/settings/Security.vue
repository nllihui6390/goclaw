<script setup>
import { ref, computed, onMounted, onUnmounted, inject } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Check, Close, Lock, Unlock, Monitor, WarningFilled, Connection } from '@element-plus/icons-vue'

const api = inject('api')

const loading = ref(false)
const approvals = ref([])
const pollingTimer = ref(null)
const activeTab = ref('approvals')

// 安全配置
const securityConfig = ref({
  enabled: false,
  deny_shell_inject: false,
  deny_sensitive_path: false,
  guard_browser: false,
  allowed_paths: []
})

const guardEnabled = computed(() => securityConfig.value.enabled)
const pendingCount = computed(() => approvals.value.length)

// 加载待审批列表
const loadApprovals = async () => {
  try {
    const data = await api.getPendingApprovals()
    approvals.value = data || []
  } catch (error) {
    console.error('加载审批列表失败:', error)
  }
}

// 加载安全配置
const loadSecurityConfig = async () => {
  try {
    const data = await api.getSecurityConfig()
    securityConfig.value = data || {}
  } catch (error) {
    console.error('加载安全配置失败:', error)
  }
}

// 保存安全配置
const saveSecurityConfig = async () => {
  try {
    loading.value = true
    await api.updateSecurityConfig(securityConfig.value)
    ElMessage.success('配置已保存')
  } catch (error) {
    ElMessage.error('保存失败: ' + (error.message || error))
  } finally {
    loading.value = false
  }
}

// 批准请求
const handleApprove = async (id) => {
  try {
    await ElMessageBox.confirm('确定要批准此操作吗？', '确认', {
      confirmButtonText: '批准',
      cancelButtonText: '取消',
      type: 'success'
    })
    loading.value = true
    await api.approveRequest(id)
    ElMessage.success('已批准')
    await loadApprovals()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('批准失败: ' + (error.message || error))
    }
  } finally {
    loading.value = false
  }
}

// 拒绝请求
const handleDeny = async (id) => {
  try {
    const { value: reason } = await ElMessageBox.prompt('请输入拒绝原因（可选）', '拒绝操作', {
      confirmButtonText: '拒绝',
      cancelButtonText: '取消',
      inputPlaceholder: '拒绝原因',
      type: 'warning'
    })
    loading.value = true
    await api.denyRequest(id, reason || '')
    ElMessage.success('已拒绝')
    await loadApprovals()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('拒绝失败: ' + (error.message || error))
    }
  } finally {
    loading.value = false
  }
}

// 格式化时间
const formatTime = (timestamp) => {
  if (!timestamp) return '-'
  const date = new Date(timestamp * 1000)
  return date.toLocaleString('zh-CN')
}

// 相对时间
const relativeTime = (timestamp) => {
  if (!timestamp) return ''
  const diff = Math.floor(Date.now() / 1000) - timestamp
  if (diff < 60) return `${diff}秒前`
  if (diff < 3600) return `${Math.floor(diff / 60)}分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`
  return formatTime(timestamp)
}

// 截断审批 ID
const shortId = (id) => {
  if (!id) return ''
  return id.length > 20 ? id.slice(0, 8) + '...' + id.slice(-6) : id
}

// 开始轮询
const startPolling = () => {
  loadApprovals()
  pollingTimer.value = setInterval(loadApprovals, 3000)
}

const stopPolling = () => {
  if (pollingTimer.value) {
    clearInterval(pollingTimer.value)
    pollingTimer.value = null
  }
}

onMounted(() => {
  startPolling()
  loadSecurityConfig()
})

onUnmounted(() => {
  stopPolling()
})
</script>

<template>
  <div class="page">
    <!-- Status Banner -->
    <div class="status-banner" :class="{ active: guardEnabled }">
      <div class="status-banner-left">
        <div class="status-icon">
          <el-icon :size="18"><Lock v-if="guardEnabled" /><Unlock v-else /></el-icon>
        </div>
        <div class="status-text">
          <span class="status-label">{{ guardEnabled ? '安全守卫已启用' : '安全守卫未启用' }}</span>
          <span class="status-detail">
            {{ guardEnabled ? 'Agent 执行敏感操作前将触发审批流程' : '所有工具调用将直接执行，无安全拦截' }}
          </span>
        </div>
      </div>
      <div class="status-banner-right">
        <div v-if="guardEnabled" class="guard-badges">
          <span class="guard-badge" :class="{ on: securityConfig.deny_shell_inject }">
            <span class="badge-dot"></span>Shell
          </span>
          <span class="guard-badge" :class="{ on: securityConfig.deny_sensitive_path }">
            <span class="badge-dot"></span>文件
          </span>
          <span class="guard-badge" :class="{ on: securityConfig.guard_browser }">
            <span class="badge-dot"></span>浏览器
          </span>
        </div>
        <el-switch
          v-model="securityConfig.enabled"
          inline-prompt
          active-text="ON"
          inactive-text="OFF"
          @change="saveSecurityConfig"
        />
      </div>
    </div>

    <!-- Tab Navigation -->
    <div class="tab-bar">
      <button
        class="tab-btn"
        :class="{ active: activeTab === 'approvals' }"
        @click="activeTab = 'approvals'"
      >
        <el-icon :size="14"><Monitor /></el-icon>
        待审批
        <span v-if="pendingCount > 0" class="tab-badge">{{ pendingCount }}</span>
      </button>
      <button
        class="tab-btn"
        :class="{ active: activeTab === 'config' }"
        @click="activeTab = 'config'"
      >
        <el-icon :size="14"><WarningFilled /></el-icon>
        安全配置
      </button>
    </div>

    <!-- Approvals Tab -->
    <div v-show="activeTab === 'approvals'" class="tab-panel">
      <div class="panel-header">
        <div class="panel-title">
          <span class="title-symbol">▸</span>
          <h3>待审批请求</h3>
        </div>
        <el-button :icon="Refresh" text size="small" @click="loadApprovals" :loading="loading">
          刷新
        </el-button>
      </div>

      <div v-if="approvals.length === 0" class="empty-state">
        <div class="empty-icon">
          <el-icon :size="40"><WarningFilled /></el-icon>
        </div>
        <p class="empty-title">暂无待审批请求</p>
        <p class="empty-hint">当 Agent 触发安全守卫时，审批请求将在此处显示</p>
      </div>

      <div v-else class="approvals-list">
        <div
          v-for="(approval, idx) in approvals"
          :key="approval.id"
          class="approval-card"
          :style="{ animationDelay: `${idx * 0.06}s` }"
        >
          <div class="approval-card-accent"></div>
          <div class="approval-card-body">
            <div class="approval-top">
              <div class="approval-tool">
                <span class="tool-icon">⚡</span>
                <span class="tool-name">{{ approval.tool_name }}</span>
              </div>
              <span class="approval-time" :title="formatTime(approval.created_at)">
                {{ relativeTime(approval.created_at) }}
              </span>
            </div>

            <div class="approval-reason">
              {{ approval.message || approval.reason }}
            </div>

            <div v-if="approval.params && Object.keys(approval.params).length > 0" class="approval-params">
              <div class="params-label">参数</div>
              <pre class="params-code">{{ JSON.stringify(approval.params, null, 2) }}</pre>
            </div>

            <div class="approval-id-row">
              <span class="approval-id" :title="approval.id">{{ shortId(approval.id) }}</span>
            </div>

            <div class="approval-actions">
              <button class="action-btn approve" @click="handleApprove(approval.id)" :disabled="loading">
                <el-icon :size="14"><Check /></el-icon>
                批准
              </button>
              <button class="action-btn deny" @click="handleDeny(approval.id)" :disabled="loading">
                <el-icon :size="14"><Close /></el-icon>
                拒绝
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Config Tab -->
    <div v-show="activeTab === 'config'" class="tab-panel">
      <div class="panel-header">
        <div class="panel-title">
          <span class="title-symbol">▸</span>
          <h3>工具安全守卫</h3>
        </div>
      </div>

      <p class="config-desc">启用后，Agent 执行敏感操作前会触发审批流程，需用户手动确认方可继续。</p>

      <div class="guard-cards">
        <!-- Shell 命令注入检测 -->
        <div class="guard-card" :class="{ disabled: !guardEnabled }">
          <div class="guard-card-header">
            <div class="guard-card-icon shell">
              <span class="guard-icon-text">$</span>
            </div>
            <div class="guard-card-info">
              <h4>Shell 命令注入检测</h4>
              <span class="guard-tag">ShellEvasionGuardian</span>
            </div>
            <el-switch
              v-model="securityConfig.deny_shell_inject"
              :disabled="!guardEnabled"
              size="small"
            />
          </div>
          <p class="guard-desc">检测危险命令模式并触发审批</p>
          <div class="guard-patterns">
            <code>$(cmd)</code>
            <code>`cmd`</code>
            <code>| cmd</code>
            <code>; cmd</code>
            <code>&& cmd</code>
            <code>rm -rf /</code>
            <code>dd if=</code>
            <code>mkfs</code>
            <code>chmod 777</code>
            <code>shutdown</code>
          </div>
        </div>

        <!-- 敏感文件路径保护 -->
        <div class="guard-card" :class="{ disabled: !guardEnabled }">
          <div class="guard-card-header">
            <div class="guard-card-icon file">
              <span class="guard-icon-text">🔒</span>
            </div>
            <div class="guard-card-info">
              <h4>敏感文件路径保护</h4>
              <span class="guard-tag">FileGuardian</span>
            </div>
            <el-switch
              v-model="securityConfig.deny_sensitive_path"
              :disabled="!guardEnabled"
              size="small"
            />
          </div>
          <p class="guard-desc">禁止访问敏感路径（直接拒绝，不触发审批）</p>
          <div class="guard-patterns">
            <code>/etc/passwd</code>
            <code>/etc/shadow</code>
            <code>~/.ssh/id_rsa</code>
            <code>.env</code>
            <code>credentials</code>
            <code>secrets</code>
          </div>
        </div>

        <!-- 浏览器操作守卫 -->
        <div class="guard-card" :class="{ disabled: !guardEnabled }">
          <div class="guard-card-header">
            <div class="guard-card-icon browser">
              <el-icon :size="16"><Connection /></el-icon>
            </div>
            <div class="guard-card-info">
              <h4>浏览器操作守卫</h4>
              <span class="guard-tag">RuleGuardian</span>
            </div>
            <el-switch
              v-model="securityConfig.guard_browser"
              :disabled="!guardEnabled"
              size="small"
            />
          </div>
          <p class="guard-desc">所有 <code>browser_use</code> 工具调用需要用户确认</p>
        </div>
      </div>

      <div class="save-row">
        <el-button type="primary" @click="saveSecurityConfig" :loading="loading">
          保存配置
        </el-button>
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
@use '@/styles/variables.scss' as *;

.page {
  padding: 32px;
}

// ─────────── Status Banner ───────────
.status-banner {
  @include glass-panel;
  border-radius: $radius-lg;
  padding: 16px 20px;
  margin-bottom: 24px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  transition: all 0.3s ease;

  &.active {
    border-color: $border-subtle;
  }
}

.status-banner-left {
  display: flex;
  align-items: center;
  gap: 14px;
}

.status-icon {
  width: 40px;
  height: 40px;
  border-radius: $radius-md;
  display: flex;
  align-items: center;
  justify-content: center;
  background: $accent-rose-dim;
  border: 1px solid rgba($accent-rose, 0.2);
  color: $accent-rose;
  transition: all 0.3s ease;

  .active & {
    background: $accent-emerald-dim;
    border-color: rgba($accent-emerald, 0.3);
    color: $accent-emerald;
  }
}

.status-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.status-label {
  font-size: $font-size-lg;
  font-weight: 600;
  color: $text-primary;
}

.status-detail {
  font-size: $font-size-sm;
  color: $text-muted;
}

.status-banner-right {
  display: flex;
  align-items: center;
  gap: 16px;
}

.guard-badges {
  display: flex;
  gap: 8px;
}

.guard-badge {
  display: flex;
  align-items: center;
  gap: 5px;
  font-size: $font-size-xs;
  font-family: $font-display;
  color: $text-muted;
  padding: 3px 10px;
  border-radius: $radius-sm;
  background: $bg-glass-light;
  border: 1px solid $border-subtle;
  transition: all 0.2s ease;

  &.on {
    color: $accent-emerald;
    background: $accent-emerald-dim;
    border-color: rgba($accent-emerald, 0.3);
  }

  .badge-dot {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: $text-muted;
    transition: all 0.2s ease;

    .on & {
      background: $accent-emerald;
      animation: pulse-dot 2s ease-in-out infinite;
    }
  }
}

// ─────────── Tab Bar ───────────
.tab-bar {
  display: flex;
  gap: 4px;
  margin-bottom: 20px;
  padding: 4px;
  background: $bg-elevated;
  border-radius: $radius-md;
  border: 1px solid $border-subtle;
  width: fit-content;
}

.tab-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 18px;
  border: none;
  border-radius: $radius-sm;
  background: transparent;
  color: $text-muted;
  font-size: $font-size-sm;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
  font-family: $font-ui;

  &:hover {
    color: $text-secondary;
    background: $bg-glass-light;
  }

  &.active {
    background: $bg-surface;
    color: $text-primary;
    box-shadow: $shadow-soft;
    border: 1px solid $border-subtle;
  }
}

.tab-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: 9px;
  background: $accent-rose;
  color: #fff;
  font-size: 11px;
  font-weight: 700;
  font-family: $font-display;
  animation: pulse-glow 2s ease-in-out infinite;
}

// ─────────── Tab Panel ───────────
.tab-panel {
  animation: fade-up 0.3s ease-out;
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.panel-title {
  display: flex;
  align-items: center;
  gap: 8px;

  .title-symbol {
    color: $accent-cyan;
    font-size: 14px;
  }

  h3 {
    margin: 0;
    font-size: $font-size-lg;
    font-weight: 600;
    color: $text-primary;
  }
}

// ─────────── Empty State ───────────
.empty-state {
  @include glass-panel;
  border-radius: $radius-lg;
  padding: 60px 40px;
  text-align: center;
}

.empty-icon {
  color: $text-muted;
  opacity: 0.4;
  margin-bottom: 16px;
}

.empty-title {
  margin: 0 0 6px;
  font-size: $font-size-lg;
  color: $text-secondary;
  font-weight: 500;
}

.empty-hint {
  margin: 0;
  font-size: $font-size-sm;
  color: $text-muted;
}

// ─────────── Approval Cards ───────────
.approvals-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.approval-card {
  @include glass-panel;
  border-radius: $radius-lg;
  display: flex;
  overflow: hidden;
  animation: fade-up 0.4s ease-out both;
  transition: all 0.25s ease;

  &:hover {
    border-color: $border-glow;
    box-shadow: $shadow-glow-cyan;
  }
}

.approval-card-accent {
  width: 3px;
  flex-shrink: 0;
  background: linear-gradient(180deg, $accent-amber, $accent-rose);
}

.approval-card-body {
  flex: 1;
  padding: 16px 20px;
}

.approval-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}

.approval-tool {
  display: flex;
  align-items: center;
  gap: 8px;
}

.tool-icon {
  font-size: 14px;
}

.tool-name {
  font-size: $font-size-lg;
  font-weight: 600;
  color: $text-primary;
  font-family: $font-display;
}

.approval-time {
  font-size: $font-size-xs;
  color: $text-muted;
  font-family: $font-display;
}

.approval-reason {
  font-size: $font-size-sm;
  color: $text-secondary;
  line-height: 1.6;
  margin-bottom: 12px;
}

.approval-params {
  margin-bottom: 12px;
  padding: 10px 14px;
  background: $bg-elevated;
  border-radius: $radius-sm;
  border: 1px solid $border-subtle;
}

.params-label {
  font-size: $font-size-xs;
  color: $text-muted;
  margin-bottom: 6px;
  font-weight: 500;
}

.params-code {
  margin: 0;
  font-size: $font-size-xs;
  color: $text-code;
  font-family: $font-display;
  line-height: 1.5;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-all;
}

.approval-id-row {
  margin-bottom: 14px;
}

.approval-id {
  font-size: $font-size-xs;
  color: $text-muted;
  font-family: $font-display;
  opacity: 0.7;
}

.approval-actions {
  display: flex;
  gap: 10px;
}

.action-btn {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 7px 18px;
  border: 1px solid $border-default;
  border-radius: $radius-sm;
  font-size: $font-size-sm;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
  font-family: $font-ui;
  background: transparent;
  color: $text-secondary;

  &:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  &.approve {
    color: $accent-emerald;
    border-color: rgba($accent-emerald, 0.3);
    background: $accent-emerald-dim;

    &:hover:not(:disabled) {
      background: rgba($accent-emerald, 0.2);
      box-shadow: $shadow-glow-emerald;
    }
  }

  &.deny {
    color: $accent-rose;
    border-color: rgba($accent-rose, 0.3);
    background: $accent-rose-dim;

    &:hover:not(:disabled) {
      background: rgba($accent-rose, 0.2);
      box-shadow: 0 0 15px rgba($accent-rose, 0.2);
    }
  }
}

// ─────────── Config Tab ───────────
.config-desc {
  font-size: $font-size-sm;
  color: $text-muted;
  margin: 0 0 20px;
  line-height: 1.5;
}

.guard-cards {
  display: flex;
  flex-direction: column;
  gap: 14px;
  margin-bottom: 24px;
}

.guard-card {
  @include glass-panel;
  border-radius: $radius-lg;
  padding: 20px;
  transition: all 0.3s ease;
  animation: fade-up 0.4s ease-out both;

  @for $i from 1 through 3 {
    &:nth-child(#{$i}) {
      animation-delay: #{$i * 0.06}s;
    }
  }

  &:hover:not(.disabled) {
    border-color: $border-glow;
  }

  &.disabled {
    opacity: 0.5;
    pointer-events: none;
  }
}

.guard-card-header {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 10px;
}

.guard-card-icon {
  width: 38px;
  height: 38px;
  border-radius: $radius-md;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  border: 1px solid $border-subtle;

  &.shell {
    background: $accent-cyan-dim;
    border-color: rgba($accent-cyan, 0.2);
    color: $accent-cyan;
  }

  &.file {
    background: $accent-amber-dim;
    border-color: rgba($accent-amber, 0.2);
  }

  &.browser {
    background: rgba(#a78bfa, 0.1);
    border-color: rgba(#a78bfa, 0.2);
    color: #a78bfa;
  }
}

.guard-icon-text {
  font-size: 16px;
  font-weight: 700;
  font-family: $font-display;
}

.guard-card-info {
  flex: 1;
  min-width: 0;

  h4 {
    margin: 0 0 3px;
    font-size: $font-size-base;
    font-weight: 600;
    color: $text-primary;
  }
}

.guard-tag {
  font-size: $font-size-xs;
  font-family: $font-display;
  color: $text-muted;
  opacity: 0.7;
}

.guard-desc {
  font-size: $font-size-sm;
  color: $text-secondary;
  margin: 0 0 12px;
  line-height: 1.5;

  code {
    font-family: $font-display;
    font-size: $font-size-xs;
    background: $accent-cyan-dim;
    color: $accent-cyan;
    padding: 1px 6px;
    border-radius: 3px;
  }
}

.guard-patterns {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;

  code {
    font-family: $font-display;
    font-size: $font-size-xs;
    background: $bg-elevated;
    color: $text-secondary;
    padding: 3px 8px;
    border-radius: 4px;
    border: 1px solid $border-subtle;
    transition: all 0.15s ease;

    &:hover {
      border-color: $border-glow;
      color: $text-code;
    }
  }
}

.save-row {
  display: flex;
  justify-content: flex-end;
}
</style>
