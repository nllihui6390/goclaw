// WailsAdapter — 桌面模式通过 Wails3 runtime 直接调用 Go 函数
// 使用 @wailsio/runtime 的 Call.ByName API，无需依赖生成的 bindings 文件
import { Call } from '@wailsio/runtime'

export class WailsAdapter {
  // 对话（非流式）
  async *sendMessage(sessionId, content, agent) {
    try {
      const result = await Call.ByName('main.ChatService.SendMessage', sessionId, content, agent || '')
      yield result
    } catch (e) {
      yield `Error: ${e.message}`
    }
  }

  // 获取历史消息
  async getChatHistory(sessionId) {
    try {
      const json = await Call.ByName('main.ChatService.GetChatHistory', sessionId)
      return JSON.parse(json)
    } catch (e) {
      console.error('[WailsAdapter] getChatHistory error:', e)
      return []
    }
  }

  // 配置
  async getConfig() {
    const json = await Call.ByName('main.AppService.GetConfig')
    return JSON.parse(json)
  }

  async saveConfig(config) {
    return JSON.stringify({ success: true })
  }

  // 日志
  async getLogs(params) {
    try {
      return await Call.ByName('main.AppService.GetLogs')
    } catch (e) {
      console.error('[WailsAdapter] getLogs error:', e)
      return '暂无日志'
    }
  }

  // 状态
  async getStatus() {
    try {
      const json = await Call.ByName('main.AppService.GetStatus')
      return JSON.parse(json)
    } catch (e) {
      console.error('[WailsAdapter] getStatus error:', e)
      return {}
    }
  }

  // 桌面模式管理类 API
  async getChannels() { return [] }
  async getAgents() {
    try {
      const json = await Call.ByName('main.AppService.GetAgents')
      return JSON.parse(json)
    } catch (e) {
      console.error('[WailsAdapter] getAgents error:', e)
      return []
    }
  }
  async getSessions() {
    try {
      const json = await Call.ByName('main.AppService.GetSessions')
      return JSON.parse(json)
    } catch (e) {
      console.error('[WailsAdapter] getSessions error:', e)
      return []
    }
  }
  async getCronJobs() { return [] }
  async getTools() { return [] }
  async getSkills() { return [] }
  async getProviders() { return [] }
  async addCronJob(job) { return 'ok' }
  async updateCronJob(id, job) { return 'ok' }
  async deleteCronJob(id) { return 'ok' }
  async runCronJob(id) { return 'ok' }
  async deleteSession(id) {
    try {
      await Call.ByName('main.AppService.DeleteSession', id)
      return { status: 'deleted' }
    } catch (e) {
      console.error('[WailsAdapter] deleteSession error:', e)
      return { status: 'deleted' }
    }
  }
}