<script setup>
import { computed } from 'vue'
import { marked } from 'marked'
import FileCard from '@/components/chat/FileCard.vue'

marked.setOptions({ breaks: true, gfm: true })

const props = defineProps({
  role: String,
  content: [Array, String], // ContentBlocks 数组或纯文本字符串
  files: Array // SSE file 事件推送的文件信息（旧格式兼容）
})

// getDisplayUrl 将路径转换为前端可访问的 URL
// file:// URL → /api/v1/files/preview?path=xxx（后端负责解码和路径转换）
// http(s):// URL → 保持不变
// 本地路径 → /api/v1/files/preview?path=xxx（后端负责安全检查）
function getDisplayUrl(url) {
  if (!url) return ''
  // 远程 URL 直接使用
  if (url.startsWith('http://') || url.startsWith('https://') || url.startsWith('data:')) return url
  // file:// URL 和本地路径 → 传给后端预览端点（后端处理 URL 解码 + file:// → 本地路径转换）
  return `/api/v1/files/preview?path=${encodeURIComponent(url)}`
}

// 判断是否为图片类型的文件信息
function isImage(info) {
  if (!info) return false
  const name = info.filename || info.path || ''
  const ext = name.split('.').pop().toLowerCase().split('?')[0]
  return ['png', 'jpg', 'jpeg', 'gif', 'bmp', 'webp', 'svg', 'ico', 'avif'].includes(ext)
}

// 将 content 统一转换为 blocks 数组用于渲染
const blocks = computed(() => {
  // 如果 content 已经是数组（ContentBlocks 格式）
  if (Array.isArray(props.content)) {
    return props.content
  }
  // 如果 content 是字符串（旧格式兼容）
  if (typeof props.content === 'string' && props.content) {
    return [{ type: 'text', text: props.content }]
  }
  return []
})

// 渲染 markdown 文本
function renderMarkdown(text) {
  if (!text) return ''
  return marked(text.trim())
}

// 点击图片时在新窗口打开原图
function openImage(url) {
  window.open(getDisplayUrl(url), '_blank')
}

// 处理 SSE 推送的旧格式文件信息，转换为 ContentBlock 格式渲染
const sseFiles = computed(() => {
  if (!props.files || !props.files.length) return []
  return props.files
})
</script>

<template>
  <div class="chat-msg" :class="role">
    <div class="chat-avatar">
      <el-icon :size="20">
        <UserFilled v-if="role === 'user'" />
        <Cpu v-else />
      </el-icon>
    </div>
    <div class="chat-bubble">
      <template v-if="role === 'assistant'">
        <!-- SSE 推送的旧格式文件附件（向后兼容） -->
        <div v-if="sseFiles.length" class="files-container">
          <template v-for="(f, i) in sseFiles" :key="'sse-' + i">
            <!-- 图片直接展示 -->
            <img
              v-if="isImage(f)"
              :src="getDisplayUrl(f.path)"
              :alt="f.filename || '图片'"
              class="chat-image"
              @click="openImage(f.path)"
            />
            <!-- 非图片显示文件卡片 -->
            <FileCard
              v-else
              :file-type="f.fileType"
              :path="f.path"
              :filename="f.filename"
              :size="f.size"
            />
          </template>
        </div>

        <!-- 结构化 ContentBlocks 渲染 -->
        <template v-for="(block, i) in blocks" :key="'block-' + i">
          <!-- 文本块 -->
          <div v-if="block.type === 'text' && block.text" class="chat-markdown" v-html="renderMarkdown(block.text)" />

          <!-- 图片块 -->
          <img
            v-else-if="block.type === 'image'"
            :src="getDisplayUrl(block.source?.url || '')"
            :alt="block.filename || '图片'"
            class="chat-image"
            @click="openImage(block.source?.url || '')"
          />

          <!-- 视频块 -->
          <video v-else-if="block.type === 'video'" :src="getDisplayUrl(block.source?.url || '')" controls class="chat-video" />

          <!-- 音频块 -->
          <audio v-else-if="block.type === 'audio'" :src="getDisplayUrl(block.source?.url || '')" controls class="chat-audio" />

          <!-- 文件块 -->
          <FileCard
            v-else-if="block.type === 'file'"
            :path="block.source?.url || ''"
            :filename="block.filename || ''"
          />
        </template>
      </template>
      <!-- 用户消息：纯文本显示 -->
      <div v-else class="chat-text">{{ typeof content === 'string' ? content : (Array.isArray(content) ? content.map(b => b.text || '').join('') : content) }}</div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.chat-msg {
  display: flex;
  gap: 12px;
  margin-bottom: 20px;
  &.user { flex-direction: row-reverse; }
}
.chat-avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  background: #409eff;
  color: #fff;
  .user & { background: #67c23a; }
}
.chat-bubble {
  max-width: 70%;
  padding: 12px 16px;
  border-radius: 12px;
  font-size: 14px;
  .user & { background: #e8f4ff; }
  .assistant & { background: #ffffff; box-shadow: 0 1px 3px rgba(0,0,0,.06); }
}
.chat-text { white-space: pre-wrap; word-break: break-word; }
.files-container {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 8px;
}
.chat-image {
  max-width: 100%;
  max-height: 400px;
  border-radius: 8px;
  cursor: pointer;
  transition: opacity .2s;
  display: block;
  margin-bottom: 8px;
  &:hover { opacity: 0.85; }
}
.chat-video {
  max-width: 100%;
  border-radius: 8px;
  margin-bottom: 8px;
}
.chat-audio {
  width: 100%;
  margin-bottom: 8px;
}
</style>