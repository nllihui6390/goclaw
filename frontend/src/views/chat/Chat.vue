<script setup>
import { ref, inject, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import ChatMessage from '@/components/chat/ChatMessage.vue'

const api = inject('api')
const messages = ref([])
const input = ref('')
const sending = ref(false)
const abortCtrl = ref(null)

const sessionId = 'web-' + Date.now()

async function send() {
  const text = input.value.trim()
  if (!text || sending.value) return
  input.value = ''

  messages.value.push({ role: 'user', content: text })
  messages.value.push({ role: 'assistant', content: '', thinking: [] })
  sending.value = true

  try {
    let fullContent = ''
    for await (const chunk of api.sendMessage(sessionId, text)) {
      fullContent += chunk
      messages.value[messages.value.length - 1].content = fullContent
      await nextTick()
      scrollBottom()
    }
  } catch (e) {
    messages.value[messages.value.length - 1].content = '请求失败: ' + e.message
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
.chat-input-area {
  padding: 16px 24px;
  background: #fff;
  border-top: 1px solid #eee;
  display: flex;
  gap: 12px;
  align-items: flex-end;
}
</style>
