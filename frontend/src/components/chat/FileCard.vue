<script setup>
import { inject } from 'vue'

const api = inject('api')

const props = defineProps({
  fileType: String,
  path: String,
  filename: String,
  size: Number
})

function formatSize(bytes) {
  if (!bytes || bytes === 0) return ''
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

function download() {
  if (props.fileType === 'url') {
    window.open(props.path, '_blank')
    return
  }
  if (window._wails) {
    api.downloadFile(props.path, props.filename)
  } else {
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
        <span v-if="size" class="file-size">{{ formatSize(size) }}</span>
        <span v-if="fileType === 'url'" class="file-type-tag url">链接</span>
        <span v-else class="file-type-tag local">本地</span>
      </div>
    </div>
    <div class="file-action">
      <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
        <path d="M8 1v9.5M4.5 7L8 10.5L11.5 7M2 12v2h12v-2"/>
      </svg>
    </div>
  </div>
</template>

<style lang="scss" scoped>
@use '@/styles/variables.scss' as *;

.file-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 18px;
  background: $bg-elevated;
  border: 1px solid $border-default;
  border-radius: $radius-md;
  cursor: pointer;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);

  &:hover {
    background: $bg-surface;
    border-color: $accent-cyan;
    box-shadow: $shadow-glow-cyan;
  }
}

.file-icon {
  font-size: 24px;
  width: 36px;
  text-align: center;
}

.file-info {
  flex: 1;
  min-width: 0;
}

.file-name {
  font-size: $font-size-base;
  font-weight: 500;
  color: $text-primary;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-meta {
  font-size: $font-size-xs;
  display: flex;
  gap: 8px;
  align-items: center;
  margin-top: 4px;
}

.file-size {
  color: $text-muted;
  font-family: $font-display;
}

.file-type-tag {
  padding: 2px 6px;
  border-radius: $radius-sm;
  font-size: $font-size-xs;
  font-family: $font-display;

  &.url {
    background: $accent-cyan-dim;
    color: $accent-cyan;
  }

  &.local {
    background: $bg-elevated;
    color: $text-muted;
    border: 1px solid $border-default;
  }
}

.file-action {
  color: $text-muted;
  display: flex;
  align-items: center;
  transition: color 0.2s;
}

.file-card:hover .file-action {
  color: $accent-cyan;
}
</style>