import axios from 'axios'

// 开发时 Vite 代理 /api → localhost:8080，生产时间源部署
// 桌面模式用 WailsAdapter（不走 HTTP）
const http = axios.create({ baseURL: '/api/v1', timeout: 120000 })

export class HttpAdapter {
  // 对话（SSE 流式）
  async *sendMessage(sessionId, content, agent) {
    const resp = await fetch('/api/v1/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ session: sessionId, content, agent, stream: true })
    })
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`)

    const reader = resp.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })

      // SSE 格式: "event: <type>\ndata: <json>\n\n"
      const parts = buffer.split('\n\n')
      buffer = parts.pop() // 保留未完成的部分
      for (const part of parts) {
        const dataLine = part.split('\n').find(l => l.startsWith('data: '))
        const eventLine = part.split('\n').find(l => l.startsWith('event: '))
        if (!dataLine || eventLine === 'event: start' || eventLine === 'event: done') continue

        const json = dataLine.slice(6)
        try {
          const obj = JSON.parse(json)
          if (obj.content) yield obj.content
        } catch {
          // plain text fallback
          yield json
        }
      }
    }
  }

  // 获取历史消息
  async getChatHistory(sessionId, agent) {
    try {
      // HTTP 模式下 gateway 会加 "webhook:" 前缀，需要传完整格式
      const fullSessionId = `webhook:${sessionId}`
      const params = agent ? { agent } : {}
      const { data } = await http.get(`/chat/history/${encodeURIComponent(fullSessionId)}`, { params })
      return data
    } catch {
      return []
    }
  }

  // Agent
  getAgents() { return http.get('/agents').then(r => r.data) }
  updateAgent(name, config) { return http.put(`/agents/${name}`, config).then(r => r.data) }
  deleteAgent(name) { return http.delete(`/agents/${name}`).then(r => r.data) }

  // 渠道
  getChannels() { return http.get('/channels').then(r => r.data) }
  updateChannel(name, config) { return http.put(`/channels/${name}`, config).then(r => r.data) }

  // 会话
  getSessions() { return http.get('/sessions').then(r => r.data) }
  deleteSession(id) { return http.delete(`/sessions/${id}`).then(r => r.data) }

  // 定时任务
  getCronJobs() { return http.get('/cron/jobs').then(r => r.data) }
  addCronJob(job) { return http.post('/cron/jobs', job).then(r => r.data) }
  updateCronJob(id, job) { return http.put(`/cron/jobs/${id}`, job).then(r => r.data) }
  deleteCronJob(id) { return http.delete(`/cron/jobs/${id}`).then(r => r.data) }
  runCronJob(id) { return http.post(`/cron/jobs/${id}/run`).then(r => r.data) }

  // 供应商/模型
  getProviders() { return http.get('/providers').then(r => r.data) }

  // 工具/Skills
  getTools() { return http.get('/tools').then(r => r.data) }
  getSkills() { return http.get('/skills').then(r => r.data) }

  // 配置
  getConfig() { return http.get('/config').then(r => r.data) }
  saveConfig(config) { return http.put('/config', config).then(r => r.data) }

  // 日志
  getLogs(params) { return http.get('/logs', { params }).then(r => r.data) }

  // 状态
  getStatus() { return http.get('/status').then(r => r.data) }
}

export default new HttpAdapter()
