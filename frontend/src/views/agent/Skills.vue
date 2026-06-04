<script setup>
import { ref, inject, onMounted, watch, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { useAgentStore } from '@/stores/agent'

const api = inject('api')
const agentStore = useAgentStore()

const loading = ref(false)
const saving = ref(false)
const scanning = ref(false)

// 当前 agent 已启用的技能
const enabledSkills = ref([])
const enabledNames = ref([])

// 技能池对话框
const poolDialogVisible = ref(false)
const poolSkills = ref([])
const selectedFromPool = ref([])

// 技能目录
const skillDir = ref('')

onMounted(loadEnabledSkills)
watch(() => agentStore.selectedAgent, loadEnabledSkills)

// 加载当前 agent 已启用的技能
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

// 打开技能池对话框
async function openPoolDialog() {
  try {
    const res = await api.getSkillPool()
    poolSkills.value = res.skills || []
    // 默认勾选已启用的
    selectedFromPool.value = [...enabledNames.value]
    poolDialogVisible.value = true
  } catch (e) {
    ElMessage.error('加载技能池失败: ' + e.message)
  }
}

// 确认从技能池载入
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

// 扫描技能目录
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

// 移除单个技能
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

// 切换技能选中状态
function toggleSkill(skill) {
  const skillName = skill.name
  const idx = selectedFromPool.value.indexOf(skillName)
  if (idx >= 0) {
    selectedFromPool.value.splice(idx, 1)
  } else {
    selectedFromPool.value.push(skillName)
  }
}

// 判断技能池中的技能是否已选中
function isSkillSelected(skill) {
  return selectedFromPool.value.includes(skill.name)
}
</script>

<template>
  <div class="page" v-loading="loading">
    <div class="page-header">
      <div class="header-left">
        <h2>技能管理</h2>
        <span class="skill-info">
          当前 Agent: <el-tag size="small">{{ agentStore.selectedAgent }}</el-tag>
          · 已启用 {{ enabledSkills.length }} 个技能
        </span>
      </div>
      <div class="header-actions">
        <el-button type="primary" @click="openPoolDialog">
          <el-icon><Plus /></el-icon>从技能池载入
        </el-button>
        <el-button @click="scanSkills" :loading="scanning">
          <el-icon><Search /></el-icon>扫描
        </el-button>
        <el-button @click="loadEnabledSkills">
          <el-icon><Refresh /></el-icon>刷新
        </el-button>
      </div>
    </div>

    <!-- 已启用技能列表 -->
    <div class="skills-grid" v-if="enabledSkills.length">
      <el-card v-for="skill in enabledSkills" :key="skill.name" class="skill-card">
        <template #header>
          <div class="card-header">
            <div class="skill-title">
              <span class="skill-emoji">{{ skill.emoji || '🔧' }}</span>
              <span class="skill-name">{{ skill.name }}</span>
            </div>
            <el-tag v-if="skill.has_scripts" size="small" type="warning">含脚本</el-tag>
          </div>
        </template>

        <div class="skill-body">
          <p class="skill-desc">{{ skill.description || '无描述' }}</p>
          <div class="skill-meta">
            <el-icon><Folder /></el-icon>
            <code>{{ skill.folder }}</code>
            <el-tag v-if="skill.has_scripts" size="small" type="warning" style="margin-left: auto">脚本</el-tag>
          </div>
        </div>

        <div class="skill-footer">
          <el-button type="danger" link size="small" @click="removeSkill(skill.name)">移除</el-button>
        </div>
      </el-card>
    </div>

    <el-empty v-else-if="!loading" description="暂无已启用的技能">
      <template #extra>
        <p class="empty-hint">点击右上角"从技能池载入"添加技能</p>
      </template>
    </el-empty>

    <!-- 技能池对话框 -->
    <el-dialog v-model="poolDialogVisible" title="从技能池载入技能" width="720px">
      <div class="pool-header">
        <span>全量技能池（共 {{ poolSkills.length }} 个）</span>
        <span class="pool-hint">点击卡片选中/取消，确认后生效</span>
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
              <el-icon v-if="isSkillSelected(skill)" :size="18" color="#409eff"><CircleCheckFilled /></el-icon>
              <el-icon v-else :size="18" color="#dcdfe6"><CircleCheck /></el-icon>
            </div>
          </div>
          <p class="pool-card-desc">{{ skill.description || '无描述' }}</p>
          <div class="pool-card-meta">
            <code>{{ skill.folder }}</code>
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
.skills-grid { display: grid;grid-template-columns: repeat(auto-fill, minmax(400px, 1fr));gap: 16px;}
.skill-card { transition: all .2s; }
.skill-card :deep(.el-card__body) { display: flex; flex-direction: column; }
.skill-card:hover { box-shadow: 0 2px 12px rgba(0,0,0,.1); }
.card-header { display: flex;justify-content: space-between;align-items: center;}
.skill-title { display: flex; align-items: center; gap: 8px; }
.skill-emoji { font-size: 20px; }
.skill-name { font-weight: 600; font-size: 15px; }
.skill-body { display: flex; flex-direction: column; gap: 8px; flex: 1;padding-bottom: 8px;}
.skill-desc { margin: 0; color: #606266; font-size: 14px; line-height: 1.4; display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
.skill-meta { display: flex; align-items: center; gap: 4px; color: #909399; font-size: 13px; }
.skill-meta code { background: #f5f7fa; padding: 1px 6px; border-radius: 3px; font-size: 12px; }
.skill-footer { margin-top: auto; padding-top: 10px; border-top: 1px solid #ebeef5; text-align: right; }
.empty-hint { color: #909399; font-size: 13px; text-align: center; }

/* 技能池对话框 */
.pool-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.pool-hint { font-size: 12px; color: #909399; }
.pool-grid {
  display: grid;grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 12px;max-height: 400px;overflow-y: auto;padding: 4px;
}
.pool-card {
  padding: 14px;border-radius: 10px;border: 2px solid #e4e7ed;
  background: #fff;cursor: pointer;transition: all .2s;height: 130px;
}
.pool-card:hover { border-color: #c0c4cc; box-shadow: 0 2px 8px rgba(0,0,0,.08); }
.pool-card.selected { border-color: #409eff; background: #ecf5ff; }
.pool-card-top { display: flex; align-items: center; gap: 6px; margin-bottom: 8px; }
.pool-emoji { font-size: 20px; }
.pool-card-name { font-weight: 600; font-size: 14px; flex: 1; }
.pool-card-check { margin-left: auto; }
.pool-card-desc { margin: 0 0 8px 0; font-size: 12px; color: #606266; line-height: 1.4; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.pool-card-meta { font-size: 11px; color: #909399; }
.pool-card-meta code { background: #f5f7fa; padding: 1px 4px; border-radius: 3px; }
</style>