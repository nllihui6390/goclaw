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
const testDialogVisible = ref(false)
const editingProvider = ref(null)
const editingModels = ref([])
const currentProviderName = ref('')
const searchKeyword = ref('')
const defaultProvider = ref('')
const defaultModel = ref('')
const newProvider = ref({ name: '', type: 'openai', base_url: '', api_key: '', models: [] })
const testingProvider = ref(false)
const testResults = ref(null)
const testingProviderName = ref('')

const providerModelOptions = computed(() => {
  const p = config.value?.providers?.[defaultProvider.value]
  return p?.models?.map(m => ({ label: m.description || m.name, value: m.name })) || []
})

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

const providerOptions = computed(() => {
  if (!config.value?.providers) return []
  return Object.entries(config.value.providers).map(([name, cfg]) => ({
    label: name,
    value: name
  }))
})

function onProviderChange() {
  defaultModel.value = ''
}

onMounted(async () => {
  await loadConfig()
})

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

function openModelSettings(name, provider) {
  currentProviderName.value = name
  editingModels.value = provider.models ? [...provider.models.map(m => ({...m}))] : []
  modelDialogVisible.value = true
}

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

function addModel() {
  editingModels.value.push({ name: '', description: '' })
}

function removeModel(idx) {
  editingModels.value.splice(idx, 1)
}

function openAddProvider() {
  newProvider.value = { name: '', type: 'openai', base_url: '', api_key: '', models: [] }
  addProviderDialogVisible.value = true
}

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

async function testAllModels(providerName) {
  testingProvider.value = true
  testResults.value = null
  testingProviderName.value = providerName
  testDialogVisible.value = true
  try {
    const results = await api.testAllModels(providerName)
    testResults.value = results
    await loadConfig() // 刷新配置以反映 supports_image 更新
    const successCount = results.filter(r => r.success).length
    const failCount = results.filter(r => !r.success && r.error).length
    if (failCount === 0) {
      ElMessage.success(`全部 ${successCount} 个模型多模态测试通过`)
    } else {
      ElMessage.warning(`${successCount} 个通过，${failCount} 个失败`)
    }
  } catch (e) {
    testResults.value = [{ success: false, error: e.message }]
    ElMessage.error('测试失败: ' + e.message)
  }
  testingProvider.value = false
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
    <!-- Default model section -->
    <div class="default-section">
      <div class="section-header">
        <h3>默认模型</h3>
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
      <p class="default-tip">设置全局默认的 LLM 模型，也可以在聊天页面为具体 Agent 单独选择。</p>
    </div>

    <!-- Providers section -->
    <div class="page-header">
      <div class="header-left">
        <h2>提供商配置</h2>
      </div>
      <div class="header-actions">
        <el-input v-model="searchKeyword" placeholder="搜索供应商" clearable style="width: 180px">
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
        <el-button @click="loadConfig" :loading="loading">
          <el-icon><Refresh /></el-icon>刷新
        </el-button>
        <el-button type="primary" @click="openAddProvider">
          <el-icon><Plus /></el-icon>添加
        </el-button>
      </div>
    </div>

    <div class="providers-grid">
      <div v-for="p in filteredProviders" :key="p.name" class="provider-card">
        <div class="provider-header">
          <div class="provider-icon">
            <el-icon :size="20"><Cpu /></el-icon>
          </div>
          <div class="provider-info">
            <span class="provider-name">{{ p.name }}</span>
            <el-tag :type="p.type === 'ollama' ? 'success' : 'primary'" size="small">
              {{ providerTypeMap[p.type] || p.type }}
            </el-tag>
          </div>
        </div>

        <div class="provider-details">
          <div class="detail-row">
            <span class="detail-label">API 地址</span>
            <span class="detail-value mono">{{ p.base_url || '—' }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">API Key</span>
            <span class="detail-value mono">{{ maskKey(p.api_key) }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">模型数</span>
            <span class="detail-value">{{ p.modelsCount }}</span>
          </div>
        </div>

        <div class="provider-models">
          <el-tag v-for="m in p.models?.slice(0, 3)" :key="m.name" size="small" class="model-tag">{{ m.name }}</el-tag>
          <span v-if="p.models?.length > 3" class="more-models">+{{ p.models.length - 3 }}</span>
        </div>

        <div class="provider-actions">
          <el-button size="small" @click="openModelSettings(p.name, p)">模型设置</el-button>
          <el-button size="small" type="primary" @click="openProviderSettings(p.name, p)">设置</el-button>
          <el-button size="small" type="warning" @click="testAllModels(p.name)" :loading="testingProvider">测试</el-button>
          <el-button size="small" type="danger" link @click="deleteProvider(p.name)">删除</el-button>
        </div>
      </div>

      <el-empty v-if="!filteredProviders.length && !loading" description="暂无供应商" />
    </div>

    <!-- Dialogs -->
    <el-dialog v-model="addProviderDialogVisible" title="新增供应商" width="450px">
      <el-form :model="newProvider" label-width="80px">
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

    <el-dialog v-model="modelDialogVisible" title="模型设置" width="550px">
      <div class="model-edit-header">
        <span>{{ currentProviderName }} 的模型列表</span>
        <el-button type="primary" size="small" @click="addModel">添加模型</el-button>
      </div>
      <div class="model-edit-list">
        <div v-for="(m, idx) in editingModels" :key="idx" class="model-edit-item">
          <el-input v-model="m.name" placeholder="模型名称" style="width: 140px" />
          <el-input v-model="m.description" placeholder="描述" style="width: 140px" />
          <el-tooltip content="支持图片输入（视觉）" placement="top">
            <el-switch v-model="m.supports_image" active-text="图片" inline-prompt size="small" style="--el-switch-on-color: #67c23a" />
          </el-tooltip>
          <el-tooltip content="支持视频输入" placement="top">
            <el-switch v-model="m.supports_video" active-text="视频" inline-prompt size="small" style="--el-switch-on-color: #409eff" />
          </el-tooltip>
          <el-button type="danger" link size="small" @click="removeModel(idx)">删除</el-button>
        </div>
        <el-empty v-if="!editingModels.length" description="暂无模型" :image-size="50" />
      </div>
      <template #footer>
        <el-button @click="modelDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveModelSettings" :loading="saving">保存</el-button>
      </template>
    </el-dialog>

    <!-- 测试对话框 -->
    <el-dialog v-model="testDialogVisible" title="多模态测试" width="520px" :close-on-click-modal="false">
      <div v-if="!testResults" class="test-loading">
        <el-icon class="is-loading"><Loading /></el-icon>
        <p>正在测试所有模型的多模态能力...</p>
      </div>
      <div v-else class="test-results">
        <div class="test-provider-header">
          <span>供应商：{{ testingProviderName }}</span>
          <span class="test-summary">{{ testResults.filter(r => r.success).length }}/{{ testResults.length }} 通过</span>
        </div>
        <div class="test-model-list">
          <div v-for="result in testResults" :key="result.model" class="test-model-item" :class="result.success ? 'pass' : 'fail'">
            <div class="test-model-header">
              <el-icon v-if="result.success" class="test-icon success"><CircleCheck /></el-icon>
              <el-icon v-else class="test-icon error"><CircleClose /></el-icon>
              <span class="test-model-name">{{ result.model }}</span>
              <span class="test-latency">{{ result.latency_ms }}ms</span>
            </div>
            <div v-if="result.success" class="test-model-status success">
              支持图片输入
            </div>
            <div v-else class="test-model-status error">
              {{ result.error || '不支持图片输入' }}
            </div>
          </div>
        </div>
      </div>
      <template #footer>
        <el-button @click="testDialogVisible = false">关闭</el-button>
        <el-button type="primary" @click="testAllModels(testingProviderName)" :loading="testingProvider" v-if="testResults">重新测试</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style lang="scss" scoped>
@use '@/styles/variables.scss' as *;

.page {
  padding: 32px;
}

// Default section
.default-section {
  @include glass-panel;
  border-radius: $radius-lg;
  padding: 20px;
  margin-bottom: 28px;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
}

.section-symbol {
  font-size: 16px;
  color: $accent-cyan;
}

.section-header h3 {
  margin: 0;
  font-size: $font-size-lg;
  font-weight: 600;
  color: $text-primary;
}

.default-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.default-tip {
  margin-top: 12px;
  font-size: $font-size-sm;
  color: $text-muted;
  line-height: 1.5;
}

// Page header
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 28px;
  flex-wrap: wrap;
  gap: 12px;
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

.header-actions {
  display: flex;
  gap: 8px;
}

// Providers grid
.providers-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(370px, 1fr));
  gap: 16px;
}

.provider-card {
  @include glass-panel;
  border-radius: $radius-lg;
  padding: 20px;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);

  &:hover {
    border-color: $accent-cyan-dim;
    box-shadow: $shadow-glow-cyan;
  }
}

.provider-header {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 16px;
}

.provider-icon {
  width: 44px;
  height: 44px;
  border-radius: $radius-md;
  background: $accent-cyan-dim;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid $border-default;

  .el-icon { color: $accent-cyan; }
}

.provider-symbol {
  font-size: 16px;
  color: $accent-cyan;
}

.provider-info {
  display: flex;
  align-items: center;
  gap: 8px;
}

.provider-name {
  font-size: $font-size-lg;
  font-weight: 600;
  color: $text-primary;
}

.provider-details {
  margin-bottom: 16px;
}

.detail-row {
  display: flex;
  gap: 8px;
  margin-bottom: 6px;
  font-size: $font-size-xs;
}

.detail-label {
  color: $text-muted;
  width: 60px;
}

.detail-value {
  color: $text-secondary;
}

.mono {
  font-family: $font-display;
}

.provider-models {
  margin-bottom: 16px;
}

.model-tag {
  margin-right: 4px;
}

.more-models {
  font-size: $font-size-xs;
  color: $text-muted;
  font-family: $font-display;
}

.provider-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}

// Model edit
.model-edit-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.model-edit-list {
  max-height: 400px;
  overflow-y: auto;
}

.model-edit-item {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
}

// Test dialog styles
.test-loading {
  text-align: center;
  padding: 32px 0;
  .el-icon {
    font-size: 40px;
    color: $accent-cyan;
  }
  p {
    margin-top: 12px;
    color: $text-muted;
    font-size: $font-size-sm;
  }
}

.test-results {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.test-provider-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-bottom: 8px;
  border-bottom: 1px solid $border-default;
  font-size: $font-size-sm;
  color: $text-primary;
}

.test-summary {
  font-family: $font-display;
  color: $accent-cyan;
  font-weight: 600;
}

.test-model-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 400px;
  overflow-y: auto;
}

.test-model-item {
  padding: 12px;
  border-radius: $radius-md;
  border: 1px solid $border-default;

  &.pass {
    border-color: #67c23a;
    background: rgba(103, 194, 58, 0.05);
  }
  &.fail {
    border-color: #f56c6c;
    background: rgba(245, 108, 108, 0.05);
  }
}

.test-model-header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.test-icon {
  font-size: 18px;
  &.success { color: #67c23a; }
  &.error { color: #f56c6c; }
}

.test-model-name {
  font-size: $font-size-sm;
  font-weight: 600;
  color: $text-primary;
  flex: 1;
}

.test-latency {
  font-family: $font-display;
  font-size: $font-size-xs;
  color: $accent-cyan;
}

.test-model-status {
  margin-top: 6px;
  font-size: $font-size-xs;
  padding-left: 26px;

  &.success { color: #67c23a; }
  &.error { color: #f56c6c; }
}
</style>