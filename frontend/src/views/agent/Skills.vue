<script setup>
import { ref, inject, onMounted, watch, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { useAgentStore } from '@/stores/agent'

const api = inject('api')
const agentStore = useAgentStore()

const loading = ref(false)
const saving = ref(false)
const scanning = ref(false)
const uploading = ref(false)
const fileInput = ref(null)
const enabledSkills = ref([])
const enabledNames = ref([])
const poolDialogVisible = ref(false)
const poolSkills = ref([])
const selectedFromPool = ref([])
const skillDir = ref('')

onMounted(loadEnabledSkills)
watch(() => agentStore.selectedAgent, loadEnabledSkills)

async function loadEnabledSkills() {
  loading.value = true
  try {
    const res = await api.getEnabledSkills(agentStore.selectedAgent)
    enabledSkills.value = res.skills || []
    enabledNames.value = res.enabled || []
    skillDir.value = res.skill_dir || ''
  } catch (e) {
    ElMessage.error('加载失败: ' + e.message)
  }
  loading.value = false
}

async function openPoolDialog() {
  try {
    const res = await api.getSkillPool()
    poolSkills.value = res.skills || []
    selectedFromPool.value = [...enabledNames.value]
    poolDialogVisible.value = true
  } catch (e) {
    ElMessage.error('加载技能池失败: ' + e.message)
  }
}

async function confirmFromPool() {
  saving.value = true
  try {
    await api.setEnabledSkills(agentStore.selectedAgent, selectedFromPool.value)
    ElMessage.success('技能配置已更新')
    poolDialogVisible.value = false
    await loadEnabledSkills()
  } catch (e) {
    ElMessage.error('保存失败: ' + e.message)
  }
  saving.value = false
}

async function scanSkills() {
  scanning.value = true
  try {
    const res = await api.scanSkills()
    if (res.error) {
      ElMessage.error('扫描失败: ' + res.error)
    } else {
      ElMessage.success(res.message || `扫描完成，发现 ${res.total} 个技能`)
    }
  } catch (e) {
    ElMessage.error('扫描失败: ' + e.message)
  }
  scanning.value = false
}

async function removeSkill(skillName) {
  const newList = enabledNames.value.filter(n => n !== skillName)
  saving.value = true
  try {
    await api.setEnabledSkills(agentStore.selectedAgent, newList)
    ElMessage.success(`已移除技能: ${skillName}`)
    await loadEnabledSkills()
  } catch (e) {
    ElMessage.error('移除失败: ' + e.message)
  }
  saving.value = false
}

function toggleSkill(skill) {
  const skillName = skill.name
  const idx = selectedFromPool.value.indexOf(skillName)
  if (idx >= 0) {
    selectedFromPool.value.splice(idx, 1)
  } else {
    selectedFromPool.value.push(skillName)
  }
}

function isSkillSelected(skill) {
  return selectedFromPool.value.includes(skill.name)
}

async function uploadSkill() {
  fileInput.value?.click()
}

async function onFileSelected(e) {
  const file = e.target.files?.[0]
  if (!file) return
  e.target.value = ''

  if (!file.name.endsWith('.zip')) {
    ElMessage.warning('只支持 .zip 文件')
    return
  }

  uploading.value = true
  try {
    const res = await api.uploadSkill(file)
    if (res.error) {
      ElMessage.error(res.error)
    } else {
      ElMessage.success(res.message || `导入成功，${res.total} 个技能`)
      await scanSkills()
      await loadEnabledSkills()
    }
  } catch (e) {
    ElMessage.error('上传失败: ' + e.message)
  }
  uploading.value = false
}
</script>

<template>
  <div class="page" v-loading="loading">
    <!-- Page header -->
    <div class="page-header">
      <div class="header-left">
        <h2>技能管理</h2>
        <div class="header-info">
          <el-tag size="small">{{ agentStore.selectedAgent }}</el-tag>
          <span class="skill-count">{{ enabledSkills.length }} 个技能</span>
        </div>
      </div>
      <div class="header-actions">
        <input ref="fileInput" type="file" accept=".zip" hidden @change="onFileSelected" />
        <el-button type="primary" @click="openPoolDialog">
          <el-icon><Plus /></el-icon>从技能池载入
        </el-button>
        <el-button @click="uploadSkill" :loading="uploading">
          <el-icon><Upload /></el-icon>上传技能
        </el-button>
        <el-button @click="scanSkills" :loading="scanning">
          <el-icon><Search /></el-icon>扫描
        </el-button>
        <el-button @click="loadEnabledSkills">
          <el-icon><Refresh /></el-icon>刷新
        </el-button>
      </div>
    </div>

    <!-- Skills grid -->
    <div class="skills-grid" v-if="enabledSkills.length">
      <div v-for="skill in enabledSkills" :key="skill.name" class="skill-card">
        <div class="skill-status-badge"><span class="status-dot"></span>已启用</div>
        <div class="skill-header">
          <div class="skill-emoji">{{ skill.emoji || '🔧' }}</div>
          <div class="skill-title">
            <span class="skill-name">{{ skill.name }}</span>
            <div class="skill-tags">
              <el-tag v-if="skill.version" size="small" type="info">v{{ skill.version }}</el-tag>
              <el-tag v-if="skill.has_scripts" size="small" type="warning">脚本</el-tag>
            </div>
          </div>
        </div>

        <div class="skill-body">
          <p class="skill-desc">{{ skill.description || '无描述' }}</p>
          <div class="skill-meta">
            <el-icon><Folder /></el-icon>
            <code>{{ skill.folder }}</code>
          </div>
        </div>

        <div class="skill-footer">
          <el-button type="danger" size="small" @click="removeSkill(skill.name)">移除</el-button>
        </div>
      </div>
    </div>

    <el-empty v-else-if="!loading" description="暂无已启用的技能">
      <template #extra>
        <p class="empty-hint">点击"从技能池载入"添加技能</p>
      </template>
    </el-empty>

    <!-- Pool dialog -->
    <el-dialog v-model="poolDialogVisible" title="从技能池载入技能" width="720px">
      <div class="pool-header">
        <span class="pool-title">全量技能池（共 {{ poolSkills.length }} 个）</span>
        <span class="pool-hint">点击卡片选中/取消</span>
      </div>

      <div class="pool-grid" v-if="poolSkills.length">
        <div
          v-for="skill in poolSkills"
          :key="skill.name"
          class="pool-card"
          :class="{ selected: isSkillSelected(skill) }"
          @click="toggleSkill(skill)"
        >
          <div class="pool-card-top">
            <span class="pool-emoji">{{ skill.emoji || '🔧' }}</span>
            <span class="pool-card-name">{{ skill.name }}</span>
            <el-tag v-if="skill.has_scripts" size="small" type="warning">脚本</el-tag>
            <div class="pool-card-check">
              <el-icon v-if="isSkillSelected(skill)" :size="18" color="#00d4ff"><CircleCheckFilled /></el-icon>
              <el-icon v-else :size="18" color="#6b7280"><CircleCheck /></el-icon>
            </div>
          </div>
          <p class="pool-card-desc">{{ skill.description || '无描述' }}</p>
          <div class="pool-card-meta">
            <code>{{ skill.folder }}</code>
            <el-tag v-if="skill.version" size="small" type="info">v{{ skill.version }}</el-tag>
          </div>
        </div>
      </div>

      <el-empty v-else description="技能池为空，请先点击扫描按钮" :image-size="60" />

      <template #footer>
        <el-button @click="poolDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="confirmFromPool" :loading="saving">
          确认（选中 {{ selectedFromPool.length }} 个）
        </el-button>
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
  flex-wrap: wrap;
  gap: 16px;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}


.header-left h2 {
  margin: 0;
  font-size: $font-size-xl;
  font-weight: 600;
  color: $text-primary;
}

.header-info {
  display: flex;
  align-items: center;
  gap: 8px;
}

.skill-count {
  font-size: $font-size-sm;
  color: $text-muted;
  font-family: $font-display;
}

.header-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.skills-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(380px, 1fr));
  gap: 16px;
}

.skill-card {
  @include glass-panel;
  border-radius: $radius-lg;
  padding: 20px;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  position: relative;
  border-color: rgba(76, 175, 80, 0.25);

  &:hover {
    border-color: rgba(76, 175, 80, 0.4);
    box-shadow: 0 0 12px rgba(76, 175, 80, 0.15);
  }
}

.skill-status-badge {
  position: absolute;
  top: 12px;
  right: 12px;
  display: flex;
  align-items: center;
  gap: 5px;
  font-size: $font-size-xs;
  font-weight: 600;
  color: #4caf50;
  background: rgba(76, 175, 80, 0.1);
  padding: 3px 10px;
  border-radius: $radius-sm;
  border: 1px solid rgba(76, 175, 80, 0.3);

  .status-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: #4caf50;
    animation: pulse-dot 2s ease-in-out infinite;
  }
}

@keyframes pulse-dot {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

.skill-header {
  display: flex;
  align-items: flex-start;
  gap: 14px;
  margin-bottom: 14px;
}

.skill-emoji {
  font-size: 28px;
  width: 44px;
  height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: $bg-elevated;
  border-radius: $radius-md;
  border: 1px solid $border-default;
}

.skill-title {
  flex: 1;
  min-width: 0;
}

.skill-name {
  font-size: $font-size-lg;
  font-weight: 600;
  color: $text-primary;
  display: block;
  margin-bottom: 6px;
}

.skill-tags {
  display: flex;
  gap: 6px;
}

.skill-body {
  margin-bottom: 16px;
}

.skill-desc {
  margin: 0 0 10px 0;
  color: $text-secondary;
  font-size: $font-size-sm;
  line-height: 1.5;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.skill-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  color: $text-muted;
  font-size: $font-size-xs;

  code {
    background: $bg-elevated;
    padding: 2px 8px;
    border-radius: $radius-sm;
    font-family: $font-display;
    border: 1px solid $border-default;
  }
}

.skill-footer {
  position: absolute;
  bottom: 16px;
  right: 16px;
  opacity: 0;
  transition: opacity 0.2s;
}

.skill-card:hover .skill-footer {
  opacity: 1;
}

.empty-hint {
  color: $text-muted;
  font-size: $font-size-sm;
  text-align: center;
}

// Pool dialog
.pool-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.pool-title {
  font-weight: 500;
  color: $text-primary;
}

.pool-hint {
  font-size: $font-size-xs;
  color: $text-muted;
}

.pool-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 12px;
  max-height: 400px;
  overflow-y: auto;
  padding: 4px;
}

.pool-card {
  padding: 16px;
  border-radius: $radius-md;
  border: 2px solid $border-default;
  background: $bg-elevated;
  cursor: pointer;
  transition: all 0.2s;
  height: 130px;

  &:hover {
    border-color: $border-default;
    box-shadow: $shadow-soft;
  }

  &.selected {
    border-color: $accent-cyan;
    background: $accent-cyan-dim;
  }
}

.pool-card-top {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.pool-emoji {
  font-size: 18px;
}

.pool-card-name {
  font-weight: 600;
  font-size: $font-size-sm;
  color: $text-primary;
  flex: 1;
}

.pool-card-check {
  margin-left: auto;
}

.pool-card-desc {
  margin: 0 0 8px 0;
  font-size: $font-size-xs;
  color: $text-secondary;
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.pool-card-meta {
  font-size: $font-size-xs;
  color: $text-muted;
  display: flex;
  align-items: center;
  gap: 6px;

  code {
    background: $bg-surface;
    padding: 2px 6px;
    border-radius: $radius-sm;
    font-family: $font-display;
  }
}
</style>