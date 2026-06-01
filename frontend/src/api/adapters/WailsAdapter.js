// WailsAdapter — 桌面模式通过 Wails3 bridge 直接调用 Go 函数，无 HTTP

export class WailsAdapter {
  // 对话（流式 — Go chan → JS AsyncGenerator）
  async *sendMessage(sessionId, content, agent) {
    try {
      const service = window.go?.main?.ChatService
      if (!service) throw new Error('ChatService not available')

      // Go 的 chan string 被 Wails3 转为 AsyncGenerator
      const stream = await service.SendMessage(sessionId, content, agent || '')
      for await (const chunk of stream) {
        yield chunk
      }
    } catch (e) {
      // Fallback: 非流式
      yield await window.go.main.ChatService.SendMessageSync(sessionId, content, agent || '')
    }
  }

  // 配置
  async getConfig() {
    const json = await window.go.main.AppService.GetConfig()
    return JSON.parse(json)
  }
  async saveConfig(config) {
    return window.go.main.AppService.SaveConfig(JSON.stringify(config))
  }

  // 渠道
  async getChannels() {
    const json = await window.go.main.AppService.GetChannels()
    return JSON.parse(json)
  }

  // 日志
  async getLogs() {
    return window.go.main.AppService.GetLogs()
  }

  // 状态
  async getStatus() {
    const json = await window.go.main.AppService.GetStatus()
    return JSON.parse(json)
  }

  // 以下当前仅桌面模式 AppService 提供
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
}
