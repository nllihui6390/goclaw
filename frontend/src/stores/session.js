import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useSessionStore = defineStore('session', () => {
  // 按 agent 分存储会话 ID：localStorage key = goclaw_session_{agent}
  const sessionId = ref('')
  const currentAgent = ref('')
  const initialized = ref({}) // 按 agent 记录是否已初始化

  /** 获取指定 agent 的会话 ID */
  function getStoredId(agent) {
    return localStorage.getItem(`goclaw_session_${agent}`) || ''
  }

  /** 保存指定 agent 的会话 ID */
  function saveId(agent, id) {
    localStorage.setItem(`goclaw_session_${agent}`, id)
  }

  /** 初始化会话：优先从后端获取最新 session，降级使用 localStorage */
  async function initSession(api, agentName) {
    currentAgent.value = agentName || 'default'

    // 已初始化过该 agent，直接返回缓存的 ID
    if (initialized.value[currentAgent.value]) {
      sessionId.value = getStoredId(currentAgent.value)
      return sessionId.value
    }

    // 优先从后端获取最新 session（实现跨设备同步）
    try {
      const sessionID = await api.getLatestSession(currentAgent.value)
      if (sessionID) {
        sessionId.value = sessionID
        saveId(currentAgent.value, sessionID)
        initialized.value[currentAgent.value] = true
        return sessionID
      }
    } catch (e) {
      console.warn('[SessionStore] getLatestSession failed, fallback to localStorage:', e)
    }

    // 降级：尝试使用 localStorage 中的缓存
    const saved = getStoredId(currentAgent.value)
    if (saved) {
      sessionId.value = saved
      initialized.value[currentAgent.value] = true
      return saved
    }

    // 最后降级：创建新 session
    try {
      const data = await api.createSession(currentAgent.value)
      sessionId.value = data.session_id
      saveId(currentAgent.value, sessionId.value)
      initialized.value[currentAgent.value] = true
    } catch {
      // 最极端降级：前端生成 UUID
      sessionId.value = 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, c => {
        const r = Math.random() * 16 | 0
        return (c === 'x' ? r : (r & 0x3 | 0x8)).toString(16)
      })
      saveId(currentAgent.value, sessionId.value)
      initialized.value[currentAgent.value] = true
    }
    return sessionId.value
  }

  /** 切换 agent：返回该 agent 的会话 ID，如果没有则创建 */
  async function switchAgent(api, agentName) {
    currentAgent.value = agentName || 'default'
    sessionId.value = getStoredId(currentAgent.value)

    if (!sessionId.value || !initialized.value[currentAgent.value]) {
      return initSession(api, currentAgent.value)
    }
    return sessionId.value
  }

  /** 新会话（清空当前 agent 的 ID） */
  function newSession() {
    if (currentAgent.value) {
      localStorage.removeItem(`goclaw_session_${currentAgent.value}`)
    }
    sessionId.value = ''
    initialized.value[currentAgent.value] = false
  }

  return { sessionId, currentAgent, initialized, initSession, switchAgent, newSession, saveId }
})