<script setup>
import { ref, inject, onMounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useAgentStore } from '@/stores/agent'

const api = inject('api')
const agentStore = useAgentStore()

const files = ref([])
const loading = ref(false)
const saving = ref(false)
const editFile = ref(null)
const editContent = ref('')

const personaFiles = ['AGENTS.md', 'HEARTBEAT.md', 'MEMORY.md', 'PROFILE.md', 'SOUL.md', 'BOOTSTRAP.md']

onMounted(loadFiles)
watch(() => agentStore.selectedAgent, loadFiles)

async function loadFiles() {
  loading.value = true
  try {
    const data = await api.getAgentFiles(agentStore.selectedAgent)
    files.value = data || []
  } catch (e) {
    // 降级：使用静态文件名列表
    files.value = personaFiles.map(name => ({ name, exists: false }))
  }
  loading.value = false
}

async function openFile(file) {
  editFile.value = file
  editContent.value = '加载中...'
  try {
    let content = await api.readAgentFile(agentStore.selectedAgent, file.name)
    // 兼容 Wails 返回 JSON 字符串的情况（外层引号包裹）
    if (typeof content === 'string') {
      try {
        const parsed = JSON.parse(content)
        if (typeof parsed === 'string') content = parsed
      } catch {}
    }
    editContent.value = content || ''
  } catch (e) {
    editContent.value = '加载失败: ' + e.message
  }
}

async function saveFile() {
  saving.value = true
  try {
    await api.writeAgentFile(agentStore.selectedAgent, editFile.value.name, editContent.value)
    ElMessage.success(`${editFile.value.name} 保存成功`)
    await loadFiles()
  } catch (e) {
    ElMessage.error('保存失败: ' + e.message)
  }
  saving.value = false
}

function formatSize(bytes) {
  if (!bytes) return ''
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}
</script>

<template>
  <div class="page" v-loading="loading">
    <div class="page-header">
      <div class="header-left">
        <h2>文件管理</h2>
        <el-tag size="small">Agent: {{ agentStore.selectedAgent }}</el-tag>
      </div>
    </div>

    <div class="split-layout">
      <!-- 左侧文件列表 -->
      <div class="file-list">
        <div
          v-for="file in files"
          :key="file.name"
          class="file-item"
          :class="{ active: editFile?.name === file.name }"
          @click="openFile(file)"
        >
          <el-icon :size="18"><Document /></el-icon>
          <div class="item-info">
            <span class="item-name">{{ file.name }}</span>
            <span class="item-size" v-if="file.size">{{ formatSize(file.size) }}</span>
          </div>
        </div>
        <el-empty v-if="!files.length" description="暂无文件" :image-size="40" />
      </div>

      <!-- 右侧编辑区 -->
      <div class="editor-area">
        <template v-if="editFile">
          <div class="editor-header">
            <span class="editor-title">{{ editFile.name }}</span>
            <el-button type="primary" size="small" @click="saveFile" :loading="saving">保存</el-button>
          </div>
          <el-input
            v-model="editContent"
            type="textarea"
            class="editor-content"
            placeholder="选择左侧文件开始编辑..."
          />
        </template>
        <div v-else class="editor-empty">
          <span>← 选择左侧文件开始编辑</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page { padding: 24px; display: flex; flex-direction: column; height: calc(100vh - 48px); box-sizing: border-box; }
.page-header { margin-bottom: 16px; flex-shrink: 0; }
.header-left { display: flex; align-items: center; gap: 12px; }
.header-left h2 { margin: 0; font-weight: 500; }

.split-layout {
  flex: 1;
  display: flex;
  gap: 16px;
  min-height: 0;
}

/* 左侧文件列表 */
.file-list {
  width: 220px;
  flex-shrink: 0;
  border-right: 1px solid #ebeef5;
  overflow-y: auto;
  padding-right: 8px;
}

/* 手机端：上下布局 */
@media (max-width: 768px) {
  .page { padding: 12px; }
  .split-layout { flex-direction: column; }
  .file-list {
    width: 100%;
    flex-shrink: 0;
    border-right: none;
    border-bottom: 1px solid #ebeef5;
    overflow-x: auto;
    overflow-y: hidden;
    padding-right: 0;
    padding-bottom: 8px;
    display: flex;
    gap: 8px;
  }
  .file-item {
    flex-shrink: 0;
    white-space: nowrap;
    padding: 8px 14px;
  }
  .item-info { flex-direction: row; gap: 6px; align-items: baseline; }
  .editor-area { min-height: 300px; }
}
.file-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 6px;
  cursor: pointer;
  transition: all .15s;
  color: #606266;
}
.file-item:hover { background: #f5f7fa; }
.file-item.active { background: #ecf5ff; color: #409eff; }
.item-info { display: flex; flex-direction: column; gap: 1px; min-width: 0; }
.item-name { font-size: 13px; font-weight: 500; }
.item-size { font-size: 11px; color: #bbb; }

/* 右侧编辑区 */
.editor-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.editor-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
  flex-shrink: 0;
}
.editor-title { font-weight: 600; font-size: 15px; }
.editor-content {
  flex: 1;
  font-family: 'Consolas', 'Monaco', monospace;
  font-size: 13px;
  line-height: 1.6;
}
.editor-content :deep(textarea) {
  height: 100% !important;
  min-height: 300px;
}
.editor-empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #bbb;
  font-size: 14px;
}

/* 滚动条优化 */
.file-list::-webkit-scrollbar,
.editor-content :deep(textarea)::-webkit-scrollbar {
  width: 4px;
  height: 4px;
}
.file-list::-webkit-scrollbar-thumb,
.editor-content :deep(textarea)::-webkit-scrollbar-thumb {
  background: #d0d5dd;
  border-radius: 2px;
}
.file-list::-webkit-scrollbar-track,
.editor-content :deep(textarea)::-webkit-scrollbar-track {
  background: transparent;
}
</style>
