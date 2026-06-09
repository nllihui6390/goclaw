<script setup>
import { ref, inject, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useToolsStore } from '@/stores/tools'
import { useAgentStore } from '@/stores/agent'

const api = inject('api')
const agentStore = useAgentStore()
const agents = ref([])
const providers = ref([])
const loading = ref(false)
const currentAgent = ref(null)
const dialogVisible = ref(false)
const formRef = ref(null)
const isEditing = ref(false)

const toolsStore = useToolsStore()
const toolsOptions = computed(() => toolsStore.allToolNames)

const formRules = {
  name: [
    { required: true, message: '请输入 Agent ID', trigger: 'blur' },
    { pattern: /^[a-zA-Z0-9_-]+$/, message: '仅允许字母、数字、下划线、连字符', trigger: 'blur' }
  ],
  display_name: [
    { required: true, message: '请输入名称', trigger: 'blur' }
  ]
}

const providerOptions = computed(() => {
  return Object.keys(providers.value).map(k => ({
    label: k,
    value: k
  }))
})

const modelOptions = computed(() => {
  if (!currentAgent.value?.provider) return []
  const p = providers.value[currentAgent.value.provider]
  if (!p?.models) return []
  return p.models.map(m => ({
    label: m.description ? `${m.name} - ${m.description}` : m.name,
    value: m.name
  }))
})

async function loadData() {
  loading.value = true
  try {
    const [agList, provData] = await Promise.all([
      api.getAgents(),
      api.getProviders()
    ])
    agents.value = agList || []
    agentStore.setAgentList(agents.value)
    if (Array.isArray(provData)) {
      const map = {}
      provData.forEach(p => { map[p.name] = p })
      providers.value = map
    } else {
      providers.value = provData || {}
    }
  } catch (e) {
    ElMessage.error('加载失败: ' + e.message)
  }
  loading.value = false
}

onMounted(loadData)

function openEdit(agent) {
  isEditing.value = !!agent.name
  currentAgent.value = {
    name: '',
    display_name: '',
    description: '',
    provider: '',
    model: '',
    system_prompt: '',
    tools: [],
    max_iterations: 50,
    ...agent
  }
  dialogVisible.value = true
}

function onProviderChange() {
  currentAgent.value.model = ''
}

async function saveAgent() {
  if (!formRef.value) return
  try {
    await formRef.value.validate()
  } catch {
    return
  }
  try {
    if (isEditing.value) {
      await api.updateAgent(currentAgent.value.name, currentAgent.value)
    } else {
      await api.createAgent(currentAgent.value)
    }
    ElMessage.success('保存成功')
    await loadData()
    dialogVisible.value = false
  } catch (e) {
    ElMessage.error('保存失败: ' + e.message)
  }
}

async function deleteAgent(agent) {
  if (agent.name === 'default') {
    ElMessage.warning('default 为内置 Agent，无法删除')
    return
  }
  try {
    await ElMessageBox.confirm(
      `确定删除 Agent "${agent.display_name || agent.name}"？此操作不可恢复。`,
      '确认删除',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' }
    )
    await api.deleteAgent(agent.name)
    ElMessage.success('删除成功')
    await loadData()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error('删除失败: ' + e.message)
  }
}

function getProviderTypeColor(provider) {
  const type = providers.value[provider]?.type
  if (type === 'ollama') return 'success'
  return 'primary'
}

function getProviderType(provider) {
  return providers.value[provider]?.type || 'openai'
}

function selectAllTools() {
  if (!currentAgent.value) return
  currentAgent.value.tools = [...toolsOptions.value]
}

function selectNoneTools() {
  if (!currentAgent.value) return
  currentAgent.value.tools = []
}
</script>

<template>
  <div class="page" v-loading="loading">
    <!-- Page header -->
    <div class="page-header">
      <div class="header-title">
        <h2>Agent 配置</h2>
        <span class="agent-count">{{ agents.length }} 个 Agent</span>
      </div>
      <div class="header-actions">
        <el-button @click="loadData">
          <el-icon><Refresh /></el-icon>刷新
        </el-button>
        <el-button type="primary" @click="openEdit({})">
          <el-icon><Plus /></el-icon>新增 Agent
        </el-button>
      </div>
    </div>

    <!-- Agent cards grid -->
    <div class="agents-grid" v-if="agents.length">
      <div v-for="agent in agents" :key="agent.name" class="agent-card">
        <!-- Card top: icon + name + ID -->
        <div class="card-top">
          <div class="agent-icon-wrap">
            <el-icon :size="20"><Avatar /></el-icon>
          </div>
          <div class="agent-info">
            <span class="agent-name">{{ agent.display_name || agent.name }}</span>
            <span class="agent-id">{{ agent.name }}</span>
          </div>
          <el-tag v-if="agent.name === 'default'" size="small" type="info">内置</el-tag>
        </div>

        <!-- Card body: description + provider/model -->
        <div class="card-body">
          <p class="agent-desc">{{ agent.description || '暂无描述' }}</p>
          <div class="agent-meta">
            <div class="meta-item">
              <span class="meta-label">供应商</span>
              <el-tag size="small" :type="getProviderTypeColor(agent.provider)" effect="plain">
                {{ agent.provider || '—' }}
              </el-tag>
            </div>
            <div class="meta-item">
              <span class="meta-label">模型</span>
              <span class="meta-value">{{ agent.model || '—' }}</span>
            </div>
            <div class="meta-item">
              <span class="meta-label">工具</span>
              <span class="meta-value tools-count">{{ agent.tools?.length || 0 }} 个</span>
            </div>
          </div>
        </div>

        <!-- Card footer: actions -->
        <div class="card-footer">
          <el-button size="small" @click="openEdit(agent)">编辑配置</el-button>
          <el-button size="small" type="danger" :disabled="agent.name === 'default'" @click="deleteAgent(agent)">删除</el-button>
        </div>
      </div>
    </div>

    <el-empty v-if="!agents.length && !loading" description="暂无 Agent 配置" />

    <!-- Edit dialog -->
    <el-dialog
      v-model="dialogVisible"
      :title="currentAgent?.name ? '编辑 Agent' : '新增 Agent'"
      width="min(900px, calc(100vw - 32px))"
      :close-on-click-modal="false"
      class="agent-dialog"
    >
      <el-form ref="formRef" :model="currentAgent" :rules="formRules" label-width="80px" v-if="currentAgent">
        <el-form-item label="Agent_ID" prop="name">
          <el-input v-model="currentAgent.name" placeholder="唯一标识，如 default、local" :disabled="isEditing" />
        </el-form-item>

        <el-form-item label="名称" prop="display_name">
          <el-input v-model="currentAgent.display_name" placeholder="中文展示名称，如 默认助手" />
        </el-form-item>

        <el-form-item label="描述">
          <el-input v-model="currentAgent.description" type="textarea" :rows="2" placeholder="描述该 Agent 的用途" />
        </el-form-item>

        <div class="form-row">
          <el-form-item label="供应商">
            <el-select v-model="currentAgent.provider" placeholder="选择供应商" clearable @change="onProviderChange" style="width: 100%">
              <el-option v-for="p in providerOptions" :key="p.value" :label="p.label" :value="p.value" />
            </el-select>
          </el-form-item>

          <el-form-item label="模型">
            <el-select v-model="currentAgent.model" placeholder="选择模型" clearable filterable style="width: 100%">
              <el-option v-for="m in modelOptions" :key="m.value" :label="m.label" :value="m.value" />
            </el-select>
          </el-form-item>
        </div>

        <el-form-item label="最大迭代">
          <el-input-number v-model="currentAgent.max_iterations" :min="1" :max="50" />
        </el-form-item>

        <el-form-item label="系统提示词">
          <el-input v-model="currentAgent.system_prompt" type="textarea" :rows="4" />
        </el-form-item>

        <el-form-item label="工具">
          <div class="tools-header">
            <el-button size="small" link type="primary" @click="selectAllTools">全选</el-button>
            <el-button size="small" link type="primary" @click="selectNoneTools">全不选</el-button>
          </div>
          <el-checkbox-group v-model="currentAgent.tools">
            <el-checkbox v-for="t in toolsOptions" :key="t" :value="t">
              <span class="tool-checkbox">{{ toolsStore.getTool(t).icon }} {{ t }}</span>
            </el-checkbox>
          </el-checkbox-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveAgent">保存</el-button>
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

.agent-count {
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

// ──── Agent cards grid ────
.agents-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 16px;
}

.agent-card {
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

.agent-icon-wrap {
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

.agent-info {
  flex: 1;
  min-width: 0;
}

.agent-name {
  font-size: $font-size-lg;
  font-weight: 600;
  color: $text-primary;
  display: block;
  margin-bottom: 4px;
}

.agent-id {
  font-size: $font-size-xs;
  color: $text-muted;
  font-family: $font-display;
}

// ──── Card body ────
.card-body {
  flex: 1;
}

.agent-desc {
  font-size: $font-size-sm;
  color: $text-secondary;
  line-height: 1.5;
  margin: 0 0 12px 0;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.agent-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
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
  color: $text-primary;
  font-family: $font-display;
}

.tools-count {
  color: $accent-cyan;
}

// ──── Card footer ────
.card-footer {
  display: flex;
  gap: 8px;
}

.tool-checkbox {
  font-family: $font-display;
  font-size: $font-size-sm;
}

.tools-header {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
}

// 供应商和模型同行显示
.form-row {
  display: flex;
  gap: 16px;

  :deep(.el-form-item) {
    flex: 1;
    margin-bottom: 18px;
  }
}

// ──── Mobile ────
@media (max-width: 768px) {
  .page { padding: 16px; }
  .agents-grid {
    grid-template-columns: 1fr;
  }
  .form-row {
    flex-direction: column;
    gap: 0;
  }
}
</style>