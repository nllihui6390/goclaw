<script setup>
import { ref, inject, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'

const api = inject('api')

const loading = ref(false)
const saving = ref(false)
const config = ref({})
const dialogVisible = ref(false)
const modelDialogVisible = ref(false)
const addProviderDialogVisible = ref(false)
const editingProvider = ref(null)
const editingModels = ref([])
const editingModel = ref(null)
const currentProviderName = ref('')
const currentModelIdx = ref(-1)

// 搜索关键字
const searchKeyword = ref('')

// 默认模型选择
const defaultProvider = ref('')
const defaultModel = ref('')

// 新增供应商表单
const newProvider = ref({ name: '', type: 'openai', base_url: '', api_key: '', models: [] })

// 供应商可选模型列表
const providerModelOptions = computed(() => {
  const p = config.value?.providers?.[defaultProvider.value]
  return p?.models?.map(m => ({ label: m.description || m.name, value: m.name })) || []
})

// 过滤后的供应商列表
const filteredProviders = computed(() => {
  if (!config.value?.providers) return []
  const list = Object.entries(config.value.providers).map(([name, cfg]) => ({
    name,
    type: cfg.type,
    base_url: cfg.base_url,
    api_key: cfg.api_key,
    models: cfg.models || [],
    modelsCount: cfg.models?.length || 0
  }))
  if (!searchKeyword.value) return list
  const kw = searchKeyword.value.toLowerCase()
  return list.filter(p =>
    p.name.toLowerCase().includes(kw) ||
    p.type.toLowerCase().includes(kw) ||
    p.base_url?.toLowerCase().includes(kw)
  )
})

// 供应商下拉选项
const providerOptions = computed(() => {
  if (!config.value?.providers) return []
  return Object.entries(config.value.providers).map(([name, cfg]) => ({
    label: name,
    value: name
  }))
})

// 供应商切换时重置模型选择
function onProviderChange() {
  defaultModel.value = ''
}

onMounted(async () => {
  await loadConfig()
})

// 加载配置
async function loadConfig() {
  loading.value = true
  try {
    config.value = await api.getConfig() || {}
    defaultProvider.value = config.value.gateway?.default_provider || ''
    defaultModel.value = config.value.gateway?.default_model || ''
  } catch (e) {
    ElMessage.error('加载配置失败: ' + e.message)
  }
  loading.value = false
}

// 保存默认模型
async function saveDefaultModel() {
  saving.value = true
  try {
    config.value.gateway.default_provider = defaultProvider.value
    config.value.gateway.default_model = defaultModel.value
    await api.saveConfig(config.value)
    ElMessage.success('默认模型已保存')
  } catch (e) {
    ElMessage.error('保存失败: ' + e.message)
  }
  saving.value = false
}

// 打开供应商设置对话框
function openProviderSettings(name, provider) {
  currentProviderName.value = name
  editingProvider.value = {
    type: provider.type || 'openai',
    base_url: provider.base_url || '',
    api_key: provider.api_key || ''
  }
  editingModels.value = provider.models ? [...provider.models.map(m => ({...m}))] : []
  dialogVisible.value = true
}

// 打开模型设置对话框
function openModelSettings(name, provider) {
  currentProviderName.value = name
  editingModels.value = provider.models ? [...provider.models.map(m => ({...m}))] : []
  modelDialogVisible.value = true
}

// 保存供应商设置
async function saveProviderSettings() {
  saving.value = true
  try {
    config.value.providers[currentProviderName.value] = {
      ...config.value.providers[currentProviderName.value],
      type: editingProvider.value.type,
      base_url: editingProvider.value.base_url,
      api_key: editingProvider.value.api_key
    }
    await api.saveConfig(config.value)
    ElMessage.success('供应商设置已保存')
    dialogVisible.value = false
  } catch (e) {
    ElMessage.error('保存失败: ' + e.message)
  }
  saving.value = false
}

// 保存模型设置
async function saveModelSettings() {
  saving.value = true
  try {
    config.value.providers[currentProviderName.value] = {
      ...config.value.providers[currentProviderName.value],
      models: editingModels.value
    }
    await api.saveConfig(config.value)
    ElMessage.success('模型设置已保存')
    modelDialogVisible.value = false
  } catch (e) {
    ElMessage.error('保存失败: ' + e.message)
  }
  saving.value = false
}

// 添加模型
function addModel() {
  editingModels.value.push({ name: '', description: '' })
}

// 删除模型
function removeModel(idx) {
  editingModels.value.splice(idx, 1)
}

// 打开新增供应商对话框
function openAddProvider() {
  newProvider.value = { name: '', type: 'openai', base_url: '', api_key: '', models: [] }
  addProviderDialogVisible.value = true
}

// 保存新增供应商
async function saveNewProvider() {
  if (!newProvider.value.name) {
    ElMessage.warning('请输入供应商名称')
    return
  }
  saving.value = true
  try {
    if (!config.value.providers) config.value.providers = {}
    config.value.providers[newProvider.value.name] = {
      type: newProvider.value.type,
      base_url: newProvider.value.base_url,
      api_key: newProvider.value.api_key,
      models: []
    }
    await api.saveConfig(config.value)
    ElMessage.success('供应商已添加')
    addProviderDialogVisible.value = false
    await loadConfig()
  } catch (e) {
    ElMessage.error('添加失败: ' + e.message)
  }
  saving.value = false
}

// 删除供应商
async function deleteProvider(name) {
  try {
    await ElMessageBox.confirm(`确定删除供应商 "${name}"？`, '确认删除', { type: 'warning' })
    delete config.value.providers[name]
    await api.saveConfig(config.value)
    ElMessage.success('删除成功')
    await loadConfig()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error('删除失败: ' + e.message)
  }
}

function maskKey(key) {
  if (!key) return '(未设置)'
  if (key.length < 8) return '****'
  return key.slice(0, 4) + '****' + key.slice(-4)
}

const providerTypeMap = { openai: 'OpenAI', ollama: 'Ollama' }
</script>

<template>
  <div class="page" v-loading="loading">
    <!-- 上部分：默认模型配置 -->
    <el-card class="default-card">
      <div class="page-header">
        <div class="header-left"><h3>默认模型</h3></div>
      </div>
      <div class="default-row">
        <el-select v-model="defaultProvider" placeholder="选择供应商" style="width: 160px" @change="onProviderChange">
          <el-option v-for="p in providerOptions" :key="p.value" :label="p.label" :value="p.value" />
        </el-select>
        <el-select v-model="defaultModel" placeholder="选择模型" style="width: 200px">
          <el-option v-for="m in providerModelOptions" :key="m.value" :label="m.label" :value="m.value" />
        </el-select>
        <el-button type="primary" @click="saveDefaultModel" :loading="saving">保存</el-button>
      </div>
      <p class="default-tip">在这里设置全局默认的 LLM 模型。你也可以在聊天页面为具体 Agent 单独选择使用的模型。</p>
    </el-card>

    <!-- 下半部分：提供商 -->
    <div class="page-header">
      <div class="header-left"><h2>提供商配置</h2></div>
      <div class="header-actions">
        <el-input v-model="searchKeyword" placeholder="搜索供应商" clearable style="width: 180px">
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
        <el-button @click="loadConfig" :loading="loading" title="刷新">
          <el-icon><Refresh /></el-icon>
        </el-button>
        <el-button type="primary" @click="openAddProvider">
          <el-icon><Plus /></el-icon>添加
        </el-button>
      </div>
    </div>
    <div class="providers-grid">
      <el-card v-for="p in filteredProviders" :key="p.name" class="provider-card">
        <div class="provider-top">
          <div class="provider-info">
            <span class="provider-name">{{ p.name }}</span>
            <el-tag :type="p.type === 'ollama' ? 'success' : 'primary'" size="small">{{ providerTypeMap[p.type] || p.type }}</el-tag>
          </div>
          <div class="provider-detail">
            <div class="detail-item"><span class="detail-label">API 地址</span><span class="detail-value mono">{{ p.base_url }}</span></div>
            <div class="detail-item"><span class="detail-label">API Key</span><span class="detail-value mono">{{ maskKey(p.api_key) }}</span></div>
            <div class="detail-item"><span class="detail-label">模型数</span><span class="detail-value">{{ p.modelsCount }}</span></div>
          </div>
        </div>
        <div class="provider-models">
          <el-tag v-for="m in p.models?.slice(0, 3)" :key="m.name" size="small" class="model-tag">{{ m.name }}</el-tag>
          <span v-if="p.models?.length > 3" class="more-models">+{{ p.models.length - 3 }}</span>
        </div>
        <div class="provider-actions">
          <el-button size="small" @click="openModelSettings(p.name, p)">模型设置</el-button>
          <el-button size="small" type="primary" @click="openProviderSettings(p.name, p)">设置</el-button>
          <el-button size="small" type="danger" link @click="deleteProvider(p.name)">删除</el-button>
        </div>
      </el-card>
      <el-empty v-if="!filteredProviders.length && !loading" description="暂无供应商" />
    </div>

    <!-- 新增供应商对话框 -->
    <el-dialog v-model="addProviderDialogVisible" title="新增供应商" width="450px">
      <el-form :model="newProvider" label-width="100px">
        <el-form-item label="名称" required>
          <el-input v-model="newProvider.name" placeholder="如: deepseek, openai" />
        </el-form-item>
        <el-form-item label="类型">
          <el-select v-model="newProvider.type" style="width: 100%">
            <el-option label="OpenAI Compatible" value="openai" />
            <el-option label="Ollama (Local)" value="ollama" />
          </el-select>
        </el-form-item>
        <el-form-item label="API 地址">
          <el-input v-model="newProvider.base_url" placeholder="https://api.openai.com/v1" />
        </el-form-item>
        <el-form-item label="API Key">
          <el-input v-model="newProvider.api_key" type="password" show-password />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addProviderDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveNewProvider" :loading="saving">保存</el-button>
      </template>
    </el-dialog>

    <!-- 供应商设置对话框 -->
    <el-dialog v-model="dialogVisible" title="供应商设置" width="450px">
      <el-form :model="editingProvider" label-width="100px" v-if="editingProvider">
        <el-form-item label="名称">
          <el-input :value="currentProviderName" disabled />
        </el-form-item>
        <el-form-item label="类型">
          <el-select v-model="editingProvider.type" style="width: 100%">
            <el-option label="OpenAI Compatible" value="openai" />
            <el-option label="Ollama (Local)" value="ollama" />
          </el-select>
        </el-form-item>
        <el-form-item label="API 地址">
          <el-input v-model="editingProvider.base_url" />
        </el-form-item>
        <el-form-item label="API Key">
          <el-input v-model="editingProvider.api_key" type="password" show-password />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveProviderSettings" :loading="saving">保存</el-button>
      </template>
    </el-dialog>

    <!-- 模型设置对话框 -->
    <el-dialog v-model="modelDialogVisible" title="模型设置" width="550px">
      <div class="model-edit-header">
        <span>{{ currentProviderName }} 的模型列表</span>
        <el-button type="primary" size="small" @click="addModel">添加模型</el-button>
      </div>
      <div class="model-edit-list">
        <div v-for="(m, idx) in editingModels" :key="idx" class="model-edit-item">
          <el-input v-model="m.name" placeholder="模型名称" style="width: 160px" />
          <el-input v-model="m.description" placeholder="描述" style="width: 200px" />
          <el-button type="danger" link size="small" @click="removeModel(idx)">删除</el-button>
        </div>
        <el-empty v-if="!editingModels.length" description="暂无模型" :image-size="50" />
      </div>
      <template #footer>
        <el-button @click="modelDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveModelSettings" :loading="saving">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page { padding: 24px; }
.page-header {
  display: flex;justify-content: space-between;align-items: center;
  margin-bottom: 24px;flex-wrap: wrap;gap: 12px;
}
.header-left { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.header-left h2 { margin: 0; font-weight: 500; }
.skill-info { color: #909399; font-size: 13px; display: flex; align-items: center; gap: 6px; }
.header-actions { display: flex; gap: 8px; }

.card-header { display: flex; justify-content: space-between; align-items: center; font-size: 16px; font-weight: 500; }
.toolbar { display: flex; gap: 8px; align-items: center; }

/* 上部分：默认模型 */
.default-card { margin-bottom: 20px; }
.default-row { display: flex; align-items: center; gap: 12px; }
.default-tip { margin-top: 10px; font-size: 13px; color: #909399; line-height: 1.5; }

/* 下半部分：提供商卡片 */
.providers-card { margin-bottom: 0; }
.providers-card :deep(.el-card__body) { padding: 16px; }
.providers-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(340px, 1fr)); gap: 16px; }

.provider-card { transition: all .2s; }
.provider-card:hover { box-shadow: 0 2px 12px rgba(0,0,0,.1); }

.provider-top { margin-bottom: 12px; }
.provider-info { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.provider-name { font-size: 16px; font-weight: 600; }

.provider-detail { font-size: 12px; color: #909399; }
.detail-item { display: flex; gap: 4px; margin-bottom: 4px; }
.detail-label { color: #606266; width: 60px; }
.detail-value { color: #303133; }
.mono { font-family: 'Consolas', 'Monaco', monospace; }

.provider-models { margin-bottom: 12px; }
.model-tag { margin-right: 4px; }
.more-models { font-size: 12px; color: #909399; }

.provider-actions { display: flex; gap: 8px; align-items: center; }

/* 模型编辑 */
.model-edit-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; }
.model-edit-list { max-height: 400px; overflow-y: auto; }
.model-edit-item { display: flex; gap: 8px; align-items: center; margin-bottom: 8px; }
</style>