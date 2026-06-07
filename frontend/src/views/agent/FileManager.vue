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

const personaFiles = ['AGENTS.md', 'HEARTBEAT.md', 'MEMORY.md', 'PROFILE.md', 'SOUL.md']


onMounted(loadFiles)
watch(() => agentStore.selectedAgent, () => {
  editFile.value = null
  editContent.value = ''
  loadFiles()
})

async function loadFiles() {
  loading.value = true
  try {
    const data = await api.getAgentFiles(agentStore.selectedAgent)
    files.value = data || []
  } catch (e) {
    files.value = personaFiles.map(name => ({ name, exists: false }))
  }
  loading.value = false
}

async function openFile(file) {
  editFile.value = file
  editContent.value = '加载中...'
  try {
    let content = await api.readAgentFile(agentStore.selectedAgent, file.name)
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
    <!-- Page header -->
    <div class="page-header">
      <div class="header-left">
        <h2>文件管理</h2>
        <el-tag size="small">Agent: {{ agentStore.selectedAgent }}</el-tag>
      </div>
    </div>

    <!-- Split layout -->
    <div class="split-layout">
      <!-- File list -->
      <div class="file-list">
        <div class="list-header">
          <span class="list-title">Persona 文件</span>
          <span class="list-count">{{ files.length }} 个</span>
        </div>

        <div class="file-items">
          <div
            v-for="file in files"
            :key="file.name"
            class="file-item"
            :class="{ active: editFile?.name === file.name }"
            @click="openFile(file)"
          >
            <div class="item-info">
              <span class="item-name">{{ file.name }}</span>
              <span class="item-size" v-if="file.size">{{ formatSize(file.size) }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Editor area -->
      <div class="editor-area">
        <template v-if="editFile">
          <div class="editor-header">
            <div class="editor-title">
              <span class="title-text">{{ editFile.name }}</span>
            </div>
            <el-button type="primary" size="small" @click="saveFile" :loading="saving">
              <el-icon><Folder /></el-icon>保存
            </el-button>
          </div>

          <div class="editor-content">
            <el-input
              v-model="editContent"
              type="textarea"
              class="code-editor"
              placeholder="文件内容..."
            />
          </div>
        </template>

        <div v-else class="editor-empty">
          <span class="empty-text">选择左侧文件开始编辑</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
@use '@/styles/variables.scss' as *;

.page {
  padding: 32px;
  display: flex;
  flex-direction: column;
  height: calc(100vh - $header-height);
  box-sizing: border-box;
}

.page-header {
  margin-bottom: 20px;
  flex-shrink: 0;
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

.split-layout {
  flex: 1;
  display: flex;
  gap: 20px;
  min-height: 0;
}

// File list
.file-list {
  width: 240px;
  flex-shrink: 0;
  @include glass-panel;
  border-radius: $radius-lg;
  padding: 16px;
  display: flex;
  flex-direction: column;
}

.list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  padding-bottom: 12px;
  border-bottom: 1px solid $border-subtle;
}

.list-title {
  font-size: $font-size-sm;
  font-weight: 600;
  color: $text-primary;
}

.list-count {
  font-size: $font-size-xs;
  color: $text-muted;
  font-family: $font-display;
}

.file-items {
  flex: 1;
}

.file-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 14px;
  margin-bottom: 6px;
  border-radius: $radius-md;
  cursor: pointer;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  background: $bg-elevated;
  border: 1px solid $border-default;
  position: relative;
  overflow: hidden;

  // 左侧指示条动画
  &::before {
    content: '';
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    width: 3px;
    background: $accent-cyan;
    transform: scaleY(0);
    transition: transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
    border-radius: 0 2px 2px 0;
  }

  &:hover {
    border-color: rgba(0, 212, 255, 0.3);
    background: $accent-cyan-dim;

    &::before {
      transform: scaleY(1);
    }

    .item-name {
      color: $accent-cyan;
    }
  }

  // 桌面端 hover 右移动画
  @media (min-width: 769px) {
    &:hover {
      transform: translateX(4px);
    }
  }

  &.active {
    background: $accent-cyan-dim;
    border-color: rgba(0, 212, 255, 0.3);

    &::before {
      transform: scaleY(1);
      box-shadow: 0 0 8px rgba(0, 212, 255, 0.4);
    }

    .item-name {
      color: $accent-cyan;
      font-weight: 600;
    }
  }
}

.item-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.item-name {
  font-size: $font-size-sm;
  font-weight: 500;
  color: $text-primary;
  font-family: $font-display;
}

.item-size {
  font-size: $font-size-xs;
  color: $text-muted;
}

// Editor area
.editor-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  @include glass-panel;
  border-radius: $radius-lg;
  padding: 16px;
}

.editor-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
  flex-shrink: 0;
}

.editor-title {
  display: flex;
  align-items: center;
  gap: 8px;
}

.title-text {
  font-size: $font-size-lg;
  font-weight: 600;
  color: $text-primary;
  font-family: $font-display;
}

.editor-content {
  flex: 1;
  min-height: 0;
}

.code-editor {
  height: 100%;

  :deep(textarea) {
    height: 100% !important;
    min-height: 300px;
    font-family: $font-display;
    font-size: $font-size-sm;
    line-height: 1.7;
    background: $bg-deep;
    border: 1px solid $border-subtle;
    border-radius: $radius-md;
    padding: 16px;
    color: $text-primary;
    resize: none;

    &::placeholder {
      color: $text-muted;
    }
  }
}

.editor-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
}

.empty-text {
  font-size: $font-size-sm;
  color: $text-muted;
}

// Mobile layout
@media (max-width: 768px) {
  .page { padding: 16px; }
  .split-layout { flex-direction: column; }
  .file-list {
    width: 100%;
    flex-shrink: 0;
    max-height: none;
  }
  .file-items {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    overflow-y: visible;
  }
  .file-item {
    margin-bottom: 0;
    padding: 8px 12px;
    flex: 0 0 auto;
    min-width: 120px;
  }
  .editor-area { min-height: 300px; }
}
</style>