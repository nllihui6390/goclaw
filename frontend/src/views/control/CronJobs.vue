<script setup>
import { ref, inject, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'

const api = inject('api')

const loading = ref(false)
const saving = ref(false)
const config = ref({})
const dialogVisible = ref(false)
const editingIndex = ref(-1)
const formRef = ref(null)

const formRules = {
  name: [{ required: true, message: '请输入任务名称', trigger: 'blur' }],
  schedule: [{ required: true, message: '请输入调度规则', trigger: 'blur' }],
  type: [{ required: true, message: '请选择任务类型', trigger: 'change' }],
  content: [{ required: true, message: '请输入内容', trigger: 'blur' }],
}

// 表单数据
const formData = ref({
  name: '', schedule: '', type: 'text', content: '',
  agent_name: '', agent_prompt: '', session_id: '',
  active_start: '', active_end: ''
})

// 定时任务列表
const jobs = computed(() => config.value?.cron?.jobs || [])

// Agent 列表
const agentNames = computed(() => {
  return config.value?.agents?.map(a => a.name) || []
})

// 加载配置
onMounted(loadConfig)

async function loadConfig() {
  loading.value = true
  try {
    config.value = await api.getConfig() || {}
  } catch (e) {
    ElMessage.error('加载失败: ' + e.message)
  }
  loading.value = false
}

// 保存全部配置
async function saveConfig() {
  saving.value = true
  try {
    await api.saveConfig(config.value)
    ElMessage.success('保存成功')
    await loadConfig()
  } catch (e) {
    ElMessage.error('保存失败: ' + e.message)
  }
  saving.value = false
}

// 打开新增对话框
function openAdd() {
  editingIndex.value = -1
  formData.value = {
    name: '', schedule: '', type: 'text', content: '',
    agent_name: '', agent_prompt: '', session_id: '',
    active_start: '', active_end: ''
  }
  dialogVisible.value = true
}

// 打开编辑对话框
function openEdit(index) {
  editingIndex.value = index
  formData.value = { ...jobs.value[index] }
  dialogVisible.value = true
}

// 保存任务
function saveJob() {
  if (!formData.value.name || !formData.value.schedule) return
  if (editingIndex.value >= 0) {
    jobs.value[editingIndex.value] = { ...formData.value }
  } else {
    if (!config.value.cron) config.value.cron = { enabled: true, jobs: [] }
    jobs.value.push({ ...formData.value })
  }
  dialogVisible.value = false
  saveConfig()
}

// 删除任务
async function deleteJob(index) {
  try {
    await ElMessageBox.confirm('确定删除该定时任务？', '确认删除', { type: 'warning' })
    jobs.value.splice(index, 1)
    await saveConfig()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error('删除失败: ' + e.message)
  }
}

// 立即执行
async function runJob(index) {
  try {
    const job = jobs.value[index]
    await api.runCronJob(job.name)
    ElMessage.success('任务已触发')
  } catch (e) {
    ElMessage.error('触发失败: ' + e.message)
  }
}

// 切换启用状态
async function toggleEnabled() {
  if (!config.value.cron) config.value.cron = { enabled: true, jobs: [] }
  config.value.cron.enabled = !config.value.cron.enabled
  await saveConfig()
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
            <el-tag :type="config.cron?.enabled ? 'success' : 'info'" size="small" class="status-tag">
              {{ config.cron?.enabled ? '运行中' : '已禁用' }}
            </el-tag>
          </div>
          <div class="toolbar">
            <el-switch v-model="config.cron.enabled" @change="toggleEnabled" active-text="启用" />
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

    <!-- 新增/编辑对话框 -->
    <el-dialog
      v-model="dialogVisible"
      :title="editingIndex >= 0 ? '编辑定时任务' : '新增定时任务'"
      width="550px"
      :close-on-click-modal="false"
    >
      <el-form ref="formRef" :model="formData" :rules="formRules" label-width="100px">
        <el-form-item label="名称" prop="name">
          <el-input v-model="formData.name" placeholder="如: 每日天气提醒" />
        </el-form-item>
        <el-form-item label="调度规则" prop="schedule">
          <el-input v-model="formData.schedule" placeholder="如: @every 5m 或 09:00" />
          <span class="form-tip">支持 @every 5m（每5分钟）、09:00（每天9点）或标准 cron 表达式</span>
        </el-form-item>
        <el-form-item label="类型" prop="type">
          <el-radio-group v-model="formData.type">
            <el-radio value="text">文本消息</el-radio>
            <el-radio value="agent">Agent 任务</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="内容" prop="content">
          <el-input v-model="formData.content" type="textarea" :rows="3" placeholder="消息内容或 Agent 指令" />
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
.status-tag { font-size: 12px; }
.toolbar { display: flex; gap: 12px; align-items: center; }
.content-text { font-size: 12px; color: #606266; font-family: monospace; }
.form-tip { font-size: 12px; color: #909399; margin-top: 4px; display: block; }
</style>