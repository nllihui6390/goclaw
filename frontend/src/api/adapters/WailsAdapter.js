// WailsAdapter — 桌面模式通过 Wails3 bridge 直接调用 Go 函数，无 HTTP
// 使用 @vite-ignore 动态 import，dev 模式绑定文件不存在时不会导致构建失败

let SendMessageFn, GetChatHistoryFn, GetConfigFn

async function loadBindings() {
  const chat = await import(/* @vite-ignore */ '../../../bindings/go-claw/chatservice.js')
  const app = await import(/* @vite-ignore */ '../../../bindings/go-claw/appservice.js')
  SendMessageFn = chat.SendMessage
  GetChatHistoryFn = chat.GetChatHistory
  GetConfigFn = app.GetConfig
}

export class WailsAdapter {
  // 对话（非流式 — 返回完整响应一次性显示）
  async *sendMessage(sessionId, content, agent) {
    try {
      if (!SendMessageFn) await loadBindings()
      const result = await SendMessageFn(sessionId, content, agent || '')
      yield result
    } catch (e) {
      yield `Error: ${e.message}`
    }
  }

  // 获取历史消息
  async getChatHistory(sessionId) {
    try {
      if (!GetChatHistoryFn) await loadBindings()
      const json = await GetChatHistoryFn(sessionId)
      return JSON.parse(json)
    } catch (e) {
      console.error('[WailsAdapter] getChatHistory error:', e)
      return []
    }
  }

  // 配置
  async getConfig() {
    if (!GetConfigFn) await loadBindings()
    const json = await GetConfigFn()
    return JSON.parse(json)
  }

  async saveConfig(config) {
    return JSON.stringify({ success: true })
  }

  // 桌面模式暂不支持管理类 API
  async getChannels() { return [] }
  async getAgents() { return [] }
  async getSessions() { return [] }
  async getCronJobs() { return [] }
  async getTools() { return [] }
  async getSkills() { return [] }
  async getProviders() { return [] }
  async addCronJob(job) { return 'ok' }
  async updateCronJob(id, job) { return 'ok' }
  async deleteCronJob(id) { return 'ok' }
  async runCronJob(id) { return 'ok' }
  async deleteSession(id) { return 'ok' }
  async getLogs() { return '' }
  async getStatus() { return {} }
}