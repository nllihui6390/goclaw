<script setup>
import { computed, ref, inject, onMounted, onUnmounted } from 'vue'
import { marked } from 'marked'
import FileCard from '@/components/chat/FileCard.vue'

marked.setOptions({ breaks: true, gfm: true })

const api = inject('api')

const props = defineProps({
  role: String,
  content: [Array, String],
  files: Array,
  thinking: Array,      // 思考内容数组
  tool_calls: Array     // 工具调用数组
})

const isWails = !!(
  typeof window !== 'undefined' && (
    window._wails?.environment ||
    window.chrome?.webview?.postMessage ||
    window.webkit?.messageHandlers?.external ||
    window.wails?.invoke
  )
)

const blobUrls = ref({})
const thinkingExpanded = ref(false)

function formatJSON(str) {
  if (!str) return ''
  try {
    // 尝试解析为 JSON 并格式化
    const obj = JSON.parse(str)
    return JSON.stringify(obj, null, 2)
  } catch {
    // 如果不是 JSON，直接返回原字符串
    return str
  }
}

function isLocalPath(url) {
  if (!url) return false
  if (url.startsWith('http://') || url.startsWith('https://') || url.startsWith('data:')) return false
  return true
}

function getDisplayUrl(url) {
  if (!url) return ''
  if (url.startsWith('http://') || url.startsWith('https://') || url.startsWith('data:')) return url
  if (isWails && blobUrls.value[url]) return blobUrls.value[url]
  return `/api/v1/files/preview?path=${encodeURIComponent(url)}`
}

function isImage(info) {
  if (!info) return false
  const name = info.filename || info.path || ''
  const ext = name.split('.').pop().toLowerCase().split('?')[0]
  return ['png', 'jpg', 'jpeg', 'gif', 'bmp', 'webp', 'svg', 'ico', 'avif'].includes(ext)
}

const allImageUrls = computed(() => {
  const urls = []
  if (props.files?.length) {
    for (const f of props.files) {
      if (isImage(f)) urls.push(getDisplayUrl(f.path))
    }
  }
  if (Array.isArray(props.content)) {
    for (const block of props.content) {
      if (block.type === 'image') urls.push(getDisplayUrl(block.source?.url || ''))
    }
  }
  return urls.filter(u => u)
})

function getImageIndex(url) {
  return allImageUrls.value.indexOf(getDisplayUrl(url))
}

const blocks = computed(() => {
  if (Array.isArray(props.content)) return props.content
  if (typeof props.content === 'string' && props.content) return [{ type: 'text', text: props.content }]
  return []
})

function extractFilename(url) {
  if (!url) return '图片'
  let path = url
  if (path.startsWith('file:///')) path = path.slice(8)
  else if (path.startsWith('file://')) path = path.slice(7)
  else if (path.startsWith('file:')) path = path.slice(5)
  const parts = path.split(/[\/\\]/)
  const last = parts[parts.length - 1]
  if (last && last.includes('.')) return last
  return '图片'
}

// Handle click on markdown images
function onMdClick(e) {
  if (e.target.tagName === 'IMG' && e.target.classList.contains('md-image')) {
    const src = e.target.src
    openPreview(src, allImageUrls.value.indexOf(src))
  }
}

function onMdContextMenu(e) {
  if (e.target.tagName === 'IMG' && e.target.classList.contains('md-image')) {
    const src = e.target.src
    const filename = src.split('/').pop() || '图片'
    onImageContextMenu(e, src, src, filename)
  }
}

function renderMarkdown(text) {
  if (!text) return ''
  let html = marked(text.trim())
  // Post-process: convert local image srcs to proxy URLs
  html = html.replace(/<img\s+src="([^"]+)"/g, (match, href) => {
    if (href && !href.startsWith('http://') && !href.startsWith('https://') && !href.startsWith('data:') && !href.startsWith('blob:')) {
      const proxyUrl = blobUrls.value[href] || `/api/v1/files/preview?path=${encodeURIComponent(href)}`
      return `<img src="${proxyUrl}" data-original="${href}" class="md-image"`
    }
    return match
  })
  return html
}

const sseFiles = computed(() => props.files?.length ? props.files : [])

onMounted(async () => {
  if (!isWails || !api.getMedia) return

  const paths = []
  if (props.files?.length) {
    for (const f of props.files) {
      if (isLocalPath(f.path)) paths.push(f.path)
    }
  }
  if (Array.isArray(props.content)) {
    for (const block of props.content) {
      const url = block.source?.url || ''
      if (isLocalPath(url)) paths.push(url)
      // Extract markdown image paths from text blocks
      if (block.type === 'text' && block.text) {
        const mdImages = block.text.matchAll(/!\[.*?\]\(([^)]+)\)/g)
        for (const m of mdImages) {
          if (isLocalPath(m[1])) paths.push(m[1])
        }
      }
    }
  }

  const results = await Promise.all(paths.map(async (path) => {
    try {
      const blobUrl = await api.getMedia(path)
      return { path, blobUrl }
    } catch (e) {
      console.error('[ChatMessage] getMedia error:', path, e)
      return { path, blobUrl: null }
    }
  }))

  for (const { path, blobUrl } of results) {
    if (blobUrl) {
      blobUrls.value[path] = blobUrl
    }
  }
})

onUnmounted(() => {
  for (const url of Object.values(blobUrls.value)) {
    URL.revokeObjectURL(url)
  }
})

// ──── Image preview ────
const previewVisible = ref(false)
const previewUrl = ref('')
const previewIndex = ref(0)
const previewScale = ref(1)
const previewTranslate = ref({ x: 0, y: 0 })

function openPreview(url, index) {
  if (!url) return
  previewUrl.value = url
  previewIndex.value = index
  previewScale.value = 1
  previewTranslate.value = { x: 0, y: 0 }
  previewVisible.value = true
}

function closePreview() {
  previewVisible.value = false
}

function prevImage() {
  if (previewIndex.value > 0) {
    previewIndex.value--
    previewUrl.value = allImageUrls.value[previewIndex.value]
    previewScale.value = 1
    previewTranslate.value = { x: 0, y: 0 }
  }
}

function nextImage() {
  if (previewIndex.value < allImageUrls.value.length - 1) {
    previewIndex.value++
    previewUrl.value = allImageUrls.value[previewIndex.value]
    previewScale.value = 1
    previewTranslate.value = { x: 0, y: 0 }
  }
}

function onPreviewWheel(e) {
  e.preventDefault()
  previewScale.value = Math.max(0.2, Math.min(7, previewScale.value + (e.deltaY > 0 ? -0.1 : 0.1)))
}

let isDragging = false
let dragStart = { x: 0, y: 0 }

function onPreviewMouseDown(e) {
  isDragging = true
  dragStart = { x: e.clientX - previewTranslate.value.x, y: e.clientY - previewTranslate.value.y }
}

function onPreviewMouseMove(e) {
  if (!isDragging) return
  previewTranslate.value = { x: e.clientX - dragStart.x, y: e.clientY - dragStart.y }
}

function onPreviewMouseUp() {
  isDragging = false
}

// ──── Context menu ────
const ctxMenuVisible = ref(false)
const ctxMenuStyle = ref({ left: '0px', top: '0px' })
const ctxMenuSourceUrl = ref('')
const ctxMenuDisplayUrl = ref('')
const ctxMenuFilename = ref('')

function onImageContextMenu(e, sourceUrl, displayUrl, filename) {
  e.preventDefault()
  ctxMenuSourceUrl.value = sourceUrl || ''
  ctxMenuDisplayUrl.value = displayUrl || ''
  ctxMenuFilename.value = filename || extractFilename(sourceUrl)
  const menuWidth = 160
  const menuHeight = 120
  let left = e.clientX
  let top = e.clientY
  if (left + menuWidth > window.innerWidth) left = window.innerWidth - menuWidth - 8
  if (top + menuHeight > window.innerHeight) top = window.innerHeight - menuHeight - 8
  ctxMenuStyle.value = { left: left + 'px', top: top + 'px' }
  ctxMenuVisible.value = true
}

function closeCtxMenu() {
  ctxMenuVisible.value = false
}

async function copyImage() {
  closeCtxMenu()
  const url = ctxMenuDisplayUrl.value
  if (!url) return
  try {
    const resp = await fetch(url)
    const blob = await resp.blob()
    await navigator.clipboard.write([
      new ClipboardItem({ [blob.type]: blob })
    ])
  } catch (e) {
    console.error('[ChatMessage] copyImage error:', e)
    try { await navigator.clipboard.writeText(ctxMenuSourceUrl.value) } catch {}
  }
}

async function copyImageLink() {
  closeCtxMenu()
  try { await navigator.clipboard.writeText(ctxMenuSourceUrl.value) } catch {}
}

async function saveImageAs() {
  closeCtxMenu()
  const sourceUrl = ctxMenuSourceUrl.value
  const displayUrl = ctxMenuDisplayUrl.value
  const filename = ctxMenuFilename.value

  if (isWails && api?.downloadFile) {
    api.downloadFile(sourceUrl, filename)
  } else if (displayUrl.startsWith('blob:')) {
    try {
      const resp = await fetch(displayUrl)
      const blob = await resp.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = filename
      a.click()
      setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (e) {
      console.error('[ChatMessage] saveImageAs blob error:', e)
    }
  } else if (displayUrl.startsWith('data:')) {
    const a = document.createElement('a')
    a.href = displayUrl
    a.download = filename
    a.click()
  } else if (sourceUrl.startsWith('file://') || sourceUrl.startsWith('file:')) {
    const a = document.createElement('a')
    a.href = `/api/v1/files/download?path=${encodeURIComponent(sourceUrl)}&filename=${encodeURIComponent(filename)}`
    a.download = filename
    a.click()
  } else if (sourceUrl.startsWith('http://') || sourceUrl.startsWith('https://')) {
    const a = document.createElement('a')
    a.href = sourceUrl
    a.download = filename
    a.target = '_blank'
    a.click()
  }
}
</script>

<template>
  <div class="chat-msg" :class="role">
    <!-- Avatar -->
    <div class="chat-avatar">
      <el-icon :size="18">
        <UserFilled v-if="role === 'user'" />
        <Cpu v-else />
      </el-icon>
    </div>

    <!-- Bubble -->
    <div class="chat-bubble">
      <template v-if="role === 'assistant'">
        <!-- Thinking content (collapsible) -->
        <div v-if="thinking && thinking.length" class="collapsible-block thinking-block">
          <div class="collapsible-header" @click="thinkingExpanded = !thinkingExpanded">
            <span class="collapse-icon">{{ thinkingExpanded ? '▼' : '▶' }}</span>
            <span class="thinking-label">💭 思考过程</span>
          </div>
          <div v-show="thinkingExpanded" class="collapsible-content thinking-content">
            <div v-for="(t, i) in thinking" :key="i" class="thinking-text">{{ t }}</div>
          </div>
        </div>

        <!-- Tool calls (collapsible) -->
        <div v-if="tool_calls && tool_calls.length" class="tool-calls-container">
          <div v-for="(tc, i) in tool_calls" :key="i" class="collapsible-block tool-call-block">
            <div class="collapsible-header" @click="tc.expanded = !tc.expanded">
              <span class="collapse-icon">{{ tc.expanded ? '▼' : '▶' }}</span>
              <span class="tool-icon">{{ tc.status === 'error' ? '❌' : '✅' }}</span>
              <span class="tool-name">{{ tc.name }}</span>
              <span class="tool-status" :class="tc.status">{{ tc.status === 'error' ? '失败' : tc.status === 'calling' ? '调用中...' : '成功' }}</span>
            </div>
            <div v-show="tc.expanded" class="collapsible-content tool-call-content">
              <div v-if="tc.args" class="tool-section">
                <strong>参数：</strong>
                <pre class="tool-code">{{ formatJSON(tc.args) }}</pre>
              </div>
              <div v-if="tc.result" class="tool-section">
                <strong>结果：</strong>
                <pre class="tool-code">{{ tc.result }}</pre>
              </div>
              <div v-if="tc.error" class="tool-section error">
                <strong>错误：</strong>
                <pre class="tool-code">{{ tc.error }}</pre>
              </div>
            </div>
          </div>
        </div>

        <!-- Files -->
        <div v-if="sseFiles.length" class="files-container">
          <template v-for="(f, i) in sseFiles" :key="'sse-' + i">
            <img
              v-if="isImage(f)"
              :src="getDisplayUrl(f.path)"
              :alt="f.filename || '图片'"
              class="chat-image"
              @click="openPreview(getDisplayUrl(f.path), getImageIndex(f.path))"
              @contextmenu="onImageContextMenu($event, f.path, getDisplayUrl(f.path), f.filename || extractFilename(f.path))"
            />
            <FileCard v-else :file-type="f.fileType" :path="f.path" :filename="f.filename" :size="f.size" />
          </template>
        </div>

        <!-- Content blocks -->
        <template v-for="(block, i) in blocks" :key="'block-' + i">
          <div v-if="block.type === 'text' && block.text" class="chat-markdown" v-html="renderMarkdown(block.text)" @click="onMdClick" @contextmenu="onMdContextMenu" />

          <img
            v-else-if="block.type === 'image'"
            :src="getDisplayUrl(block.source?.url || '')"
            :alt="block.filename || '图片'"
            class="chat-image"
            @click="openPreview(getDisplayUrl(block.source?.url || ''), getImageIndex(block.source?.url || ''))"
            @contextmenu="onImageContextMenu($event, block.source?.url || '', getDisplayUrl(block.source?.url || ''), block.filename || extractFilename(block.source?.url || ''))"
          />

          <video v-else-if="block.type === 'video'" :src="getDisplayUrl(block.source?.url || '')" controls class="chat-video" />
          <audio v-else-if="block.type === 'audio'" :src="getDisplayUrl(block.source?.url || '')" controls class="chat-audio" />
          <FileCard v-else-if="block.type === 'file'" :path="block.source?.url || ''" :filename="block.filename || ''" />
        </template>
      </template>

      <!-- User messages -->
      <template v-else>
        <template v-if="Array.isArray(content)">
          <template v-for="(block, i) in content" :key="'user-block-' + i">
            <img
              v-if="block.type === 'image'"
              :src="getDisplayUrl(block.source?.url || '')"
              :alt="block.filename || '图片'"
              class="chat-image"
              @click="openPreview(getDisplayUrl(block.source?.url || ''), getImageIndex(block.source?.url || ''))"
              @contextmenu="onImageContextMenu($event, block.source?.url || '', getDisplayUrl(block.source?.url || ''), block.filename || extractFilename(block.source?.url || ''))"
            />
            <FileCard v-else-if="block.type === 'file'" :path="block.source?.url || ''" :filename="block.filename || ''" />
            <div v-else-if="block.type === 'text' && block.text" class="chat-text">{{ block.text }}</div>
          </template>
        </template>
        <div v-else class="chat-text">{{ content }}</div>
      </template>
    </div>
  </div>

  <!-- Image preview modal -->
  <Teleport to="body">
    <div v-if="previewVisible" class="preview-overlay" @click.self="closePreview" @wheel="onPreviewWheel">
      <button class="preview-close" @click="closePreview">×</button>
      <button v-if="previewIndex > 0" class="preview-nav prev" @click.stop="prevImage">‹</button>
      <button v-if="previewIndex < allImageUrls.length - 1" class="preview-nav next" @click.stop="nextImage">›</button>
      <img
        :src="previewUrl"
        class="preview-image"
        :style="{ transform: `scale(${previewScale}) translate(${previewTranslate.x / previewScale}px, ${previewTranslate.y / previewScale}px)` }"
        @mousedown="onPreviewMouseDown"
        @mousemove="onPreviewMouseMove"
        @mouseup="onPreviewMouseUp"
        @mouseleave="onPreviewMouseUp"
        draggable="false"
      />
      <div class="preview-info">
        <span>{{ previewIndex + 1 }} / {{ allImageUrls.length }}</span>
        <span>{{ Math.round(previewScale * 100) }}%</span>
      </div>
    </div>

    <!-- Context menu -->
    <div
      v-if="ctxMenuVisible"
      class="ctx-menu"
      :style="ctxMenuStyle"
      @click.stop
    >
      <div class="ctx-menu-item" @click="copyImage">📋 复制图片</div>
      <div class="ctx-menu-item" @click="copyImageLink">🔗 复制链接</div>
      <div class="ctx-menu-item" @click="saveImageAs">💾 另存为...</div>
      <div class="ctx-menu-item cancel" @click="closeCtxMenu">❌ 取消</div>
    </div>
    <div v-if="ctxMenuVisible" class="ctx-menu-backdrop" @click="closeCtxMenu"></div>
  </Teleport>
</template>

<style lang="scss" scoped>
@use '@/styles/variables.scss' as *;

.chat-msg {
  display: flex;
  gap: 16px;
  margin-bottom: 24px;
  animation: fade-up 0.3s ease-out;

  &.user {
    flex-direction: row-reverse;
  }
}

.chat-avatar {
  width: 40px;
  height: 40px;
  border-radius: $radius-md;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  background: $bg-elevated;
  border: 1px solid $border-default;

  .user & {
    background: $accent-amber-dim;
    border-color: rgba(255, 159, 67, 0.3);
    color: $accent-amber;
  }

  .assistant & {
    background: $accent-cyan-dim;
    border-color: rgba(0, 212, 255, 0.3);
    color: $accent-cyan;
  }
}

.chat-bubble {
  max-width: 70%;
  min-width: 200px;
  padding: 16px 20px;
  border-radius: $radius-lg;
  font-size: $font-size-base;

  .user & {
    @include glass-panel;
    background: rgba(255, 159, 67, 0.1);
    border-color: rgba(255, 159, 67, 0.2);
  }

  .assistant & {
    @include glass-panel;
    background: $bg-surface;
  }
}

.chat-text {
  white-space: pre-wrap;
  word-break: break-word;
  color: $text-primary;
  line-height: 1.6;
}

// ──── Collapsible blocks (thinking & tool calls) ────
.collapsible-block {
  margin-bottom: 12px;
  border: 1px solid $border-subtle;
  border-radius: $radius-md;
  overflow: hidden;
}

.collapsible-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  cursor: pointer;
  background: rgba(0, 212, 255, 0.05);
  transition: background 0.15s;
  user-select: none;

  &:hover {
    background: rgba(0, 212, 255, 0.1);
  }

  .collapse-icon {
    font-size: 10px;
    color: $text-muted;
    width: 12px;
    flex-shrink: 0;
  }

  .thinking-label {
    font-size: $font-size-sm;
    color: $accent-cyan;
    font-weight: 500;
  }

  .tool-icon {
    font-size: 14px;
    flex-shrink: 0;
  }

  .tool-name {
    font-size: $font-size-sm;
    font-weight: 600;
    color: $text-primary;
  }

  .tool-status {
    font-size: $font-size-xs;
    padding: 2px 8px;
    border-radius: $radius-sm;
    margin-left: auto;

    &.success {
      background: rgba(34, 197, 94, 0.15);
      color: #22c55e;
    }

    &.error {
      background: rgba(239, 68, 68, 0.15);
      color: #ef4444;
    }

    &.calling {
      background: rgba(255, 159, 67, 0.15);
      color: $accent-amber;
    }
  }
}

.collapsible-content {
  padding: 12px;
  border-top: 1px solid $border-subtle;
  background: rgba(0, 0, 0, 0.1);
}

.thinking-content {
  .thinking-text {
    font-size: $font-size-sm;
    color: $text-secondary;
    white-space: pre-wrap;
    word-break: break-word;
    line-height: 1.5;
    margin-bottom: 8px;

    &:last-child {
      margin-bottom: 0;
    }
  }
}

.tool-calls-container {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 12px;
}

.tool-call-content {
  .tool-section {
    margin-bottom: 10px;

    &:last-child {
      margin-bottom: 0;
    }

    strong {
      font-size: $font-size-sm;
      color: $text-secondary;
      display: block;
      margin-bottom: 4px;
    }

    &.error strong {
      color: #ef4444;
    }
  }

  .tool-code {
    font-family: 'Consolas', 'Monaco', monospace;
    font-size: $font-size-xs;
    background: rgba(0, 0, 0, 0.2);
    border: 1px solid $border-subtle;
    border-radius: $radius-sm;
    padding: 8px 10px;
    white-space: pre-wrap;
    word-break: break-word;
    color: $text-primary;
    max-height: 300px;
    overflow-y: auto;
    margin: 0;
  }
}

.files-container {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-bottom: 12px;
}

.chat-image,
:deep(.md-image) {
  max-width: 100%;
  max-height: 400px;
  border-radius: $radius-md;
  cursor: pointer;
  display: block;
  margin-bottom: 12px;
  transition: opacity 0.2s;
  border: 1px solid $border-subtle;

  &:hover {
    opacity: 0.85;
  }
}

.chat-video {
  max-width: 100%;
  border-radius: $radius-md;
  margin-bottom: 12px;
}

.chat-audio {
  width: 100%;
  margin-bottom: 12px;
}

// ──── Preview overlay ────
.preview-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.95);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
}

.preview-close {
  position: absolute;
  top: 24px;
  right: 24px;
  width: 44px;
  height: 44px;
  border: none;
  border-radius: $radius-md;
  background: $bg-elevated;
  color: $text-primary;
  font-size: 28px;
  cursor: pointer;
  transition: all 0.2s;

  &:hover {
    background: $accent-cyan;
    color: $bg-deep;
  }
}

.preview-nav {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  width: 56px;
  height: 56px;
  border: none;
  border-radius: $radius-lg;
  background: $bg-elevated;
  color: $text-primary;
  font-size: 32px;
  cursor: pointer;
  transition: all 0.2s;

  &:hover {
    background: $accent-cyan;
    color: $bg-deep;
  }

  &.prev { left: 24px; }
  &.next { right: 24px; }
}

.preview-image {
  max-width: 90vw;
  max-height: 90vh;
  object-fit: contain;
  cursor: grab;
  transition: transform 0.05s;

  &:active { cursor: grabbing; }
}

.preview-info {
  position: absolute;
  bottom: 24px;
  left: 50%;
  transform: translateX(-50%);
  @include glass-panel;
  padding: 10px 20px;
  border-radius: $radius-xl;
  color: $text-primary;
  font-family: $font-display;
  font-size: $font-size-sm;
  display: flex;
  gap: 24px;
}

// ──── Context menu ────
.ctx-menu {
  position: fixed;
  @include glass-panel;
  border-radius: $radius-lg;
  padding: 8px 0;
  z-index: 10001;
  min-width: 150px;
  animation: fade-up 0.15s ease-out;
}

.ctx-menu-item {
  padding: 10px 16px;
  cursor: pointer;
  font-size: $font-size-sm;
  color: $text-secondary;
  transition: all 0.15s;

  &:hover {
    background: $bg-glass-light;
    color: $text-primary;
  }

  &.cancel {
    border-top: 1px solid $border-subtle;
    margin-top: 4px;
    padding-top: 12px;
    color: $text-muted;
  }
}

.ctx-menu-backdrop {
  position: fixed;
  inset: 0;
  z-index: 10000;
}
</style>