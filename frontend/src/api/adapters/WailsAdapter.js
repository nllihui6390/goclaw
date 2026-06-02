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
  async getChatHistory(sessionId, agent) {
    try {
      const json = await Call.ByName('main.ChatService.GetChatHistory', sessionId, agent || '')
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

  // 定时任务（从 clawdata/cron_jobs.json 读取）
  async getCronJobs() {
    try {
      const json = await Call.ByName('main.AppService.GetCronJobs')
      return JSON.parse(json)
    } catch (e) {
      console.error('[WailsAdapter] getCronJobs error:', e)
      return []
    }
  }
  async addCronJob(job) {
    try {
      const json = await Call.ByName('main.AppService.SaveCronJob', JSON.stringify(job))
      return JSON.parse(json)
    } catch (e) {
      console.error('[WailsAdapter] addCronJob error:', e)
      return { status: 'created' }
    }
  }
  async updateCronJob(id, job) {
    return this.addCronJob(job)
  }
  async deleteCronJob(id) {
    try {
      await Call.ByName('main.AppService.DeleteCronJob', id)
      return { status: 'deleted' }
    } catch (e) {
      console.error('[WailsAdapter] deleteCronJob error:', e)
      return { status: 'deleted' }
    }
  }
  async runCronJob(id) {
    try {
      await Call.ByName('main.AppService.RunCronJob', id)
      return { status: 'executed' }
    } catch (e) {
      console.error('[WailsAdapter] runCronJob error:', e)
      return { status: 'executed' }
    }
  }

  // 获取定时任务启用状态（从 clawdata/cron_enabled.json 读取）
  async getCronEnabled() {
    try {
      const json = await Call.ByName('main.AppService.GetCronEnabled')
      return JSON.parse(json)
    } catch (e) {
      return true
    }
  }
  async setCronEnabled(enabled) {
    try {
      await Call.ByName('main.AppService.SetCronEnabled', String(enabled))
      return { status: 'ok' }
    } catch (e) {
      return { status: 'ok' }
    }
  }

  async getTools() { return [] }
  async getSkills() { return [] }
  async getProviders() { return [] }
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