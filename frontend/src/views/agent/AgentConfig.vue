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
const isEditing = ref(false)  // 区分新增/编辑模式

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

// 供应商选项
const providerOptions = computed(() => {
  return Object.keys(providers.value).map(k => ({
    label: k,
    value: k
  }))
})

// 当前选中供应商的模型列表
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
    // 同步更新共享的 agent 列表，供 Header 等组件使用
    agentStore.setAgentList(agents.value)
    // API 返回数组 [{name, type, models, ...}]，转为 {name: {...}} 便于查找
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
    max_iterations: 20,
    ...agent
  }
  dialogVisible.value = true
}

// 切换供应商时清空模型选择
function onProviderChange() {
  currentAgent.value.model = ''
}

async function saveAgent() {
  if (!formRef.value) return
  try {
    await formRef.value.validate()
  } catch {
    return  // 验证不通过
  }
  try {
    await api.updateAgent(currentAgent.value.name, currentAgent.value)
    ElMessage.success('保存成功')
    await loadData()
    dialogVisible.value = false
  } catch (e) {
    ElMessage.error('保存失败: ' + e.message)
  }
}

// 删除 Agent（default 内置 agent 不可删除）
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
</script>

<template>
  <div class="page">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>Agent 配置</span>
          <el-button type="primary" size="small" @click="openEdit({})">新增</el-button>
        </div>
      </template>
      <el-table :data="agents" v-loading="loading" stripe>
        <el-table-column align="center" prop="name" label="Agent ID" width="100" />
        <el-table-column prop="display_name" label="名称" width="120" />
        <el-table-column prop="description" label="描述" min-width="150" show-overflow-tooltip />
        <el-table-column align="center"prop="provider" label="供应商" width="100" />
        <el-table-column align="center" prop="model" label="模型" width="150" />
        <el-table-column align="center" label="工具" min-width="120">
          <template #default="{ row }">
            <span>{{ row.tools?.length || 0 }}</span>
          </template>
        </el-table-column>
        <el-table-column align="center" label="操作" width="150">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" :disabled="row.name === 'default'" @click="deleteAgent(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="currentAgent?.name ? '编辑 Agent' : '新增 Agent'" width="650px">
      <el-form ref="formRef" :model="currentAgent" :rules="formRules" label-width="100px" v-if="currentAgent">
        <el-form-item label="Agent ID" prop="name">
          <el-input v-model="currentAgent.name" placeholder="唯一标识，如 default、local" :disabled="isEditing" />
        </el-form-item>
        <el-form-item label="名称" prop="display_name">
          <el-input v-model="currentAgent.display_name" placeholder="中文展示名称，如 默认助手" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="currentAgent.description" type="textarea" :rows="2" placeholder="描述该 Agent 的用途" />
        </el-form-item>
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
        <el-form-item label="最大迭代">
          <el-input-number v-model="currentAgent.max_iterations" :min="1" :max="50" />
        </el-form-item>
        <el-form-item label="系统提示词">
          <el-input v-model="currentAgent.system_prompt" type="textarea" :rows="4" />
        </el-form-item>
        <el-form-item label="工具">
          <el-checkbox-group v-model="currentAgent.tools">
            <el-checkbox v-for="t in toolsOptions" :key="t" :value="t">
              {{ toolsStore.getTool(t).icon }} {{ t }}
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

<style scoped>
.page { padding: 24px; }
.card-header { display: flex; justify-content: space-between; align-items: center; }
.mr1 { margin-right: 4px; }
</style>