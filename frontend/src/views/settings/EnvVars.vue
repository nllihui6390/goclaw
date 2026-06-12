<script setup>
import { ref, computed, onMounted, inject } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Key, Refresh, Search, Plus } from '@element-plus/icons-vue'

const api = inject('api')

const loading = ref(false)
const saving = ref(false)
const envVars = ref([])
const searchKeyword = ref('')
const dialogVisible = ref(false)
const isEditing = ref(false)
const editingEntry = ref({ key: '', value: '', description: '', enabled: true })

const filteredVars = computed(() => {
  if (!searchKeyword.value) return envVars.value
  const kw = searchKeyword.value.toLowerCase()
  return envVars.value.filter(v =>
    v.key.toLowerCase().includes(kw) ||
    v.description?.toLowerCase().includes(kw)
  )
})

const enabledCount = computed(() => envVars.value.filter(v => v.enabled).length)

function maskValue(value) {
  if (!value) return '(未设置)'
  if (value.length < 8) return '****'
  return value.slice(0, 4) + '****' + value.slice(-4)
}

function isSensitiveKey(key) {
  const sensitiveWords = ['KEY', 'SECRET', 'TOKEN', 'PASSWORD', 'CREDENTIAL']
  return sensitiveWords.some(w => key.toUpperCase().includes(w))
}

function isProviderKey(key) {
  return key.startsWith('PROVIDER_')
}

const sourceTagMap = {
  config: { text: '配置', type: 'primary' },
  dotenv: { text: '.env', type: 'success' },
  system: { text: '系统', type: 'info' },
  none: { text: '—', type: 'info' }
}

onMounted(async () => {
  await loadEnvVars()
})

async function loadEnvVars() {
  loading.value = true
  try {
    envVars.value = await api.getEnvVars() || []
  } catch (e) {
    ElMessage.error('加载环境变量失败: ' + e.message)
  }
  loading.value = false
}

function openCreateDialog() {
  isEditing.value = false
  editingEntry.value = { key: '', value: '', description: '', enabled: true }
  dialogVisible.value = true
}

function openEditDialog(entry) {
  isEditing.value = true
  editingEntry.value = { ...entry }
  dialogVisible.value = true
}

async function saveEntry() {
  // 验证 key 格式
  if (!editingEntry.value.key) {
    ElMessage.warning('请输入变量名')
    return
  }
  const keyRegex = /^[A-Z_][A-Z0-9_]*$/
  if (!keyRegex.test(editingEntry.value.key)) {
    ElMessage.warning('变量名只允许大写字母、数字、下划线，首字符必须是字母或下划线')
    return
  }

  saving.value = true
  try {
    if (isEditing.value) {
      await api.updateEnvVar(editingEntry.value)
      ElMessage.success('环境变量已更新')
    } else {
      await api.createEnvVar(editingEntry.value)
      ElMessage.success('环境变量已创建')
    }
    dialogVisible.value = false
    await loadEnvVars()
  } catch (e) {
    ElMessage.error('操作失败: ' + e.message)
  }
  saving.value = false
}

async function toggleEnabled(entry) {
  saving.value = true
  try {
    await api.updateEnvVar({ ...entry })
    await loadEnvVars()
  } catch (e) {
    ElMessage.error('更新失败: ' + e.message)
  }
  saving.value = false
}

async function deleteEntry(key) {
  try {
    await ElMessageBox.confirm(`确定删除环境变量 "${key}"？`, '确认删除', { type: 'warning' })
    saving.value = true
    await api.deleteEnvVar(key)
    ElMessage.success('删除成功')
    await loadEnvVars()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error('删除失败: ' + e.message)
  }
  saving.value = false
}

async function reloadEnvVars() {
  loading.value = true
  try {
    await api.reloadEnvVars()
    ElMessage.success('环境变量已重新加载')
    await loadEnvVars()
  } catch (e) {
    ElMessage.error('重载失败: ' + e.message)
  }
  loading.value = false
}
</script>

<template>
  <div class="page" v-loading="loading">
    <!-- Priority Banner -->
    <div class="priority-banner">
      <div class="banner-left">
        <div class="banner-icon">
          <el-icon :size="18"><Key /></el-icon>
        </div>
        <div class="banner-text">
          <span class="banner-label">环境变量管理</span>
          <span class="banner-detail">
            优先级: 配置文件 > .env 文件 > 系统环境变量 | 已启用 {{ enabledCount }} / {{ envVars.length }} 个变量
          </span>
        </div>
      </div>
      <div class="banner-right">
        <el-button size="small" @click="reloadEnvVars" :loading="loading">
          <el-icon><Refresh /></el-icon>重载
        </el-button>
      </div>
    </div>

    <!-- Action Bar -->
    <div class="action-bar">
      <el-input v-model="searchKeyword" placeholder="搜索变量名或描述" clearable style="width: 220px">
        <template #prefix><el-icon><Search /></el-icon></template>
      </el-input>
      <el-button @click="loadEnvVars" :loading="loading">
        <el-icon><Refresh /></el-icon>刷新
      </el-button>
      <el-button type="primary" @click="openCreateDialog">
        <el-icon><Plus /></el-icon>添加
      </el-button>
    </div>

    <!-- Env Vars Grid -->
    <div class="vars-grid" v-if="filteredVars.length">
      <div v-for="v in filteredVars" :key="v.key" class="var-card">
        <div class="var-header">
          <div class="var-key-icon">
            <el-icon :size="18"><Key /></el-icon>
          </div>
          <div class="var-key-info">
            <span class="var-key-name">{{ v.key }}</span>
          </div>
          <el-switch
            v-model="v.enabled"
            inline-prompt
            active-text="ON"
            inactive-text="OFF"
            @change="toggleEnabled(v)"
          />
        </div>
        <div class="var-tags">
          <el-tag v-if="isProviderKey(v.key)" type="warning" size="small">供应商</el-tag>
          <el-tag v-if="isSensitiveKey(v.key)" type="danger" size="small">敏感</el-tag>
          <el-tag v-if="v.source" :type="sourceTagMap[v.source]?.type || 'info'" size="small">
            {{ sourceTagMap[v.source]?.text || v.source }}
          </el-tag>
        </div>
        <div class="var-details">
          <div class="detail-row">
            <span class="detail-label">值</span>
            <span class="detail-value mono">{{ maskValue(v.value) }}</span>
          </div>
          <div class="detail-row" v-if="v.description">
            <span class="detail-label">描述</span>
            <span class="detail-value">{{ v.description }}</span>
          </div>
        </div>

        <div class="var-actions">
          <el-button size="small" @click="openEditDialog(v)">编辑</el-button>
          <el-button size="small" type="danger" @click="deleteEntry(v.key)">删除</el-button>
        </div>
      </div>
    </div>

    <!-- Empty State -->
    <div v-if="!filteredVars.length && !loading" class="empty-wrapper">
      <el-empty description="暂无环境变量" :image-size="80" />
    </div>

    <!-- Add/Edit Dialog -->
    <el-dialog v-model="dialogVisible" :title="isEditing ? '编辑环境变量' : '添加环境变量'" width="450px">
      <el-form :model="editingEntry" label-width="80px">
        <el-form-item label="变量名" required>
          <el-input
            v-model="editingEntry.key"
            :disabled="isEditing"
            placeholder="如: PROVIDER_DEEPSEEK_API_KEY"
          />
        </el-form-item>
        <el-form-item label="值" required>
          <el-input v-model="editingEntry.value" type="password" show-password placeholder="环境变量的值" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="editingEntry.description" placeholder="可选：变量的用途说明" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="editingEntry.enabled" inline-prompt active-text="ON" inactive-text="OFF" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveEntry" :loading="saving">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style lang="scss" scoped>
@use '@/styles/variables.scss' as *;

.page {
  padding: 32px;
}

// ─────────── Priority Banner ───────────
.priority-banner {
  @include glass-panel;
  border-radius: $radius-lg;
  padding: 16px 20px;
  margin-bottom: 24px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.banner-left {
  display: flex;
  align-items: center;
  gap: 14px;
}

.banner-icon {
  width: 40px;
  height: 40px;
  border-radius: $radius-md;
  display: flex;
  align-items: center;
  justify-content: center;
  background: $accent-cyan-dim;
  border: 1px solid rgba($accent-cyan, 0.2);
  color: $accent-cyan;
}

.banner-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.banner-label {
  font-size: $font-size-lg;
  font-weight: 600;
  color: $text-primary;
}

.banner-detail {
  font-size: $font-size-sm;
  color: $text-muted;
}

.banner-right {
  display: flex;
  gap: 8px;
}

// ─────────── Action Bar ───────────
.action-bar {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-bottom: 20px;
}

// ─────────── Vars Grid ───────────
.vars-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(380px, 1fr));
  gap: 16px;
}

.var-card {
  @include glass-panel;
  border-radius: $radius-lg;
  padding: 20px;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);

  &:hover {
    border-color: $accent-cyan-dim;
    box-shadow: $shadow-glow-cyan;
  }
}

.var-header {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 16px;
}
.var-tags {
  display: flex;
  gap: 8px;
  margin-bottom: 16px;
}

.var-key-icon {
  width: 40px;
  height: 40px;
  border-radius: $radius-md;
  background: $accent-cyan-dim;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid $border-default;

  .el-icon { color: $accent-cyan; }
}

.var-key-info {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
}

.var-key-name {
  font-size: $font-size-lg;
  font-weight: 600;
  color: $text-primary;
  font-family: $font-display;
}

.var-details {
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
  width: 40px;
}

.detail-value {
  color: $text-secondary;
}

.mono {
  font-family: $font-display;
}

.var-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}

// ─────────── Empty Wrapper ───────────
.empty-wrapper {
  @include glass-panel;
  border-radius: $radius-lg;
  padding: 60px 40px;
  text-align: center;
}
</style>