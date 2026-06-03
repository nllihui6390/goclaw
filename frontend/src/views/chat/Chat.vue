<script setup>
import { ref, inject, nextTick, onMounted, watch, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAgentStore } from '@/stores/agent'
import ChatMessage from '@/components/chat/ChatMessage.vue'

const route = useRoute()
const router = useRouter()
const api = inject('api')
const agentStore = useAgentStore()
const messages = ref([])
const input = ref('')
const sending = ref(false)
const files = ref([])
const fileInput = ref(null)

// 默认桌面会话 ID
const defaultSessionId = 'desktop:local'

// 从 route.query 读取会话 ID，无参数时使用默认值
const sessionId = computed(() => route.query.session || defaultSessionId)

// 是否正在查看非默认会话（从会话管理跳转过来的）
const viewingSession = computed(() => !!route.query.session)

function onFileChange(e) {
  const selected = Array.from(e.target.files || [])
  files.value = [...files.value, ...selected]
  // 清空 input 以便重复选择同一文件
  e.target.value = ''
}

function removeFile(index) {
  files.value.splice(index, 1)
}

// 返回默认对话（清除 query 参数）
function backToDefault() {
  router.push('/')
}

// 加载历史记录（原子替换，避免先清空再加载的闪烁）
async function loadHistory() {
  // 正在发送消息时不重载，防止打断当前对话
  if (sending.value) return
  try {
    const history = await api.getChatHistory(sessionId.value, agentStore.selectedAgent)
    messages.value = (history && history.length > 0)? history.map(m => ({ role: m.role, content: m.content })): []
    await nextTick()
    scrollBottom()
  } catch (e) {
    console.log('[Chat] 加载历史失败:', e.message)
  }
}

// 初始化：如果 query 指定了 agent，先切换再加载
onMounted(() => {
  if (route.query.agent && route.query.agent !== agentStore.selectedAgent) {
    agentStore.setAgent(route.query.agent)
  }
  loadHistory()
})

// 切换 agent 时重新加载对应聊天记录（仅默认会话模式）
watch(() => agentStore.selectedAgent, () => {
  if (!viewingSession.value) loadHistory()
})

// 从会话管理跳转过来时（query 变化），切换 agent 并重新加载
watch(() => route.query, (q, oldQ) => {
  // session 变化时重新加载（包括从有 session 变为无 session，即返回默认对话）
  if (q.session !== oldQ?.session) {
    if (q.agent && q.agent !== agentStore.selectedAgent) {
      agentStore.setAgent(q.agent)
    }
    loadHistory()
  }
})

async function send() {
  const text = input.value.trim()
  if (!text || sending.value) return
  input.value = ''

  messages.value.push({ role: 'user', content: text })
  await nextTick()
  setTimeout(scrollBottom, 50)
  sending.value = true

  try {
    if (api.isStreaming) {
      // SSE 流式模式（HttpAdapter）：逐步接收 chunk，渐进式渲染
      let fullContent = ''
      for await (const chunk of api.sendMessage(sessionId.value, text, agentStore.selectedAgent)) {
        fullContent += chunk
        if (messages.value[messages.value.length - 1].role !== 'assistant') {
          messages.value.push({ role: 'assistant', content: fullContent })
        } else {
          messages.value[messages.value.length - 1].content = fullContent
        }
        await nextTick()
        scrollBottom()
      }
    } else {
      // 非流式模式（WailsAdapter）：一次性返回完整响应
      const content = await api.sendMessage(sessionId.value, text, agentStore.selectedAgent)
      messages.value.push({ role: 'assistant', content })
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
</script>

<template>
  <div class="chat-page">
    <!-- 非默认会话提示条：从会话管理跳转过来时显示 -->
    <div v-if="viewingSession" class="session-banner">
      <span>正在查看会话：<strong>{{ sessionId }}</strong>（Agent: {{ route.query.agent || agentStore.selectedAgent }}）</span>
      <button class="back-btn" @click="backToDefault">返回默认对话</button>
    </div>
    <div class="chat-messages">
      <ChatMessage
        v-for="(msg, i) in messages"
        :key="i"
        :role="msg.role"
        :content="msg.content"
      />
      <!-- 等待响应时显示加载动画（最后一条不是assistant时表示还在等待） -->
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
      <div v-if="messages.length === 0" class="chat-empty">
        <h2>go-claw AI Agent</h2>
        <p>在下方输入消息开始对话</p>
      </div>
    </div>
    <div class="chat-input-area">
      <!-- 已选文件列表 -->
      <div v-if="files.length" class="files-bar">
        <div v-for="(f, i) in files" :key="i" class="file-tag">
          <el-icon><Document /></el-icon>
          <span class="file-name">{{ f.name }}</span>
          <span class="file-size">{{ formatSize(f.size) }}</span>
          <button class="file-remove" @click="removeFile(i)" :disabled="sending">&times;</button>
        </div>
      </div>
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
              <svg width="16" height="16" viewBox="0 0 1024 1024" fill="currentColor">
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
.chat-page {
  flex: 1;
  display: flex;
  flex-direction: column;
  height: 100%;
  max-width: 900px;
  width: 100%;
  margin: 0 auto;
}
.session-banner {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 16px;
  background: #fdf6ec;
  border-bottom: 1px solid #f5dab1;
  font-size: 13px;
  color: #e6a23c;
  flex-shrink: 0;
  strong { font-weight: 600; }
}
.back-btn {
  background: #e6a23c;
  color: #fff;
  border: none;
  padding: 4px 12px;
  border-radius: 4px;
  font-size: 12px;
  cursor: pointer;
  &:hover { background: #cf9236; }
}
.chat-messages {
  flex: 1;
  overflow-y: auto;
  padding: 24px 32px;
  background: #f5f6f8;
}
.chat-empty {
  text-align: center;
  margin-top: 20vh;
  h2 { color: #999; font-weight: 400; }
  p { color: #bbb; margin-top: 8px; }
}
.chat-loading {
  display: flex;
  gap: 12px;
  margin-bottom: 20px;
}
.loading-avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  background: #409eff;
  color: #fff;
}
.loading-bubble {
  background: #fff;
  border-radius: 12px;
  padding: 10px 16px;
  display: flex;
  gap: 4px;
  align-items: center;
  box-shadow: 0 1px 3px rgba(0,0,0,.06);
}
.loading-dot {
  width: 6px;
  height: 6px;
  background: #409eff;
  border-radius: 50%;
  animation: dotBounce 1.4s ease-in-out infinite both;
  &:nth-child(1) { animation-delay: 0s; }
  &:nth-child(2) { animation-delay: 0.2s; }
  &:nth-child(3) { animation-delay: 0.4s; }
}
@keyframes dotBounce {
  0%, 80%, 100% { transform: scale(0.4); opacity: 0.3; }
  40% { transform: scale(1); opacity: 1; }
}
.files-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 10px;
}
.file-tag {
  display: flex;
  align-items: center;
  gap: 6px;
  background: #f0f2f5;
  border-radius: 6px;
  padding: 4px 8px;
  font-size: 12px;
  color: #606266;
  .file-name { max-width: 160px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .file-size { color: #bbb; }
}
.file-remove {
  background: none;
  border: none;
  cursor: pointer;
  font-size: 16px;
  line-height: 1;
  color: #999;
  padding: 0 2px;
  &:hover { color: #f56c6c; }
}
.chat-input-area {
  padding: 16px 24px 20px;
}
.sender-wrapper {
  border: 1px solid #d0d5dd;
  border-radius: 12px;
  background: #fff;
  transition: border-color .2s;
  &:focus-within {
    border-color: #409eff;
    box-shadow: 0 0 0 3px rgba(64, 158, 255, .12);
  }
}
.sender-content {
  padding: 10px 14px 0;
}
.sender-input {
  width: 100%;
  border: none;
  outline: none;
  resize: none;
  font-size: 14px;
  line-height: 1.6;
  font-family: inherit;
  color: #303133;
  background: transparent;
  &::placeholder { color: #bbb; }
  &:disabled { cursor: not-allowed; opacity: .6; }
}
.sender-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 10px 6px 14px;
}
.sender-left {
  display: flex;
  align-items: center;
  gap: 4px;
}
.sender-right {
  display: flex;
  align-items: center;
  gap: 10px;
}
.sender-icon-btn {
  width: 30px;
  height: 30px;
  border: none;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: all .15s;
  color: #909399;
  background: transparent;
  &:hover:not(:disabled) { background: #f0f2f5; color: #606266; }
  &:disabled { opacity: .4; cursor: not-allowed; }
}
.sender-counter {
  font-size: 12px;
  color: #bbb;
  user-select: none;
}
.sender-btn {
  width: 32px;
  height: 32px;
  border: none;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: all .2s;
  color: #fff;
  background: #409eff;
  &:hover:not(:disabled) { background: #337ecc; }
  &:disabled { background: #d0d5dd; cursor: not-allowed; }
  &.stop { background: #e6a23c; }
}
</style>
