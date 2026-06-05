<script setup>
import { computed } from 'vue'
import { marked } from 'marked'
import FileCard from '@/components/chat/FileCard.vue'

marked.setOptions({ breaks: true, gfm: true })

const IMAGE_EXTENSIONS = ['png', 'jpg', 'jpeg', 'gif', 'bmp', 'webp', 'svg', 'ico', 'avif', 'tiff', 'tif']

// 判断文件块是否为图片
function isImage(info) {
  if (!info.filename && !info.path) return false
  const name = info.filename || info.path || ''
  const ext = name.split('.').pop().toLowerCase().split('?')[0] // 去掉 URL query 参数
  return IMAGE_EXTENSIONS.includes(ext)
}

const props = defineProps({
  role: String,
  content: String,
  files: Array // 文件附件列表（SSE file 事件推送）
})

// 解析 [FILE_BLOCK] 标记，将 content 拆分为 segments（向后兼容）
const segments = computed(() => {
  if (!props.content) return []
  const text = props.content
  const result = []
  const regex = /\[FILE_BLOCK\]\n([\s\S]*?)\n?\[\/FILE_BLOCK\]/g
  let lastIndex = 0
  let match

  while ((match = regex.exec(text)) !== null) {
    // match 前面的文本段
    if (match.index > lastIndex) {
      result.push({ type: 'text', content: text.slice(lastIndex, match.index) })
    }
    // 解析文件块信息
    const block = match[1]
    const info = { fileType: '', path: '', filename: '', size: 0 }
    for (const line of block.split('\n')) {
      const trimmed = line.trim()
      if (trimmed.startsWith('类型:')) info.fileType = trimmed.slice(3).trim()
      else if (trimmed.startsWith('路径:')) info.path = trimmed.slice(3).trim()
      else if (trimmed.startsWith('文件名:')) info.filename = trimmed.slice(4).trim()
      else if (trimmed.startsWith('大小:')) {
        const sizeStr = trimmed.slice(3).trim()
        const num = parseInt(sizeStr)
        if (!isNaN(num)) info.size = num
      }
    }
    result.push({ type: 'file', info })
    lastIndex = match.index + match[0].length
  }

  // 尾部文本
  if (lastIndex < text.length) {
    result.push({ type: 'text', content: text.slice(lastIndex) })
  }

  // 没有 FILE_BLOCK 时，整体作为 text 段
  if (result.length === 0) {
    result.push({ type: 'text', content: text })
  }

  return result
})

// 渲染文本段
function renderText(content) {
  if (!content) return ''
  return marked(content.trim())
}

// 点击图片时在新窗口打开
function openImage(url) {
  window.open(url, '_blank')
}
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
        <!-- SSE 推送的文件附件 -->
        <div v-if="files && files.length" class="files-container">
          <template v-for="(f, i) in files" :key="i">
            <!-- 图片直接展示 -->
            <img
              v-if="isImage(f)"
              :src="f.path"
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
        <!-- 文本内容（支持 [FILE_BLOCK] 解析） -->
        <template v-for="(seg, i) in segments" :key="i">
          <div v-if="seg.type === 'text' && seg.content.trim()" class="chat-markdown" v-html="renderText(seg.content)" />
          <!-- 图片直接展示 -->
          <img
            v-else-if="seg.type === 'file' && isImage(seg.info)"
            :src="seg.info.path"
            :alt="seg.info.filename || '图片'"
            class="chat-image"
            @click="openImage(seg.info.path)"
          />
          <!-- 非图片显示文件卡片 -->
          <FileCard
            v-else-if="seg.type === 'file'"
            :file-type="seg.info.fileType"
            :path="seg.info.path"
            :filename="seg.info.filename"
            :size="seg.info.size"
          />
        </template>
      </template>
      <div v-else class="chat-text">{{ content }}</div>
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
</style>