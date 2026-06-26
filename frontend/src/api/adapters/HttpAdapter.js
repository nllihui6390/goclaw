import axios from 'axios'

// 开发时 Vite 代理 /api → localhost:8080，生产时间源部署
// 桌面模式用 WailsAdapter（不走 HTTP）
const http = axios.create({ baseURL: '/api/v1', timeout: 120000 })

export class HttpAdapter {
  // 流式适配器标志
  isStreaming = true

  // 创建新会话
  createSession(agent) { return http.post('/chat/session', { agent }).then(r => r.data) }

  // 获取指定 agent 的最新 session ID
  getLatestSession(agent) {
    return http.get('/chat/session/latest', { params: { agent } }).then(r => r.data.session_id)
  }

  // 对话（SSE 流式）
  // yield { type: 'text', content } 或 { type: 'file', info: {...} }
  async *sendMessage(sessionId, content, agent, signal) {
    // 如果 sessionId 已含频道前缀（如 "webhook:xxx"），需剥离前缀避免重复拼接
    let chatSession = sessionId
    const resp = await fetch('/api/v1/chat', { method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ session: chatSession, content, agent, stream: true }),
      signal  // AbortSignal，用于停止按钮中断
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
      if (done) {
        // Stream 结束时，处理 buffer 中剩余的数据
        if (buffer.trim()) {
          const parts = buffer.split('\n\n').filter(p => p.trim())
          for (const part of parts) {
            const dataLine = part.split('\n').find(l => l.startsWith('data: '))
            const eventLine = part.split('\n').find(l => l.startsWith('event: '))
            if (!dataLine || eventLine === 'event: start' || eventLine === 'event: done') continue
            const json = dataLine.slice(6)
            try {
              const obj = JSON.parse(json)
              // chunk 事件：后端整段最终回复（当 text 事件未流式推送时）
              if (eventLine === 'event: chunk') {
                yield { type: 'text', text: obj.content || '' }
                continue
              }
              if (eventLine === 'event: file') {
                yield { type: 'file', info: obj }
              } else if (eventLine === 'event: content') {
                yield { type: 'content', blocks: obj.blocks }
              } else if (eventLine === 'event: text') {
                yield { type: 'text', text: obj.text }
              } else if (eventLine === 'event: thinking') {
                yield { type: 'thinking', content: obj.thinking }
              } else if (eventLine === 'event: tool_call') {
                yield { type: 'tool_call', tool_name: obj.tool_name, args: obj.args }
              } else if (eventLine === 'event: tool_result') {
                yield { type: 'tool_result', tool_name: obj.tool_name, result: obj.result }
              } else if (eventLine === 'event: tool_error') {
                yield { type: 'tool_error', tool_name: obj.tool_name, error: obj.error }
              } else if (eventLine === 'event: guard') {
                yield {
                  type: 'guard',
                  tool_name: obj.tool_name,
                  args: obj.args,
                  guard_message: obj.guard_message,
                  approval_id: obj.approval_id,
                  approval_state: obj.approval_state
                }
              } else if (obj.content) {
                yield { type: 'text', content: obj.content }
              }
            } catch {
              yield { type: 'text', content: json }
            }
          }
        }
        break
      }
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
          // chunk 事件：后端整段最终回复（当 text 事件未流式推送时）
          if (eventLine === 'event: chunk') {
            yield { type: 'text', text: obj.content || '' }
            continue
          }
          // 文件事件
          if (eventLine === 'event: file') {
            yield { type: 'file', info: obj }
          } else if (eventLine === 'event: content') {
            yield { type: 'content', blocks: obj.blocks }
          } else if (eventLine === 'event: text') {
            yield { type: 'text', text: obj.text }
          } else if (eventLine === 'event: thinking') {
            yield { type: 'thinking', content: obj.thinking }
          } else if (eventLine === 'event: tool_call') {
            yield { type: 'tool_call', tool_name: obj.tool_name, args: obj.args }
          } else if (eventLine === 'event: tool_result') {
            yield { type: 'tool_result', tool_name: obj.tool_name, result: obj.result }
          } else if (eventLine === 'event: tool_error') {
            yield { type: 'tool_error', tool_name: obj.tool_name, error: obj.error }
          } else if (eventLine === 'event: guard') {
            // 安全守卫事件（审批通知）
            yield {
              type: 'guard',
              tool_name: obj.tool_name,
              args: obj.args,
              guard_message: obj.guard_message,
              approval_id: obj.approval_id,
              approval_state: obj.approval_state
            }
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

  // 停止当前 session 的 agent 处理
  stopChat(sessionId) { return http.post('/chat/stop', { session: sessionId }).then(r => r.data) }

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
  getChannelQRCode(channel, params) { return http.get('/channels/qrcode', { params: { channel, ...params } }).then(r => r.data) }
  getChannelQRCodeStatus(channel, token, params) { return http.get('/channels/qrcode/status', { params: { channel, token, ...params } }).then(r => r.data) }

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

  // 环境变量
  getEnvVars() { return http.get('/getEnvVars').then(r => r.data) }
  createEnvVar(entry) { return http.post('/createEnvVars', entry).then(r => r.data) }
  updateEnvVar(entry) { return http.post('/updateEnvVars', entry).then(r => r.data) }
  deleteEnvVar(key) { return http.post('/deleteEnvVars', { key }).then(r => r.data) }
  reloadEnvVars() { return http.post('/reloadEnvVars').then(r => r.data) }

  // Token 使用量
  getTokenUsage(params) {
    const search = new URLSearchParams({ start_date: params.start_date, end_date: params.end_date })
    if (params.model) search.set('model', params.model)
    if (params.provider) search.set('provider', params.provider)
    return http.get(`/token-usage?${search.toString()}`).then(r => r.data)
  }
  getTokenUsageDetails(params) {
    const search = new URLSearchParams({ start_date: params.start_date, end_date: params.end_date })
    if (params.model) search.set('model', params.model)
    if (params.provider) search.set('provider', params.provider)
    return http.get(`/token-usage/details?${search.toString()}`).then(r => r.data)
  }

  // MCP 集成
  getMCPServers(agent) {
    return http.get('/mcp', { params: { agent } }).then(r => r.data)
  }
  createMCPServer(agent, config) {
    return http.post('/mcp/create', config, { params: { agent } }).then(r => r.data)
  }
  updateMCPServer(agent, name, config) {
    return http.post('/mcp/update', config, { params: { agent, name } }).then(r => r.data)
  }
  deleteMCPServer(agent, name) {
    return http.post('/mcp/delete', null, { params: { agent, name } }).then(r => r.data)
  }
  toggleMCPServer(agent, name) {
    return http.post('/mcp/toggle', null, { params: { agent, name } }).then(r => r.data)
  }
  getMCPServerTools(name) {
    return http.get('/mcp/tools', { params: { name } }).then(r => r.data)
  }
}

export default new HttpAdapter()
