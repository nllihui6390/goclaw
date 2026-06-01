<script setup>
import { computed } from 'vue'
import { marked } from 'marked'

marked.setOptions({ breaks: true, gfm: true })

const props = defineProps({
  role: String,
  content: String
})

const rendered = computed(() => {
  return props.content ? marked(props.content) : ''
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
      <div v-if="role === 'assistant'" class="chat-markdown" v-html="rendered" />
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
</style>
