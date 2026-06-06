<script setup>
import { computed, ref, inject, onMounted, onUnmounted } from 'vue'
import { marked } from 'marked'
import FileCard from '@/components/chat/FileCard.vue'

marked.setOptions({ breaks: true, gfm: true })

const api = inject('api')

const props = defineProps({
  role: String,
  content: [Array, String],
  files: Array
})

// 检测是否为 Wails 桌面模式
const isWails = !!(
  typeof window !== 'undefined' && (
    window._wails?.environment ||
    window.chrome?.webview?.postMessage ||
    window.webkit?.messageHandlers?.external ||
    window.wails?.invoke
  )
)

// Blob URLs 存储（当前消息的所有本地文件）
const blobUrls = ref({})

// 判断是否为本地路径（需要通过 getMedia 加载）
function isLocalPath(url) {
  if (!url) return false
  if (url.startsWith('http://') || url.startsWith('https://') || url.startsWith('data:')) return false
  return true
}

// 获取显示 URL
function getDisplayUrl(url) {
  if (!url) return ''
  // 远程 URL 直接使用
  if (url.startsWith('http://') || url.startsWith('https://') || url.startsWith('data:')) return url
  // Wails 模式：使用已加载的 Blob URL
  if (isWails && blobUrls.value[url]) return blobUrls.value[url]
  // Web 模式或未加载：使用 HTTP 端点（Web 模式有效，Wails 模式下会显示占位）
  return `/api/v1/files/preview?path=${encodeURIComponent(url)}`
}

// 判断是否为图片类型
function isImage(info) {
  if (!info) return false
  const name = info.filename || info.path || ''
  const ext = name.split('.').pop().toLowerCase().split('?')[0]
  return ['png', 'jpg', 'jpeg', 'gif', 'bmp', 'webp', 'svg', 'ico', 'avif'].includes(ext)
}

// 收集所有图片 URL
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

// 从 URL 提取文件名（带扩展名）
function extractFilename(url) {
  if (!url) return '图片'
  // 处理 file:// 路径和普通路径
  let path = url
  if (path.startsWith('file:///')) path = path.slice(8) // Windows: file:///C:/...
  else if (path.startsWith('file://')) path = path.slice(7) // Unix: file:///home/...
  else if (path.startsWith('file:')) path = path.slice(5)
  // 取最后一段路径作为文件名
  const parts = path.split(/[\/\\]/)
  const last = parts[parts.length - 1]
  if (last && last.includes('.')) return last
  // URL 没有扩展名，回退
  return '图片'
}

function renderMarkdown(text) {
  if (!text) return ''
  return marked(text.trim())
}

const sseFiles = computed(() => props.files?.length ? props.files : [])

// Wails 模式：加载所有本地文件
onMounted(async () => {
  if (!isWails || !api.getMedia) return

  const paths = []
  // SSE 文件
  if (props.files?.length) {
    for (const f of props.files) {
      if (isLocalPath(f.path)) paths.push(f.path)
    }
  }
  // ContentBlocks
  if (Array.isArray(props.content)) {
    for (const block of props.content) {
      const url = block.source?.url || ''
      if (isLocalPath(url)) paths.push(url)
    }
  }

  // 并行加载所有文件
  const results = await Promise.all(paths.map(async (path) => {
    try {
      const blobUrl = await api.getMedia(path)
      return { path, blobUrl }
    } catch (e) {
      console.error('[ChatMessage] getMedia error:', path, e)
      return { path, blobUrl: null }
    }
  }))

  // 存储 Blob URLs
  for (const { path, blobUrl } of results) {
    if (blobUrl) blobUrls.value[path] = blobUrl
  }
})

// 清理 Blob URLs
onUnmounted(() => {
  for (const url of Object.values(blobUrls.value)) {
    URL.revokeObjectURL(url)
  }
})

// ─────────── 图片预览 ───────────
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

// ─────────── 图片右键菜单 ───────────
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
  // 计算菜单位置，防止溢出屏幕
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

// 复制图片到剪贴板
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
    // fallback: 复制链接
    try { await navigator.clipboard.writeText(ctxMenuSourceUrl.value) } catch {}
  }
}

// 复制图片链接/路径
async function copyImageLink() {
  closeCtxMenu()
  try { await navigator.clipboard.writeText(ctxMenuSourceUrl.value) } catch {}
}

// 另存为（Wails 用保存对话框，Web 用浏览器下载）
async function saveImageAs() {
  closeCtxMenu()
  const sourceUrl = ctxMenuSourceUrl.value
  const displayUrl = ctxMenuDisplayUrl.value
  const filename = ctxMenuFilename.value

  if (isWails && api?.downloadFile) {
    // Wails 模式：调用后端保存对话框
    api.downloadFile(sourceUrl, filename)
  } else if (displayUrl.startsWith('blob:')) {
    // Web 模式 + Blob URL：直接从 blob 下载
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
    // Data URL：直接下载
    const a = document.createElement('a')
    a.href = displayUrl
    a.download = filename
    a.click()
  } else if (sourceUrl.startsWith('file://') || sourceUrl.startsWith('file:')) {
    // Web 模式 + file:// 路径：通过 HTTP 端点下载
    const a = document.createElement('a')
    a.href = `/api/v1/files/download?path=${encodeURIComponent(sourceUrl)}&filename=${encodeURIComponent(filename)}`
    a.download = filename
    a.click()
  } else if (sourceUrl.startsWith('http://') || sourceUrl.startsWith('https://')) {
    // 远程 URL：直接下载
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
    <div class="chat-avatar">
      <el-icon :size="20">
        <UserFilled v-if="role === 'user'" />
        <Cpu v-else />
      </el-icon>
    </div>
    <div class="chat-bubble">
      <template v-if="role === 'assistant'">
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

        <template v-for="(block, i) in blocks" :key="'block-' + i">
          <div v-if="block.type === 'text' && block.text" class="chat-markdown" v-html="renderMarkdown(block.text)" />

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
      <!-- 用户消息：同样支持 ContentBlocks（图片、文件） -->
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

  <!-- 图片预览模态框 -->
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

    <!-- 图片右键菜单 -->
    <div
      v-if="ctxMenuVisible"
      class="ctx-menu"
      :style="ctxMenuStyle"
      @click.stop
    >
      <div class="ctx-menu-item" @click="copyImage">📋 复制图片</div>
      <div class="ctx-menu-item" @click="copyImageLink">🔗 复制链接</div>
      <div class="ctx-menu-item" @click="saveImageAs">💾 另存为...</div>
      <div class="ctx-menu-item" @click="closeCtxMenu">❌ 取消</div>
    </div>
    <div v-if="ctxMenuVisible" class="ctx-menu-backdrop" @click="closeCtxMenu"></div>
  </Teleport>
</template>

<style lang="scss" scoped>
.chat-msg { display: flex; gap: 12px; margin-bottom: 20px; &.user { flex-direction: row-reverse; } }
.chat-avatar { width: 36px; height: 36px; border-radius: 50%; display: flex; align-items: center; justify-content: center; flex-shrink: 0; background: #409eff; color: #fff; .user & { background: #67c23a; } }
.chat-bubble { max-width: 70%; padding: 12px 16px; border-radius: 12px; font-size: 14px; .user & { background: #e8f4ff; } .assistant & { background: #fff; box-shadow: 0 1px 3px rgba(0,0,0,.06); } }
.chat-text { white-space: pre-wrap; word-break: break-word; }
.files-container { display: flex; flex-direction: column; gap: 8px; margin-bottom: 8px; }
.chat-image { max-width: 100%; max-height: 400px; border-radius: 8px; cursor: pointer; display: block; margin-bottom: 8px; transition: opacity .2s; &:hover { opacity: 0.85; } }
.chat-video { max-width: 100%; border-radius: 8px; margin-bottom: 8px; }
.chat-audio { width: 100%; margin-bottom: 8px; }

.preview-overlay { position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,.9); display: flex; align-items: center; justify-content: center; z-index: 9999; }
.preview-close { position: absolute; top: 20px; right: 20px; width: 40px; height: 40px; border: none; border-radius: 50%; background: rgba(255,255,255,.1); color: #fff; font-size: 24px; cursor: pointer; &:hover { background: rgba(255,255,255,.2); } }
.preview-nav { position: absolute; top: 50%; transform: translateY(-50%); width: 50px; height: 50px; border: none; border-radius: 50%; background: rgba(255,255,255,.1); color: #fff; font-size: 28px; cursor: pointer; &:hover { background: rgba(255,255,255,.2); } &.prev { left: 20px; } &.next { right: 20px; } }
.preview-image { max-width: 90vw; max-height: 90vh; object-fit: contain; cursor: grab; transition: transform .05s; &:active { cursor: grabbing; } }
.preview-info { position: absolute; bottom: 20px; left: 50%; transform: translateX(-50%); background: rgba(0,0,0,.6); padding: 8px 16px; border-radius: 20px; color: #fff; font-size: 14px; display: flex; gap: 20px; }

.ctx-menu { position: fixed; background: #fff; border-radius: 8px; box-shadow: 0 4px 20px rgba(0,0,0,.15); padding: 6px 0; z-index: 10001; min-width: 140px; }
.ctx-menu-item { padding: 8px 16px; cursor: pointer; font-size: 14px; color: #333; transition: background .15s; &:hover { background: #f0f7ff; } }
.ctx-menu-backdrop { position: fixed; top: 0; left: 0; right: 0; bottom: 0; z-index: 10000; }
</style>