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

// Agent 列表
const agentNames = computed(() => {
  return config.value?.agents?.map(a => a.name) || []
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

// 启用/禁用整个 cron（写入 config.json）
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

// 打开新增
function openAdd() {
  editingIndex.value = -1
  formData.value = {
    name: '', schedule: '', type: 'text', content: '',
    agent_name: '', session_id: '',
    active_start: '', active_end: ''
  }
  dialogVisible.value = true
}

// 打开编辑
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

// 保存任务
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
      job.id = 'job_' + formData.value.name + '_' + Date.now()
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

// 删除任务
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

// 立即执行
async function runJob(index) {
  try {
    const job = jobs.value[index]
    await api.runCronJob(job.id)
    ElMessage.success('任务已触发')
  } catch (e) {
    ElMessage.error('触发失败: ' + e.message)
  }
}

function formatSchedule(schedule) {
  if (!schedule) return '-'
  if (schedule.startsWith('@every')) return `每 ${schedule.replace('@every ', '')}`
  if (schedule.includes(':') && schedule.split(':').length === 2) return `每天 ${schedule}`
  return schedule
}
</script>

<template>
  <div class="page" v-loading="loading">
    <el-card>
      <template #header>
        <div class="card-header">
          <div class="header-left">
            <span>定时任务</span>
            <el-tag :type="cronEnabled ? 'success' : 'info'" size="small">
              {{ cronEnabled ? '运行中' : '已禁用' }}
            </el-tag>
          </div>
          <div class="toolbar">
            <el-switch :model-value="cronEnabled" @change="toggleEnabled" active-text="启用" />
            <el-button type="primary" size="small" @click="openAdd">
              <el-icon><Plus /></el-icon>添加任务
            </el-button>
          </div>
        </div>
      </template>

      <el-table :data="jobs" stripe>
        <el-table-column prop="name" label="名称" min-width="120" />
        <el-table-column prop="schedule" label="调度" width="150">
          <template #default="{ row }">
            <el-tag size="small" effect="plain">{{ formatSchedule(row.schedule) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="type" label="类型" width="90">
          <template #default="{ row }">
            <el-tag :type="row.type === 'text' ? 'primary' : 'success'" size="small">
              {{ row.type === 'text' ? '文本' : 'Agent' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="content" label="内容" min-width="180">
          <template #default="{ row }">
            <span class="content-text">{{ row.content?.slice(0, 40) }}{{ row.content?.length > 40 ? '...' : '' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="agent_name" label="Agent" width="80" />
        <el-table-column label="操作" width="200" fixed="right">
          <template #default="{ row, $index }">
            <el-button link type="primary" @click="openEdit($index)">编辑</el-button>
            <el-button link type="success" @click="runJob($index)">执行</el-button>
            <el-button link type="danger" @click="deleteJob($index)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-if="!jobs.length && !loading" description="暂无定时任务" />
    </el-card>

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
          <el-input v-model="formData.session_id" placeholder="如: wecom:lhh15698（消息发送目标）" />
          <span class="form-tip">格式: 渠道:用户，如 wecom:lhh15698、console:admin</span>
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

<style scoped>
.page { padding: 24px; }
.card-header { display: flex; justify-content: space-between; align-items: center; }
.header-left { display: flex; align-items: center; gap: 10px; font-size: 16px; font-weight: 500; }
.toolbar { display: flex; gap: 12px; align-items: center; }
.content-text { font-size: 12px; color: #606266; font-family: monospace; }
.form-tip { font-size: 12px; color: #909399; margin-top: 4px; display: block; }
</style>
