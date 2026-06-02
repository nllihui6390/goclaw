<script setup>
import { ref, inject, nextTick, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { useAgentStore } from '@/stores/agent'
import ChatMessage from '@/components/chat/ChatMessage.vue'

const api = inject('api')
const agentStore = useAgentStore()
const messages = ref([])
const input = ref('')
const sending = ref(false)
const abortCtrl = ref(null)

const sessionId = 'desktop:local'

// 加载历史记录
onMounted(async () => {
  try {
    const history = await api.getChatHistory(sessionId)
    if (history && history.length > 0) {
      messages.value = history.map(m => ({
        role: m.role,
        content: m.content
      }))
      scrollBottom()
    }
  } catch (e) {
    console.log('[Chat] 加载历史失败:', e.message)
  }
})

async function send() {
  const text = input.value.trim()
  if (!text || sending.value) return
  input.value = ''

  messages.value.push({ role: 'user', content: text })
  sending.value = true

  try {
    let fullContent = ''
    for await (const chunk of api.sendMessage(sessionId, text, agentStore.selectedAgent)) {
      fullContent += chunk
      // 第一次收到内容时 push assistant 消息
      if (messages.value[messages.value.length - 1].role !== 'assistant') {
        messages.value.push({ role: 'assistant', content: fullContent })
      } else {
        messages.value[messages.value.length - 1].content = fullContent
      }
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

function onKeydown(e) {
  if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send() }
}
</script>

<template>
  <div class="chat-page">
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
      <el-input
        v-model="input"
        type="textarea"
        :rows="2"
        placeholder="输入消息..."
        @keydown="onKeydown"
        :disabled="sending"
      />
      <el-button
        type="primary"
        :icon="sending ? 'Close' : 'Promotion'"
        @click="sending ? (input = '') : send()"
        :disabled="!sending && !input.trim()"
      >
        {{ sending ? '停止' : '发送' }}
      </el-button>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.chat-page {
  flex: 1;
  display: flex;
  flex-direction: column;
  height: 100%;
}
.chat-messages {
  flex: 1;
  overflow-y: auto;
  padding: 24px;
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
.chat-input-area {
  padding: 16px 24px;
  background: #fff;
  border-top: 1px solid #eee;
  display: flex;
  gap: 12px;
  align-items: flex-end;
}
</style>
