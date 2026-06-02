<script setup>
import { ref, inject, onMounted } from 'vue'
import { ElMessage } from 'element-plus'

const api = inject('api')

const loading = ref(false)
const skillData = ref({ enabled: false, skill_dir: 'skills', skills: [], total: 0 })

onMounted(loadSkills)

async function loadSkills() {
  loading.value = true
  try {
    skillData.value = await api.getSkills() || {}
  } catch (e) {
    ElMessage.error('加载失败: ' + e.message)
  }
  loading.value = false
}
</script>

<template>
  <div class="page" v-loading="loading">
    <div class="page-header">
      <div class="header-left">
        <h2>技能管理</h2>
        <span class="skill-dir">共 {{ skillData.total || 0 }} 个技能 · 目录: {{ skillData.skill_dir }}</span>
      </div>
      <el-button @click="loadSkills">
        <el-icon><Refresh /></el-icon>刷新
      </el-button>
    </div>

    <div class="skills-grid" v-if="skillData.skills?.length">
      <el-card v-for="skill in skillData.skills" :key="skill.name" class="skill-card">
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
            <span class="meta-item">
              <el-icon><Folder /></el-icon>
              <code>{{ skill.folder }}</code>
            </span>
          </div>

          <div v-if="skill.sections?.length" class="skill-sections">
            <el-tag v-for="sec in skill.sections" :key="sec" size="small" effect="plain" class="section-tag">
              {{ sec }}
            </el-tag>
          </div>

          <div v-if="skill.scripts?.length" class="skill-scripts">
            <span class="label">脚本:</span>
            <el-tag v-for="s in skill.scripts" :key="s" size="small" type="info" class="script-tag">
              {{ s }}
            </el-tag>
          </div>
        </div>
      </el-card>
    </div>

    <el-empty v-else-if="!loading" description="暂无技能">
      <template #extra>
        <p class="empty-hint">在 {{ skillData.skill_dir }}/ 目录下创建 SKILL.md 文件即可添加技能</p>
      </template>
    </el-empty>
  </div>
</template>

<style scoped>
.page {
  padding: 24px;
}
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}
.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}
.header-left h2 {
  margin: 0;
  font-weight: 500;
}
.skill-dir {
  color: #909399;
  font-size: 13px;
}
.skills-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
  gap: 16px;
}
.skill-card {
  transition: all .2s;
}
.skill-card:hover {
  box-shadow: 0 2px 12px rgba(0,0,0,.1);
}
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.skill-title {
  display: flex;
  align-items: center;
  gap: 8px;
}
.skill-emoji {
  font-size: 20px;
}
.skill-name {
  font-weight: 600;
  font-size: 15px;
}
.skill-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.skill-desc {
  margin: 0;
  color: #606266;
  font-size: 14px;
  line-height: 1.5;
}
.skill-meta {
  display: flex;
  align-items: center;
  gap: 4px;
  color: #909399;
  font-size: 13px;
}
.meta-item {
  display: flex;
  align-items: center;
  gap: 4px;
}
.meta-item code {
  background: #f5f7fa;
  padding: 1px 6px;
  border-radius: 3px;
  font-size: 12px;
}
.skill-sections {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.section-tag {
  font-size: 11px;
}
.skill-scripts {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
}
.skill-scripts .label {
  font-size: 12px;
  color: #909399;
}
.script-tag {
  font-size: 11px;
}
.empty-hint {
  color: #909399;
  font-size: 13px;
  text-align: center;
}
</style>
