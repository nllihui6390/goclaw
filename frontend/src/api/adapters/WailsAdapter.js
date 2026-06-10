// WailsAdapter — 桌面模式通过 Wails3 runtime 直接调用 Go 函数
// 使用 @wailsio/runtime 的 Call.ByName API，无需依赖生成的 bindings 文件
import { Call } from '@wailsio/runtime'

// readFileAsBase64 读取文件为 base64 字符串
function readFileAsBase64(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const dataUrl = reader.result
      const base64 = dataUrl.split(',')[1]
      resolve(base64)
    }
    reader.onerror = reject
    reader.readAsDataURL(file)
  })
}

export class WailsAdapter {
  // 非流式适配器标志，Chat.vue 根据此标志选择消费方式
  isStreaming = false

  // 对话（非流式 — 直接返回完整响应 Promise，不使用 async generator）
  async sendMessage(sessionId, content, agent) {
    try {
      const result = await Call.ByName('main.ChatService.SendMessage', sessionId, content, agent || '')
      return result
    } catch (e) {
      return `Error: ${e.message}`
    }
  }

  // 创建新会话（后端生成 UUID，按 agent 注册索引）
  async createSession(agent) {
    try {
      const json = await Call.ByName('main.ChatService.CreateSession', agent || 'default')
      return JSON.parse(json)
    } catch (e) {
      // 降级：返回客户端生成的 UUID
      const id = 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, c => {
        const r = Math.random() * 16 | 0
        return (c === 'x' ? r : (r & 0x3 | 0x8)).toString(16)
      })
      return { session_id: id }
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
    await Call.ByName('main.AppService.SaveConfig', JSON.stringify(config))
    return { status: 'saved' }
  }

  // Agent 管理
  async getAgents() {
    try {
      const json = await Call.ByName('main.AppService.GetAgents')
      return JSON.parse(json)
    } catch (e) {
      console.error('[WailsAdapter] getAgents error:', e)
      return []
    }
  }
  async createAgent(config) {
  const json = await Call.ByName('main.AppService.CreateAgent', JSON.stringify(config))
    return JSON.parse(json)
  }
  async updateAgent(name, config) {
    const json = await Call.ByName('main.AppService.UpdateAgent', name, JSON.stringify(config))
    return JSON.parse(json)
  }
  async deleteAgent(name) {
    const json = await Call.ByName('main.AppService.DeleteAgent', name)
    return JSON.parse(json)
  }

  // 频道管理 (per-agent)
  async getChannels(agent) {
    try {
      const json = await Call.ByName('main.AppService.GetChannels', agent || 'default')
      return JSON.parse(json)
    } catch (e) {
      console.error('[WailsAdapter] getChannels error:', e)
      return []
    }
  }
  async updateChannel(agent, name, config) {
    const json = await Call.ByName('main.AppService.UpdateChannel', agent || 'default', name, JSON.stringify(config))
    return JSON.parse(json)
  }
  async getChannelQRCode(channel) {
    try {
      const json = await Call.ByName('main.AppService.GetChannelQRCode', channel)
      return JSON.parse(json)
    } catch (e) {
      console.error('[WailsAdapter] getChannelQRCode error:', e)
      return { error: e.message }
    }
  }
  async getChannelQRCodeStatus(channel, token) {
    try {
      const json = await Call.ByName('main.AppService.GetChannelQRCodeStatus', channel, token)
      return JSON.parse(json)
    } catch (e) {
      console.error('[WailsAdapter] getChannelQRCodeStatus error:', e)
      return { error: e.message }
    }
  }

  // 供应商/模型
  async getProviders() {
    try {
      const json = await Call.ByName('main.AppService.GetProviders')
      const data = JSON.parse(json)
      // 转为数组格式统一处理
      if (data && !Array.isArray(data)) {
        return Object.keys(data).map(k => ({ name: k, ...data[k] }))
      }
      return Array.isArray(data) ? data : []
    } catch (e) {
      console.error('[WailsAdapter] getProviders error:', e)
      return []
    }
  }
  async testProvider(provider, model) {
    try {
      const json = await Call.ByName('main.AppService.TestProvider', provider, model || '')
      return JSON.parse(json)
    } catch (e) {
      return { success: false, error: e.message }
    }
  }
  async testAllModels(provider) {
    try {
      const json = await Call.ByName('main.AppService.TestAllModels', provider)
      return JSON.parse(json)
    } catch (e) {
      return [{ success: false, error: e.message }]
    }
  }

  // 工具/Skills
  async getTools() {
    try {
      const json = await Call.ByName('main.AppService.GetTools')
      return JSON.parse(json)
    } catch (e) { return [] }
  }
  async getSkillPool() {
    try {
      const json = await Call.ByName('main.AppService.GetSkillPool')
      return JSON.parse(json)
    } catch (e) { return {} }
  }
  async scanSkills() {
    try {
      const json = await Call.ByName('main.AppService.ScanSkills')
      return JSON.parse(json)
    } catch (e) { return { error: e.message } }
  }
  async uploadSkill(file) {
    try {
      const base64 = await readFileAsBase64(file)
      const json = await Call.ByName('main.AppService.UploadSkill', file.name, base64)
      return JSON.parse(json)
    } catch (e) { return { error: e.message } }
  }
  // 上传聊天文件
  async uploadChatFile(sessionId, file) {
    try {
      const base64 = await readFileAsBase64(file)
      const json = await Call.ByName('main.AppService.UploadChatFile', sessionId, file.name, base64)
      return JSON.parse(json)
    } catch (e) { return { error: e.message } }
  }
  async getEnabledSkills(agent) {
    try {
      const json = await Call.ByName('main.AppService.GetEnabledSkills', agent)
      return JSON.parse(json)
    } catch (e) { return {} }
  }
  async setEnabledSkills(agent, skills) {
    try {
      const json = await Call.ByName('main.AppService.SetEnabledSkills', agent, JSON.stringify(skills))
      return JSON.parse(json)
    } catch (e) { return { error: e.message } }
  }

  // 会话
  async getSessions() {
    try {
      const json = await Call.ByName('main.AppService.GetSessions')
      return JSON.parse(json)
    } catch (e) {
      console.error('[WailsAdapter] getSessions error:', e)
      return []
    }
  }
  async deleteSession(id) {
    try {
      await Call.ByName('main.AppService.DeleteSession', id)
      return { status: 'deleted' }
    } catch (e) {
      console.error('[WailsAdapter] deleteSession error:', e)
      return { status: 'deleted' }
    }
  }

  // 定时任务
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

  async getCronEnabled() {
    try {
      const json = await Call.ByName('main.AppService.GetCronEnabled')
      return JSON.parse(json)
    } catch (e) { return true }
  }
  async setCronEnabled(enabled) {
    try {
      await Call.ByName('main.AppService.SetCronEnabled', String(enabled))
      return { status: 'ok' }
    } catch (e) { return { status: 'ok' } }
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

  // Agent 文件管理
  async getAgentFiles(agent) {
    const json = await Call.ByName('main.AppService.GetAgentFiles', agent)
    return JSON.parse(json)
  }
  async readAgentFile(agent, file) {
    return await Call.ByName('main.AppService.ReadAgentFile', agent, file)
  }
  async writeAgentFile(agent, file, content) {
    return await Call.ByName('main.AppService.WriteAgentFile', agent, file, content)
  }

  // 安全审批
  async getPendingApprovals() {
    try {
      const json = await Call.ByName('main.AppService.GetPendingApprovals')
      return JSON.parse(json)
    } catch (e) {
      console.error('[WailsAdapter] getPendingApprovals error:', e)
      return []
    }
  }
  async approveRequest(id) {
    try {
      const json = await Call.ByName('main.AppService.ApproveRequest', id, 'wails_user')
      return JSON.parse(json)
    } catch (e) {
      console.error('[WailsAdapter] approveRequest error:', e)
      return { success: false }
    }
  }
  async denyRequest(id, reason) {
    try {
      const json = await Call.ByName('main.AppService.DenyRequest', id, 'wails_user', reason || '')
      return JSON.parse(json)
    } catch (e) {
      console.error('[WailsAdapter] denyRequest error:', e)
      return { success: false }
    }
  }

  // 安全配置
  async getSecurityConfig() {
    try {
      const json = await Call.ByName('main.AppService.GetSecurityConfig')
      return JSON.parse(json)
    } catch (e) {
      console.error('[WailsAdapter] getSecurityConfig error:', e)
      return {}
    }
  }
  async updateSecurityConfig(config) {
    try {
      const json = await Call.ByName('main.AppService.UpdateSecurityConfig', JSON.stringify(config))
      return JSON.parse(json)
    } catch (e) {
      console.error('[WailsAdapter] updateSecurityConfig error:', e)
      return { success: false }
    }
  }

  // 获取媒体文件（图片、视频、PDF 等）返回 Blob URL
  // 后端返回 JSON: {"base64": "...", "mime": "image/png"}
  // 前端解码 base64 → Uint8Array → Blob → URL.createObjectURL
  async getMedia(path) {
    try {
      const json = await Call.ByName('main.AppService.GetMedia', path)
      const result = JSON.parse(json)
      if (result.error) {
        console.error('[WailsAdapter] getMedia error:', result.error)
        return null
      }
      // 解码 base64 为二进制数据
      const binaryStr = atob(result.base64)
      const bytes = new Uint8Array(binaryStr.length)
      for (let i = 0; i < binaryStr.length; i++) {
        bytes[i] = binaryStr.charCodeAt(i)
      }
      // 创建 Blob URL
      const blob = new Blob([bytes], { type: result.mime })
      const url = URL.createObjectURL(blob)
      return url
    } catch (e) {
      console.error('[WailsAdapter] getMedia error:', e)
      return null
    }
  }

  // 文件预览（读取文件为 base64 数据 URL）
  async previewFile(path) {
    try {
      const json = await Call.ByName('main.AppService.PreviewFile', path)
      const result = JSON.parse(json)
      if (result.error) {
        console.error('[WailsAdapter] previewFile error:', result.error)
        return null
      }
      return result.dataUrl // 返回 data:image/png;base64,... 格式的 URL
    } catch (e) {
      console.error('[WailsAdapter] previewFile error:', e)
      return null
    }
  }

  // 文件下载（打开本地文件或 URL）
  async downloadFile(path, filename) {
    try {
      const json = await Call.ByName('main.AppService.DownloadFile', path, filename || '')
      return JSON.parse(json)
    } catch (e) {
      console.error('[WailsAdapter] downloadFile error:', e)
      return { error: e.message }
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

  // 重启
  async restart() {
    try {
      const json = await Call.ByName('main.AppService.Restart')
      return JSON.parse(json)
    } catch (e) {
      console.error('[WailsAdapter] restart error:', e)
      return { error: e.message }
    }
  }
}