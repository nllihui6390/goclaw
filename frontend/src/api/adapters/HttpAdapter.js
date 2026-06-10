import axios from 'axios'

// 开发时 Vite 代理 /api → localhost:8080，生产时间源部署
// 桌面模式用 WailsAdapter（不走 HTTP）
const http = axios.create({ baseURL: '/api/v1', timeout: 120000 })

export class HttpAdapter {
  // 流式适配器标志
  isStreaming = true

  // 创建新会话
  createSession(agent) { return http.post('/chat/session', { agent }).then(r => r.data) }

  // 对话（SSE 流式）
  // yield { type: 'text', content } 或 { type: 'file', info: {...} }
  async *sendMessage(sessionId, content, agent) {
    // HTTP 模式下，gateway 会拼接 "webhook:" + session 作为完整 sessionID
    // 如果 sessionId 已含频道前缀（如 "webhook:xxx"），需剥离前缀避免重复拼接
    let chatSession = sessionId
    // if (sessionId.startsWith('webhook:')) {
    //   chatSession = sessionId.slice(8) // 去掉 "webhook:" 前缀
    // }
    const resp = await fetch('/api/v1/chat', { method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ session: chatSession, content, agent, stream: true })
    })
    if (!resp.ok) {
      let msg = `HTTP ${resp.status}`
      try { const e = JSON.parse(await resp.text()); if (e.error) msg = e.error } catch {}
      throw new Error(msg)
    }

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
          // 文件事件
          if (eventLine === 'event: file') {
            yield { type: 'file', info: obj }
          } else if (eventLine === 'event: content') {
            yield { type: 'content', blocks: obj.blocks }
          } else if (obj.content) {
            yield { type: 'text', content: obj.content }
          }
        } catch {
          // plain text fallback
          yield { type: 'text', content: json }
        }
      }
    }
  }

  // 获取历史消息
  async getChatHistory(sessionId, agent) {
    try {
      // 旧格式有频道前缀（webhook:, wecom: 等）；UUID 格式不需要加前缀
      const channelPrefixes = ['wecom:', 'dingtalk:', 'lark:', 'cron:']
      const hasChannelPrefix = channelPrefixes.some(p => sessionId.startsWith(p))
      const fullSessionId = hasChannelPrefix ? sessionId : sessionId
      const params = agent ? { agent } : {}
      const { data } = await http.get(`/chat/history/${encodeURIComponent(fullSessionId)}`, { params })
      return data
    } catch {
      return []
    }
  }

  // Agent
  getAgents() { return http.get('/agents').then(r => r.data) }
  createAgent(config) { return http.post('/agents/create', config).then(r => r.data) }
  updateAgent(name, config) { return http.put(`/agents/update/${name}`, config).then(r => r.data) }
  deleteAgent(name) { return http.delete(`/agents/delete/${name}`).then(r => r.data) }

  // 频道 (per-agent)
  getChannels(agent) { return http.get('/channels', { params: { agent } }).then(r => r.data) }
  updateChannel(agent, name, config) { return http.put(`/channels/${name}`, config, { params: { agent } }).then(r => r.data) }
  getChannelQRCode(channel) { return http.get('/channels/qrcode', { params: { channel } }).then(r => r.data) }
  getChannelQRCodeStatus(channel, token) { return http.get('/channels/qrcode/status', { params: { channel, token } }).then(r => r.data) }

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
  testProvider(provider, model) { return http.post('/providers/test', { provider, model: model || '' }).then(r => r.data) }
  testAllModels(provider) { return http.post('/providers/test-all', { provider }).then(r => r.data) }

  // 工具/Skills
  getTools() { return http.get('/tools').then(r => r.data) }
  getSkillPool() { return http.get('/skills/pool').then(r => r.data) }
  scanSkills() { return http.post('/skills/scan').then(r => r.data) }
  uploadSkill(file) {
    const formData = new FormData()
    formData.append('file', file)
    return fetch('/api/v1/skills/upload', {
      method: 'POST',
      body: formData
    }).then(r => r.json())
  }
  // 上传聊天文件
  uploadChatFile(sessionId, file) {
    const formData = new FormData()
    formData.append('session', sessionId)
    formData.append('file', file)
    return fetch('/api/v1/chat/files/upload', {
      method: 'POST',
      body: formData
    }).then(r => r.json())
  }
  getEnabledSkills(agent) { return http.get('/skills/enabled', { params: { agent } }).then(r => r.data) }
  setEnabledSkills(agent, skills) { return http.put('/skills/enabled', skills, { params: { agent } }).then(r => r.data) }

  // 配置
  getConfig() { return http.get('/config').then(r => r.data) }
  saveConfig(config) { return http.put('/config', config).then(r => r.data) }

  // 日志
  getLogs(params) { return http.get('/logs', { params }).then(r => r.data) }

  // Agent 文件管理
  getAgentFiles(agent) { return http.get(`/agent-files/${encodeURIComponent(agent)}`).then(r => r.data) }
  readAgentFile(agent, file) {
    return http.get(`/agent-files/${encodeURIComponent(agent)}/${encodeURIComponent(file)}`, { responseType: 'text' }).then(r => r.data)
  }
  writeAgentFile(agent, file, content) {
    return http.put(`/agent-files/${encodeURIComponent(agent)}/${encodeURIComponent(file)}`, { content }).then(r => r.data)
  }

  // 状态
  getStatus() { return http.get('/status').then(r => r.data) }

  // 重启
  restart() { return http.post('/restart').then(r => r.data) }

  // 安全审批
  getPendingApprovals() { return http.get('/security/approvals').then(r => r.data) }
  approveRequest(id) { return http.post('/security/approve', { approval_id: id }).then(r => r.data) }
  denyRequest(id, reason) { return http.post('/security/deny', { approval_id: id, reason }).then(r => r.data) }

  // 安全配置
  getSecurityConfig() { return http.get('/security/config').then(r => r.data) }
  updateSecurityConfig(config) { return http.put('/security/config', config).then(r => r.data) }
}

export default new HttpAdapter()
