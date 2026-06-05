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

  /** 初始化会话：按 agent 查找或创建 */
  async function initSession(api, agentName) {
    currentAgent.value = agentName || 'default'

    // 已初始化过该 agent，直接返回缓存的 ID
    if (initialized.value[currentAgent.value]) {
      sessionId.value = getStoredId(currentAgent.value)
      return sessionId.value
    }

    // 尝试复用已保存的 ID
    const saved = getStoredId(currentAgent.value)
    if (saved) {
      sessionId.value = saved
      initialized.value[currentAgent.value] = true
      return sessionId.value
    }

    // 首次启动：从后端获取新 UUID
    try {
      const data = await api.createSession(currentAgent.value)
      sessionId.value = data.session_id
      saveId(currentAgent.value, sessionId.value)
      initialized.value[currentAgent.value] = true
    } catch {
      // 降级：前端生成
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