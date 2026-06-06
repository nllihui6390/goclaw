<script setup>
import { ref, inject, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'

const api = inject('api')

const loading = ref(false)
const saving = ref(false)
const config = ref({})
const cronEnabled = ref(true)
const jobs = ref([])
const dialogVisible = ref(false)
const editingIndex = ref(-1)

const formData = ref({
  name: '', schedule: '', type: 'text', content: '',
  agent_name: '', session_id: '',
  active_start: '', active_end: ''
})

const agentNames = computed(() => {
  // 新格式：agents.profiles 是对象，agents.order 是名称排序
  const agents = config.value?.agents
  if (agents?.order) return agents.order
  if (agents?.profiles) return Object.keys(agents.profiles)
  // 旧格式兼容：agents 是数组
  if (Array.isArray(agents)) return agents.map(a => a.name)
  return []
})

onMounted(loadData)

async function loadData() {
  loading.value = true
  try {
    const [cfg, jobList] = await Promise.all([
      api.getConfig(),
      api.getCronJobs()
    ])
    config.value = cfg || {}
    cronEnabled.value = config.value.cron?.enabled !== false
    jobs.value = (jobList || []).filter(j => j !== null)
  } catch (e) {
    ElMessage.error('加载失败: ' + e.message)
  }
  loading.value = false
}

async function toggleEnabled(val) {
  cronEnabled.value = val
  if (!config.value.cron) config.value.cron = { enabled: val }
  config.value.cron.enabled = val
  try {
    await api.saveConfig(config.value)
    ElMessage.success('定时任务已' + (cronEnabled.value ? '启用' : '禁用'))
  } catch (e) {
    ElMessage.error('保存失败: ' + e.message)
  }
}

function openAdd() {
  editingIndex.value = -1
  formData.value = {
    name: '', schedule: '', type: 'text', content: '',
    agent_name: '', session_id: '',
    active_start: '', active_end: ''
  }
  dialogVisible.value = true
}

function openEdit(index) {
  editingIndex.value = index
  const job = jobs.value[index]
  formData.value = {
    name: job.name || '',
    schedule: job.schedule || '',
    type: job.type || 'text',
    content: job.content || '',
    agent_name: job.agent_name || '',
    session_id: job.session_id || '',
    active_start: job.active_start || '',
    active_end: job.active_end || ''
  }
  dialogVisible.value = true
}

async function saveJob() {
  if (!formData.value.name || !formData.value.schedule) return
  const job = {
    name: formData.value.name,
    schedule: formData.value.schedule,
    type: formData.value.type,
    content: formData.value.content,
    agent_name: formData.value.agent_name,
    session_id: formData.value.session_id || (config.value.cron?.default_channel || 'console') + ':' + (config.value.cron?.default_user || 'cron'),
    active_start: formData.value.active_start || '',
    active_end: formData.value.active_end || '',
  }

  saving.value = true
  try {
    if (editingIndex.value >= 0) {
      const oldJob = jobs.value[editingIndex.value]
      job.id = oldJob.id
      job.enabled = oldJob.enabled ?? true
      job.last_run = oldJob.last_run || '0001-01-01T00:00:00Z'
      job.next_run = ''
      await api.updateCronJob(job.id, job)
      ElMessage.success('任务已更新')
    } else {
      job.id = ''
      job.enabled = true
      job.last_run = '0001-01-01T00:00:00Z'
      job.next_run = ''
      await api.addCronJob(job)
      ElMessage.success('任务已添加')
    }
    dialogVisible.value = false
    await loadData()
  } catch (e) {
    ElMessage.error('保存失败: ' + e.message)
  }
  saving.value = false
}

async function deleteJob(index) {
  try {
    await ElMessageBox.confirm('确定删除该定时任务？', '确认删除', { type: 'warning' })
    const job = jobs.value[index]
    await api.deleteCronJob(job.id)
    ElMessage.success('删除成功')
    await loadData()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error('删除失败: ' + e.message)
  }
}

async function runJob(index) {
  try {
    const job = jobs.value[index]
    await api.runCronJob(job.id)
    ElMessage.success('任务已触发')
  } catch (e) {
    ElMessage.error('触发失败: ' + e.message)
  }
}

function formatTime(ts) {
  if (!ts || ts.startsWith('0001')) return '-'
  try {
    return new Date(ts).toLocaleString('zh-CN')
  } catch { return '-' }
}

function formatSchedule(schedule) {
  if (!schedule) return '-'
  if (schedule.startsWith('@every')) return `每 ${schedule.replace('@every ', '')}`
  if (schedule.includes(':') && schedule.split(':').length === 2) return `每天 ${schedule}`
  return schedule
}

function getTypeIcon(type) {
  return type === 'agent' ? 'Cpu' : 'ChatDotRound'
}
</script>

<template>
  <div class="page" v-loading="loading">
    <!-- Page header -->
    <div class="page-header">
      <div class="header-left">
        <h2>定时任务</h2>
        <el-tag :type="cronEnabled ? 'success' : 'info'" size="small">
          {{ cronEnabled ? '运行中' : '已禁用' }}
        </el-tag>
        <span class="job-count">{{ jobs.length }} 个任务</span>
      </div>
      <div class="header-actions">
        <el-switch :model-value="cronEnabled" @change="toggleEnabled" active-text="启用" />
        <el-button type="primary" @click="openAdd">
          <el-icon><Plus /></el-icon>添加任务
        </el-button>
        <el-button @click="loadData">
          <el-icon><Refresh /></el-icon>刷新
        </el-button>
      </div>
    </div>

    <!-- Job cards grid -->
    <div class="jobs-grid" v-if="jobs.length">
      <div v-for="(job, index) in jobs" :key="job.id" class="job-card">
        <!-- Card top: name + type -->
        <div class="card-top">
          <div class="job-icon-wrap">
            <el-icon :size="20"><component :is="getTypeIcon(job.type)" /></el-icon>
          </div>
          <div class="job-info">
            <span class="job-name">{{ job.name }}</span>
            <div class="job-tags">
              <el-tag :type="job.type === 'text' ? 'primary' : 'success'" size="small">
                {{ job.type === 'text' ? '文本' : 'Agent' }}
              </el-tag>
              <el-tag size="small" effect="plain">{{ formatSchedule(job.schedule) }}</el-tag>
            </div>
          </div>
          <el-switch
            :model-value="job.enabled ?? true"
            size="small"
          />
        </div>

        <!-- Card body: content preview -->
        <div class="card-body">
          <p class="job-content">{{ job.content }}</p>
          <div class="job-meta">
            <span v-if="job.agent_name" class="meta-item">
              <el-icon :size="14"><Cpu /></el-icon>
              <span>Agent: {{ job.agent_name }}</span>
            </span>
            <span v-if="job.session_id" class="meta-item">
              <el-icon :size="14"><ChatLineSquare /></el-icon>
              <span>{{ job.session_id }}</span>
            </span>
          </div>
        </div>

        <!-- Card footer: time info + actions -->
        <div class="card-footer">
          <div class="time-info">
            <div class="time-item">
              <span class="time-label">上次执行</span>
              <span class="time-value">{{ formatTime(job.last_run) }}</span>
            </div>
            <div class="time-item">
              <span class="time-label">下次执行</span>
              <span class="time-value">{{ formatTime(job.next_run) }}</span>
            </div>
          </div>
          <div class="card-actions">
            <el-button size="small" @click="openEdit(index)">编辑</el-button>
            <el-button size="small" type="success" @click="runJob(index)">执行</el-button>
            <el-button size="small" type="danger" @click="deleteJob(index)">删除</el-button>
          </div>
        </div>
      </div>
    </div>

    <el-empty v-if="!jobs.length && !loading" description="暂无定时任务">
      <template #extra>
        <p class="empty-hint">点击"添加任务"创建定时任务</p>
      </template>
    </el-empty>

    <!-- Edit dialog -->
    <el-dialog
      v-model="dialogVisible"
      :title="editingIndex >= 0 ? '编辑定时任务' : '新增定时任务'"
      width="550px"
      :close-on-click-modal="false"
    >
      <el-form :model="formData" label-width="100px">
        <el-form-item label="名称" required>
          <el-input v-model="formData.name" placeholder="如: 每日天气提醒" />
        </el-form-item>

        <el-form-item label="调度规则" required>
          <el-input v-model="formData.schedule" placeholder="如: @every 5m 或 09:00" />
          <span class="form-tip">支持 @every 5m、09:00 或标准 cron 表达式</span>
        </el-form-item>

        <el-form-item label="类型">
          <el-radio-group v-model="formData.type">
            <el-radio value="text">文本消息</el-radio>
            <el-radio value="agent">Agent 任务</el-radio>
          </el-radio-group>
        </el-form-item>

        <el-form-item label="内容" required>
          <el-input v-model="formData.content" type="textarea" :rows="3" placeholder="消息内容或 Agent 指令" />
        </el-form-item>

        <el-form-item label="会话 ID">
          <el-input v-model="formData.session_id" placeholder="如: wecom:user（消息发送目标）" />
          <span class="form-tip">格式: 渠道:用户，如 wecom:user、console:admin</span>
        </el-form-item>

        <el-form-item label="Agent" v-if="formData.type === 'agent'">
          <el-select v-model="formData.agent_name" style="width: 100%" placeholder="选择 Agent">
            <el-option v-for="n in agentNames" :key="n" :label="n" :value="n" />
          </el-select>
        </el-form-item>

        <el-form-item label="活跃开始" v-if="formData.type === 'agent'">
          <el-input v-model="formData.active_start" placeholder="如: 09:00（可选）" />
        </el-form-item>

        <el-form-item label="活跃结束" v-if="formData.type === 'agent'">
          <el-input v-model="formData.active_end" placeholder="如: 18:00（可选）" />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveJob" :loading="saving">保存</el-button>
      </template>
    </el-dialog>
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

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-left h2 {
  margin: 0;
  font-size: $font-size-xl;
  font-weight: 600;
  color: $text-primary;
}

.job-count {
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
  gap: 12px;
  align-items: center;
}

// ──── Job cards grid ────
.jobs-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
  gap: 16px;
}

.job-card {
  @include glass-panel;
  border-radius: $radius-lg;
  padding: 20px;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  display: flex;
  flex-direction: column;
  gap: 16px;

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

.job-icon-wrap {
  width: 44px;
  height: 44px;
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

.job-info {
  flex: 1;
  min-width: 0;
}

.job-name {
  font-size: $font-size-lg;
  font-weight: 600;
  color: $text-primary;
  display: block;
  margin-bottom: 6px;
}

.job-tags {
  display: flex;
  gap: 6px;
}

// ──── Card body ────
.card-body {
  flex: 1;
}

.job-content {
  font-size: $font-size-sm;
  color: $text-secondary;
  line-height: 1.5;
  margin: 0 0 10px 0;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.job-meta {
  display: flex;
  gap: 16px;
  font-size: $font-size-xs;
  color: $text-muted;
}

.meta-item {
  display: flex;
  align-items: center;
  gap: 4px;
  font-family: $font-display;
}

// ──── Card footer ────
.card-footer {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 16px;
}

.time-info {
  display: flex;
  gap: 20px;
}

.time-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.time-label {
  font-size: $font-size-xs;
  color: $text-muted;
  font-family: $font-display;
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

.empty-hint {
  color: $text-muted;
  font-size: $font-size-sm;
  text-align: center;
}

.form-tip {
  font-size: $font-size-xs;
  color: $text-muted;
  margin-top: 4px;
  display: block;
}

// ──── Mobile ────
@media (max-width: 768px) {
  .page { padding: 16px; }
  .jobs-grid {
    grid-template-columns: 1fr;
  }
  .card-footer {
    flex-direction: column;
    align-items: flex-start;
  }
  .card-actions {
    width: 100%;
    justify-content: flex-end;
  }
}
</style>