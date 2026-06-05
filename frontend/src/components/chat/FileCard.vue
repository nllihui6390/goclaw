<script setup>
import { inject } from 'vue'

const api = inject('api')

const props = defineProps({
  fileType: String,   // 'file' 或 'url'
  path: String,       // 文件路径或 URL
  filename: String,   // 显示文件名
  size: Number        // 文件大小（字节）
})

function formatSize(bytes) {
  if (!bytes || bytes === 0) return ''
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

function download() {
  if (props.fileType === 'url') {
    // URL 类型直接打开链接
    window.open(props.path, '_blank')
    return
  }
  // 本地文件：HTTP 模式用下载 API，Wails 模式调用后端方法
  if (window._wails) {
    // Wails 桌面模式
    api.downloadFile(props.path, props.filename)
  } else {
    // HTTP 模式：跳转到下载 API
    const downloadUrl = `/api/v1/files/download?path=${encodeURIComponent(props.path)}&filename=${encodeURIComponent(props.filename)}`
    window.open(downloadUrl, '_blank')
  }
}

function fileIcon() {
  if (!props.filename) return '📄'
  const ext = props.filename.split('.').pop().toLowerCase()
  const iconMap = {
    pdf: '📕', doc: '📘', docx: '📘', xls: '📗', xlsx: '📗',
    csv: '📊', txt: '📄', md: '📝', json: '📋', xml: '📋',
    png: '🖼️', jpg: '🖼️', jpeg: '🖼️', gif: '🖼️',
    zip: '📦', tar: '📦', gz: '📦', py: '🐍', js: '⚡',
    go: '🔵', sh: '⚙️',
  }
  return iconMap[ext] || '📄'
}
</script>

<template>
  <div class="file-card" @click="download">
    <div class="file-icon">{{ fileIcon() }}</div>
    <div class="file-info">
      <div class="file-name">{{ filename || '未知文件' }}</div>
      <div class="file-meta">
        <span v-if="size">{{ formatSize(size) }}</span>
        <span v-if="fileType === 'url'" class="file-type-tag">链接</span>
        <span v-else class="file-type-tag">本地</span>
      </div>
    </div>
    <div class="file-action">
      <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
        <path d="M8 1v9.5M4.5 7L8 10.5L11.5 7M2 12v2h12v-2" stroke="currentColor" stroke-width="1.5" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
      </svg>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.file-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  background: #f5f7fa;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  cursor: pointer;
  transition: all .2s;
  &:hover {
    background: #ecf5ff;
    border-color: #c6e2ff;
  }
}
.file-icon {
  font-size: 24px;
  width: 32px;
  text-align: center;
}
.file-info {
  flex: 1;
  min-width: 0;
}
.file-name {
  font-size: 14px;
  font-weight: 500;
  color: #303133;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.file-meta {
  font-size: 12px;
  color: #909399;
  display: flex;
  gap: 8px;
  align-items: center;
}
.file-type-tag {
  background: #e4e7ed;
  padding: 1px 4px;
  border-radius: 3px;
  font-size: 11px;
}
.file-action {
  color: #909399;
  display: flex;
  align-items: center;
  transition: color .2s;
}
.file-card:hover .file-action {
  color: #409eff;
}
</style>