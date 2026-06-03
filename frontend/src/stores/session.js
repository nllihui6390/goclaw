import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useSessionStore = defineStore('session', () => {
  // 从 localStorage 恢复已存在的会话 ID（后端生成的 UUID）
  const saved = localStorage.getItem('goclaw_session_id')
  const sessionId = ref(saved || '')
  const initialized = ref(false)

  /** 初始化会话：优先复用已保存的 UUID，首次才从后端获取 */
  async function initSession(api, agentName) {
    if (initialized.value) return sessionId.value
    initialized.value = true

    // 已有已保存的有效 UUID，直接复用
    if (sessionId.value) return sessionId.value

    // 首次启动：从后端获取新 UUID
    try {
      const data = await api.createSession(agentName)
      sessionId.value = data.session_id
      localStorage.setItem('goclaw_session_id', sessionId.value)
    } catch {
      // 降级：前端生成
      sessionId.value = 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, c => {
        const r = Math.random() * 16 | 0
        return (c === 'x' ? r : (r & 0x3 | 0x8)).toString(16)
      })
      localStorage.setItem('goclaw_session_id', sessionId.value)
    }
    return sessionId.value
  }

  /** 新会话 */
  function newSession() {
    sessionId.value = ''
    localStorage.removeItem('goclaw_session_id')
    initialized.value = false
  }

  return { sessionId, initialized, initSession, newSession }
})
