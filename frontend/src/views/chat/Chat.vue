<script setup>
import { ref, inject, nextTick, onMounted, watch, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { useAgentStore } from '@/stores/agent'
import { useSessionStore } from '@/stores/session'
import ChatMessage from '@/components/chat/ChatMessage.vue'

const route = useRoute()
const router = useRouter()
const api = inject('api')
const agentStore = useAgentStore()
const sessionStore = useSessionStore()
const messages = ref([])
const input = ref('')
const sending = ref(false)
const files = ref([])
const fileInput = ref(null)
const showNewChatOverlay = ref(false)

const sessionId = computed(() => route.query.session || sessionStore.sessionId)
const viewingSession = computed(() => !!route.query.session)

function onFileChange(e) {
  const selected = Array.from(e.target.files || [])
  selected.forEach(file => {
    if (file.type && file.type.startsWith('image/')) {
      const reader = new FileReader()
      reader.onload = () => { file.thumb = reader.result }
      reader.readAsDataURL(file)
    }
  })
  files.value = [...files.value, ...selected]
  e.target.value = ''
}

function removeFile(index) {
  files.value.splice(index, 1)
}

function backToDefault() {
  router.push('/chat')
}

async function loadHistory() {
  if (sending.value) return
  try {
    const history = await api.getChatHistory(sessionId.value, agentStore.selectedAgent)
    messages.value = (history && history.length > 0)
      ? history.map(m => {
          const meta = m.metadata || {}
          return {
            role: m.role,
            content: m.content,
            thinking: meta.thinking ? [meta.thinking] : [],
            tool_calls: (meta.tool_calls || []).map(tc => ({
              name: tc.name,
              args: tc.args,
              result: tc.result,
              error: tc.error,
              status: tc.status || 'success',
              approval_id: tc.approval_id,
              approval_state: tc.approval_state,
              guard_message: tc.guard_message,
              expanded: tc.status === 'guard' ? true : false,
            })),
            files: undefined
          }
        })
      : []
    await nextTick()
    scrollBottom()
  } catch (e) {
    console.log('[Chat] 加载历史失败:', e.message)
  }
}

onMounted(async () => {
  await sessionStore.initSession(api, agentStore.selectedAgent)
  if (route.query.agent && route.query.agent !== agentStore.selectedAgent) {
    agentStore.setAgent(route.query.agent)
  }
  loadHistory()
})

watch(() => agentStore.selectedAgent, async (newAgent) => {
  if (!viewingSession.value) {
    await sessionStore.switchAgent(api, newAgent)
    loadHistory()
  }
})

watch(() => route.query, (q, oldQ) => {
  if (q.session !== oldQ?.session) {
    if (q.agent && q.agent !== agentStore.selectedAgent) {
      agentStore.setAgent(q.agent)
    }
    loadHistory()
  }
})

async function send() {
  const text = input.value.trim()
  if ((!text && !files.value.length) || sending.value) return
  input.value = ''
  sending.value = true

  const uploadedFiles = []
  for (const file of files.value) {
    try {
      const result = await api.uploadChatFile(sessionId.value, file)
      if (result.error) {
        ElMessage.error(`上传失败: ${file.name} - ${result.error}`)
      } else {
        uploadedFiles.push(result)
      }
    } catch (e) {
      ElMessage.error(`上传失败: ${file.name} - ${e.message}`)
    }
  }
  files.value = []

  const contentBlocks = []
  for (const f of uploadedFiles) {
    if (f.is_image) {
      contentBlocks.push({ type: 'image', source: { type: 'url', url: f.path }, filename: f.filename })
    } else {
      contentBlocks.push({ type: 'file', source: { type: 'url', url: f.path }, filename: f.filename })
    }
  }
  if (text) {
    contentBlocks.push({ type: 'text', text })
  }

  messages.value.push({ role: 'user', content: contentBlocks.length > 1 || uploadedFiles.length > 0 ? contentBlocks : text })
  await nextTick()
  setTimeout(scrollBottom, 50)

  let sendContent = text
  if (uploadedFiles.length > 0) {
    const fileDescs = uploadedFiles.map(f =>
      f.is_image ? `[图片: ${f.filename} (${f.path})]` : `[文件: ${f.filename} (${f.path})]`
    ).join('\n')
    sendContent = fileDescs + (text ? '\n' + text : '')
  }

  try {
    if (api.isStreaming) {
      let fullContent = ''
      let files = []
      let contentBlocks = []
      let thinking = []
      let toolCalls = []
      let currentToolName = ''

      // 辅助函数：确保有 assistant 消息
      const ensureAssistantMsg = () => {
        if (messages.value[messages.value.length - 1].role !== 'assistant') {
          messages.value.push({
            role: 'assistant',
            content: '',
            files: [],
            thinking: [],
            tool_calls: []
          })
        }
      }

      // 辅助函数：查找正在调用的工具
      const findCallingTool = (name) => {
        const lastMsg = messages.value[messages.value.length - 1]
        return lastMsg.tool_calls?.find(tc => tc.name === name && tc.status === 'calling')
      }

      for await (const event of api.sendMessage(sessionId.value, sendContent, agentStore.selectedAgent)) {
        if (event.type === 'file') {
          files.push(event.info)
          ensureAssistantMsg()
          messages.value[messages.value.length - 1].files = [...files]
        } else if (event.type === 'text') {
          // 流式文本增量：拼接到 fullContent 并实时更新显示
          fullContent += (event.text || '')
          ensureAssistantMsg()
          messages.value[messages.value.length - 1].content = fullContent
        } else if (event.type === 'content') {
          if (event.blocks && Array.isArray(event.blocks)) {
            contentBlocks.push(...event.blocks)
            files = []
            fullContent = '' // content 事件表示结构化块已替代纯文本
            const finalContent = [...contentBlocks, { type: 'text', text: fullContent }]
            ensureAssistantMsg()
            messages.value[messages.value.length - 1].content = finalContent
            messages.value[messages.value.length - 1].files = []
          }
        } else if (event.type === 'thinking') {
          // 处理思考内容
          thinking.push(event.content)
          ensureAssistantMsg()
          messages.value[messages.value.length - 1].thinking = [...thinking]
        } else if (event.type === 'tool_call') {
          // 处理工具调用开始
          currentToolName = event.tool_name
          const toolCall = {
            name: event.tool_name,
            args: event.args,
            status: 'calling',
            expanded: false
          }
          toolCalls.push(toolCall)
          ensureAssistantMsg()
          messages.value[messages.value.length - 1].tool_calls = [...toolCalls]
        } else if (event.type === 'tool_result') {
          // 处理工具调用结果
          const tc = findCallingTool(event.tool_name)
          if (tc) {
            tc.result = event.result
            tc.status = 'success'
            // 触发响应式更新
            ensureAssistantMsg()
            messages.value[messages.value.length - 1].tool_calls = [...messages.value[messages.value.length - 1].tool_calls]
          }
        } else if (event.type === 'tool_error') {
          // 处理工具调用错误
          const tc = findCallingTool(event.tool_name)
          if (tc) {
            tc.error = event.error
            tc.status = 'error'
            // 触发响应式更新
            ensureAssistantMsg()
            messages.value[messages.value.length - 1].tool_calls = [...messages.value[messages.value.length - 1].tool_calls]
          }
        } else if (event.type === 'guard') {
          // 处理安全守卫事件（审批通知）
          // 去重：相同 approval_id 只保留最新状态，更新而非追加
          const guardCall = {
            name: event.tool_name,
            args: event.args,
            status: 'guard',
            approval_id: event.approval_id,
            approval_state: event.approval_state,
            guard_message: event.guard_message,
            expanded: true
          }
          if (event.approval_id) {
            const existingIdx = toolCalls.findIndex(tc => tc.approval_id === event.approval_id)
            if (existingIdx >= 0) {
              toolCalls[existingIdx] = guardCall
            } else {
              toolCalls.push(guardCall)
            }
          } else {
            toolCalls.push(guardCall)
          }
          ensureAssistantMsg()
          messages.value[messages.value.length - 1].tool_calls = [...toolCalls]
        } else if (event.type === 'text') {
          fullContent += (event.text || '')
          ensureAssistantMsg()
          messages.value[messages.value.length - 1].content = fullContent
        }
        await nextTick()
        scrollBottom()
      }
    } else {
      const rawContent = await api.sendMessage(sessionId.value, sendContent, agentStore.selectedAgent)
      let content, thinking = [], toolCalls = []
      try {
        const parsed = JSON.parse(rawContent)
        if (parsed && typeof parsed === 'object' && parsed.content !== undefined) {
          // 新格式：{content: [...], metadata: {thinking: "...", tool_calls: [...]}}
          content = Array.isArray(parsed.content) ? parsed.content : rawContent
          if (parsed.metadata) {
            if (parsed.metadata.thinking) {
              thinking = [parsed.metadata.thinking]
            }
            if (parsed.metadata.tool_calls) {
              toolCalls = parsed.metadata.tool_calls.map(tc => ({
                name: tc.name,
                args: tc.args,
                result: tc.result,
                error: tc.error,
                status: tc.status || 'success',
                approval_id: tc.approval_id,
                approval_state: tc.approval_state,
                guard_message: tc.guard_message,
                expanded: tc.status === 'guard' ? true : false, // guard 事件默认展开
              }))
            }
          }
        } else if (Array.isArray(parsed)) {
          content = parsed
        } else {
          content = rawContent
        }
      } catch {
        content = rawContent
      }
      messages.value.push({ role: 'assistant', content, thinking, tool_calls: toolCalls })
      await nextTick()
      scrollBottom()
    }
  } catch (e) {
    messages.value.push({ role: 'assistant', content: '请求失败: ' + e.message })
  } finally {
    sending.value = false
  }
}

function scrollBottom() {
  const el = document.querySelector('.chat-messages')
  if (el) el.scrollTop = el.scrollHeight
}

function formatSize(bytes) {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

function onKeydown(e) {
  if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send() }
}

function openNewChat() {
  showNewChatOverlay.value = true
}

function closeNewChat() {
  showNewChatOverlay.value = false
}

async function confirmNewChat() {
  if (viewingSession.value) {
    router.push('/chat')
  }
  closeNewChat()
  sending.value = true
  try {
    const data = await api.createSession(agentStore.selectedAgent)
    sessionStore.sessionId = data.session_id
    sessionStore.saveId(agentStore.selectedAgent, data.session_id)
    messages.value = []
    ElMessage.success('新会话已创建')
  } catch (e) {
    ElMessage.error('创建会话失败: ' + e.message)
  } finally {
    sending.value = false
  }
}
</script>

<template>
  <div class="chat-page">
    <!-- New chat button -->
    <el-tooltip content="新建聊天" placement="left">
      <div class="new-chat-btn" @click="openNewChat">
        <el-icon :size="16"><Plus /></el-icon>
      </div>
    </el-tooltip>

    <!-- New chat overlay -->
    <div v-if="showNewChatOverlay" class="new-chat-overlay" @click.self="closeNewChat">
      <div class="new-chat-panel">
        <p class="new-chat-title">开始新对话？</p>
        <p class="new-chat-sub">当前聊天记录将保留在会话历史中</p>
        <div class="new-chat-actions">
          <el-button @click="closeNewChat">取消</el-button>
          <el-button type="primary" @click="confirmNewChat">新建聊天</el-button>
        </div>
      </div>
    </div>

    <!-- Session banner -->
    <div v-if="viewingSession" class="session-banner">
      <div class="banner-content">
        <span>查看会话：<strong>{{ sessionId }}</strong></span>
        <span class="banner-divider">|</span>
        <span>Agent: {{ route.query.agent || agentStore.selectedAgent }}</span>
      </div>
      <button class="back-btn" @click="backToDefault">返回默认对话</button>
    </div>

    <!-- Messages area -->
    <div class="chat-messages">
      <ChatMessage
        v-for="(msg, i) in messages"
        :key="i"
        :role="msg.role"
        :content="msg.content"
        :files="msg.files"
        :thinking="msg.thinking"
        :tool_calls="msg.tool_calls"
      />

      <!-- Loading indicator -->
      <div v-if="sending && (messages.length === 0 || messages[messages.length - 1].role !== 'assistant')" class="chat-loading">
        <div class="loading-avatar">
          <el-icon :size="20"><Cpu /></el-icon>
        </div>
        <div class="loading-bubble">
          <span class="loading-dot"></span>
          <span class="loading-dot"></span>
          <span class="loading-dot"></span>
        </div>
      </div>

      <!-- Empty state -->
      <div v-if="messages.length === 0" class="chat-empty">
        <h2 class="empty-title">go-claw 智能助手</h2>
        <p class="empty-subtitle">在下方输入消息开始对话</p>
      </div>
    </div>

    <!-- Input area -->
    <div class="chat-input-area">
      <!-- Files preview -->
      <div v-if="files.length" class="files-bar">
        <div v-for="(f, i) in files" :key="i" class="file-tag">
          <img v-if="f.type && f.type.startsWith('image/') && f.thumb" :src="f.thumb" class="file-thumb" />
          <div v-else class="file-icon-preview">📄</div>
          <span class="file-name">{{ f.name }}</span>
          <span class="file-size">{{ formatSize(f.size) }}</span>
          <button class="file-remove" @click="removeFile(i)" :disabled="sending">×</button>
        </div>
      </div>

      <!-- Input wrapper -->
      <div class="sender-wrapper">
        <div class="sender-content">
          <textarea
            v-model="input"
            class="sender-input"
            placeholder="输入消息… Enter 发送，Shift+Enter 换行"
            rows="2"
            maxlength="10000"
            @keydown="onKeydown"
            :disabled="sending"
          />
        </div>
        <div class="sender-footer">
          <div class="sender-left">
            <input ref="fileInput" type="file" multiple hidden @change="onFileChange"/>
            <button class="sender-icon-btn" title="上传文件" :disabled="sending" @click="fileInput?.click()">
              <svg width="18" height="18" viewBox="0 0 1024 1024" fill="currentColor">
                <path d="M899.3 577.8L635.3 845.1q-40.4 40.9-93.3 62.5-51 20.9-106.3 20.9t-106.3-20.8q-52.8-21.6-93.3-62.5-39.8-40.2-60.7-92.4-20.2-50.3-20.2-104.8 0-54.5 20.2-104.8 21-52.1 60.7-92.4l296-299.6q28.5-28.8 65.7-44.1 35.9-14.7 74.9-14.7 39 0 74.9 14.7 37.2 15.2 65.7 44.1 28 28.4 42.8 65.1 14.3 35.5 14.3 73.8t-14.3 73.8q-14.8 36.7-42.8 65.1l-266.9 270.2q-16.5 16.7-38.2 25.6-20.9 8.5-43.5 8.5t-43.5-8.5q-21.6-8.9-38.2-25.6-16.3-16.5-24.8-37.8-8.3-20.6-8.3-42.9 0-22.3 8.3-42.9 8.6-21.3 24.8-37.8l237.7-240.7a32 32 0 0 1 45.5 45l-237.7 240.7q-7.2 7.3-11 16.7-3.7 9.1-3.7 19 0 9.9 3.7 19 3.8 9.4 11 16.7 7.3 7.4 16.9 11.3 9.2 3.8 19.3 3.8 10 0 19.3-3.8 9.5-3.9 16.9-11.3l266.8-270.2q19-19.2 28.9-44 9.6-24 9.6-50 0-26-9.6-50-10-24.8-28.9-44-19.3-19.5-44.4-29.8-24.3-9.9-50.7-9.9-26.4 0-50.7 9.9-25.1 10.3-44.4 29.8l-296 299.6q-30.7 31.1-46.9 71.3-15.6 38.8-15.6 80.9t15.6 80.9q16.2 40.2 46.9 71.3 31.2 31.6 72 48.3 39.3 16.1 82.1 16.1t82.1-16.1q40.7-16.7 72-48.3l264-267.3a32 32 0 0 1 45.4 45.1z"/>
              </svg>
            </button>
          </div>
          <div class="sender-right">
            <span class="sender-counter">{{ input.length }}/10000</span>
            <button class="sender-btn" :class="{ stop: sending }" :disabled="!sending && !input.trim() && !files.length" @click="sending ? (input = '') : send()">
              <svg v-if="sending" width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><rect x="3" y="3" width="10" height="10" rx="1" /></svg>
              <svg v-else width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><path d="M1.5 2L2 7l6 1-6 1-.5 5L15 8 1.5 2Z" /></svg>
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
@use '@/styles/variables.scss' as *;

.chat-page {
  flex: 1;
  display: flex;
  flex-direction: column;
  height: 100%;
  max-width: 900px;
  width: 100%;
  margin: 0 auto;
  position: relative;
}

// ──── New chat button ────
.new-chat-btn {
  position: fixed;
  top: 76px;
  right: 24px;
  width: 40px;
  height: 40px;
  border-radius: $radius-lg;
  background: $bg-elevated;
  border: 1px solid $border-default;
  color: $accent-cyan;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  z-index: 50;

  &:hover {
    background: $accent-cyan-dim;
    border-color: $accent-cyan;
    transform: scale(1.05);
  }
}

// ──── New chat overlay ────
.new-chat-overlay {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(13, 15, 18, 0.85);
  backdrop-filter: blur(12px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 50;
}

.new-chat-panel {
  @include glass-panel;
  border-radius: $radius-xl;
  padding: 32px 40px;
  text-align: center;
  max-width: 320px;
  animation: fade-up 0.3s ease-out;
}

.new-chat-title {
  font-size: $font-size-xl;
  font-weight: 600;
  color: $text-primary;
  margin: 0 0 8px 0;
}

.new-chat-sub {
  font-size: $font-size-sm;
  color: $text-secondary;
  margin: 0 0 24px 0;
}

.new-chat-actions {
  display: flex;
  gap: 12px;
  justify-content: center;
}

// ──── Session banner ────
.session-banner {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 20px;
  background: $accent-amber-dim;
  border-bottom: 1px solid rgba(255, 159, 67, 0.2);
  font-size: $font-size-sm;
  color: $accent-amber;
  flex-shrink: 0;

  .banner-content {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .banner-divider {
    opacity: 0.5;
  }

  strong {
    font-weight: 600;
    color: $text-primary;
  }
}

.back-btn {
  background: $accent-amber;
  color: $bg-deep;
  border: none;
  padding: 6px 14px;
  border-radius: $radius-md;
  font-size: $font-size-sm;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;

  &:hover {
    filter: brightness(1.15);
  }
}

// ──── Messages area ────
.chat-messages {
  flex: 1;
  overflow-y: auto;
  padding: 24px 32px;
  background: transparent;
}

.chat-empty {
  text-align: center;
  margin-top: 15vh;
  animation: fade-up 0.5s ease-out;
}

.empty-title {
  font-size: $font-size-2xl;
  color: $text-primary;
  font-weight: 500;
  margin: 0 0 8px 0;
}

.empty-subtitle {
  font-size: $font-size-base;
  color: $text-secondary;
  margin: 0;
}

// ──── Loading ────
.chat-loading {
  display: flex;
  gap: 12px;
  margin-bottom: 20px;
  animation: fade-up 0.3s ease-out;
}

.loading-avatar {
  width: 36px;
  height: 36px;
  border-radius: $radius-md;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  background: $bg-elevated;
  color: $accent-cyan;
  border: 1px solid $border-default;
}

.loading-bubble {
  @include glass-panel;
  border-radius: $radius-lg;
  padding: 12px 18px;
  display: flex;
  gap: 6px;
  align-items: center;
}

.loading-dot {
  width: 8px;
  height: 8px;
  background: $accent-cyan;
  border-radius: 50%;
  animation: bounce-dot 1.4s ease-in-out infinite;

  &:nth-child(1) { animation-delay: 0s; }
  &:nth-child(2) { animation-delay: 0.2s; }
  &:nth-child(3) { animation-delay: 0.4s; }
}

@keyframes bounce-dot {
  0%, 60%, 100% {
    transform: translateY(0);
    opacity: 0.6;
  }
  30% {
    transform: translateY(-8px);
    opacity: 1;
  }
}

// ──── Files bar ────
.files-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 12px;
}

.file-tag {
  display: flex;
  align-items: center;
  gap: 8px;
  background: $bg-elevated;
  border: 1px solid $border-default;
  border-radius: $radius-md;
  padding: 8px 12px;
  font-size: $font-size-sm;
  color: $text-secondary;
  animation: fade-up 0.2s ease-out;

  .file-thumb {
    width: 32px;
    height: 32px;
    object-fit: cover;
    border-radius: $radius-sm;
  }

  .file-icon-preview {
    font-size: 20px;
  }

  .file-name {
    max-width: 140px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: $text-primary;
  }

  .file-size {
    color: $text-muted;
    font-family: $font-display;
    font-size: $font-size-xs;
  }
}

.file-remove {
  background: none;
  border: none;
  cursor: pointer;
  font-size: 18px;
  line-height: 1;
  color: $text-muted;
  padding: 0 4px;
  transition: color 0.2s;

  &:hover { color: $accent-rose; }
  &:disabled { opacity: 0.4; cursor: not-allowed; }
}

// ──── Input area ────
.chat-input-area {
  padding: 20px 24px 24px;
}

.sender-wrapper {
  @include glass-panel;
  border-radius: $radius-xl;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);

  &:focus-within {
    border-color: rgba(0, 212, 255, 0.4);
  }
}

.sender-content {
  padding: 14px 18px 0;
}

.sender-input {
  width: 100%;
  border: none;
  outline: none;
  resize: none;
  font-size: $font-size-base;
  line-height: 1.6;
  font-family: $font-ui;
  color: $text-primary;
  background: transparent;

  &::placeholder {
    color: $text-muted;
  }

  &:disabled {
    cursor: not-allowed;
    opacity: 0.6;
  }
}

.sender-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px 10px 18px;
}

.sender-left {
  display: flex;
  align-items: center;
  gap: 6px;
}

.sender-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.sender-icon-btn {
  width: 36px;
  height: 36px;
  border: none;
  border-radius: $radius-md;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: all 0.2s;
  color: $text-secondary;
  background: transparent;

  &:hover:not(:disabled) {
    background: $bg-glass-light;
    color: $text-primary;
  }

  &:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
}

.sender-counter {
  font-family: $font-display;
  font-size: $font-size-xs;
  color: $text-muted;
  user-select: none;
}

.sender-btn {
  width: 40px;
  height: 40px;
  border: none;
  border-radius: $radius-md;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  color: $bg-deep;
  background: $accent-cyan;

  &:hover:not(:disabled) {
    filter: brightness(1.15);
  }

  &:disabled {
    background: $bg-elevated;
    color: $text-muted;
    cursor: not-allowed;
  }

  &.stop {
    background: $accent-amber;
  }
}
</style>