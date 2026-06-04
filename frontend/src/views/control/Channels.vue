<script setup>
import { ref, inject, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'

const api = inject('api')
const channels = ref([])
const loading = ref(false)
const saving = ref(false)
const dialogVisible = ref(false)
const editChannel = ref(null)
const editConfig = ref({})

// QR code 扫码相关状态
const qrcodeImg = ref('')
const qrcodeLoading = ref(false)
const qrcodePollToken = ref('')
const qrcodePollTimer = ref(null)
const qrcodeStatus = ref('') // waiting/scanned/confirmed/expired

// Channel type definitions with their config fields
const channelDefs = {
  console: { icon: 'Monitor', fields: [] },
  lark: {
    icon: 'ChatDotSquare',
    fields: [
      { key: 'app_id', label: 'App ID', type: 'text', placeholder: '飞书应用 ID' },
      { key: 'app_secret', label: 'App Secret', type: 'password', placeholder: '飞书应用密钥' }
    ]
  },
  dingtalk: {
    icon: 'ChatLineSquare',
    fields: [
      { key: 'client_id', label: 'Client ID', type: 'text', placeholder: '钉钉应用 Client ID' },
      { key: 'client_secret', label: 'Client Secret', type: 'password', placeholder: '钉钉应用密钥' }
    ]
  },
  wecom: {
    icon: 'ChatRound',
    fields: [
      { key: 'bot_id', label: 'Bot ID', type: 'text', placeholder: '企业微信机器人 ID' },
      { key: 'secret', label: 'Secret', type: 'password', placeholder: '企业微信机器人密钥' }
    ]
  },
  wechat: {
    icon: 'ChatDotRound',
    qrcode: true, // 支持扫码登录
    fields: [
      { key: 'base_url', label: 'Base URL', type: 'text', placeholder: '微信回调地址' },
      { key: 'bot_prefix', label: 'Bot 前缀', type: 'text', placeholder: '机器人命令前缀' },
      { key: 'bot_token', label: 'Bot Token', type: 'password', placeholder: '微信机器人 Token' },
      { key: 'bot_token_file', label: 'Token 文件', type: 'text', placeholder: 'clawdata/wechat_bot_token' },
      { key: 'media_dir', label: '媒体目录', type: 'text', placeholder: 'clawdata/media/wechat' }
    ]
  }
}

onMounted(loadData)
onBeforeUnmount(stopQrcodePoll)

async function loadData() {
  loading.value = true
  try {
    const list = await api.getChannels()
    // 合并前端定义的图标和字段
    channels.value = (list || []).map(ch => ({
      ...ch,
      icon: channelDefs[ch.key]?.icon || 'Setting',
      fields: channelDefs[ch.key]?.fields || [],
      qrcode: channelDefs[ch.key]?.qrcode || false
    }))
  } catch (e) {
    ElMessage.error('加载失败: ' + e.message)
  }
  loading.value = false
}

// 开关切换自动保存
async function toggleChannel(channel, val) {
  const newConfig = { ...channel.config, enabled: val }
  try {
    await api.updateChannel(channel.key, newConfig)
    channel.config = newConfig
    channel.enabled = val
    ElMessage.success(`${channel.name} 已${val ? '启用' : '禁用'}`)
  } catch (e) {
    ElMessage.error('保存失败: ' + e.message)
  }
}

// 点击卡片打开编辑对话框
function openEdit(channel) {
  stopQrcodePoll()
  editChannel.value = channel
  editConfig.value = JSON.parse(JSON.stringify(channel.config))
  dialogVisible.value = true
}

// 对话框内保存
async function saveChannel() {
  saving.value = true
  try {
    await api.updateChannel(editChannel.value.key, editConfig.value)
    // 同步到内存
    editChannel.value.config = editConfig.value
    editChannel.value.enabled = editConfig.value.enabled || false
    ElMessage.success('保存成功')
    dialogVisible.value = false
  } catch (e) {
    ElMessage.error('保存失败: ' + e.message)
  }
  saving.value = false
}

function getStatusType(channel) {
  if (!channel.enabled) return 'info'
  return channel.status === 'connected' ? 'success' : 'danger'
}

function getStatusText(channel) {
  if (!channel.enabled) return '未启用'
  return channel.status === 'connected' ? '已连接' : '未连接'
}

// ─────────── QR Code 扫码登录 ───────────

async function fetchQRCode() {
  stopQrcodePoll()
  qrcodeImg.value = ''
  qrcodeStatus.value = ''
  qrcodeLoading.value = true
  try {
    const data = await api.getChannelQRCode('wechat')
    if (data.error) {
      ElMessage.error('获取二维码失败: ' + data.error)
      return
    }
    if (!data.qrcode_img) {
      ElMessage.error('获取二维码失败: 未返回二维码图片')
      return
    }
    qrcodeImg.value = data.qrcode_img
    qrcodePollToken.value = data.poll_token
    qrcodeStatus.value = 'waiting'

    // 开始轮询
    scheduleQrcodePoll()
  } catch (e) {
    ElMessage.error('获取二维码失败: ' + e.message)
  } finally {
    qrcodeLoading.value = false
  }
}

function scheduleQrcodePoll() {
  stopQrcodePoll()
  qrcodePollTimer.value = setTimeout(async () => {
    try {
      const result = await api.getChannelQRCodeStatus('wechat', qrcodePollToken.value)
      if (result.error) {
        // 忽略单次轮询错误
        scheduleQrcodePoll()
        return
      }

      qrcodeStatus.value = result.status

      if (result.status === 'confirmed' && result.credentials?.bot_token) {
        // 扫码成功，自动填入凭据
        qrcodeImg.value = ''
        qrcodeStatus.value = ''
        if (result.credentials.bot_token) {
          editConfig.value.bot_token = result.credentials.bot_token
        }
        if (result.credentials.base_url) {
          editConfig.value.base_url = result.credentials.base_url
        }
        ElMessage.success('扫码登录成功，凭据已自动填入')
        stopQrcodePoll()
        return
      } else if (result.status === 'expired') {
        qrcodeImg.value = ''
        qrcodeStatus.value = ''
        ElMessage.warning('二维码已过期，请重新获取')
        stopQrcodePoll()
        return
      }
    } catch {
      // 忽略单次轮询错误
    }
    // 继续轮询
    scheduleQrcodePoll()
  }, 2000)
}

function stopQrcodePoll() {
  if (qrcodePollTimer.value) {
    clearTimeout(qrcodePollTimer.value)
    qrcodePollTimer.value = null
  }
}

function getQrcodeStatusText() {
  switch (qrcodeStatus.value) {
    case 'waiting': return '等待扫描...'
    case 'scanned': return '已扫描，请在手机上确认登录'
    case 'confirmed': return '扫码成功！'
    case 'expired': return '二维码已过期'
    default: return ''
  }
}

// 关闭对话框时清理
function closeDialog() {
  stopQrcodePoll()
  qrcodeImg.value = ''
  qrcodeStatus.value = ''
  qrcodePollToken.value = ''
  dialogVisible.value = false
}
</script>

<template>
  <div class="page" v-loading="loading">
    <div class="page-header">
      <h2>渠道管理</h2>
    </div>

    <div class="channels-grid">
      <el-card
        v-for="channel in channels"
        :key="channel.key"
        class="channel-card"
        shadow="hover"
      >
        <div class="card-inner" @click="openEdit(channel)">
          <div class="card-top">
            <div class="channel-icon">
              <el-icon :size="28"><component :is="channel.icon" /></el-icon>
            </div>
            <div class="channel-info">
              <span class="channel-name">{{ channel.name }}</span>
              <span class="channel-key">{{ channel.key }}</span>
            </div>
          </div>
          <div class="card-bottom">
            <el-tag size="small" :type="getStatusType(channel)">{{ getStatusText(channel) }}</el-tag>
            <el-switch
              :model-value="channel.enabled"
              @update:model-value="val => toggleChannel(channel, val)"
              @click.stop
              size="small"
            />
          </div>
        </div>
      </el-card>
    </div>

    <!-- 编辑对话框 -->
    <el-dialog
      v-model="dialogVisible"
      :title="editChannel?.name + ' 配置'"
      width="500px"
      :close-on-click-modal="false"
      @close="closeDialog"
    >
      <el-form :model="editConfig" label-width="100px" v-if="editChannel">
        <el-form-item label="状态">
          <el-switch
            v-model="editConfig.enabled"
            active-text="启用"
            inactive-text="禁用"
          />
        </el-form-item>

        <!-- QR Code 扫码登录区块（仅 wechat 渠道显示） -->
        <el-form-item v-if="editChannel.qrcode" label="扫码登录">
          <el-button
            type="primary"
            :loading="qrcodeLoading"
            @click="fetchQRCode"
          >
            获取二维码
          </el-button>
          <div v-if="qrcodeImg" class="qrcode-block">
            <img
              :src="'data:image/png;base64,' + qrcodeImg"
              alt="微信扫码登录"
              class="qrcode-img"
            />
            <div class="qrcode-hint">{{ getQrcodeStatusText() }}</div>
          </div>
        </el-form-item>

        <el-form-item
          v-for="field in editChannel.fields"
          :key="field.key"
          :label="field.label"
        >
          <el-input
            v-model="editConfig[field.key]"
            :type="field.type === 'password' ? 'password' : 'text'"
            :placeholder="field.placeholder"
            clearable
          />
        </el-form-item>
      </el-form>
      <el-empty v-if="!editChannel" description="请选择一个渠道" :image-size="50" />
      <template #footer>
        <el-button @click="closeDialog">取消</el-button>
        <el-button type="primary" @click="saveChannel" :loading="saving">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page { padding: 24px; }
.page-header { margin-bottom: 24px; }
.page-header h2 { margin: 0; font-weight: 500; }
.channels-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(400px, 1fr)); gap: 16px; }
.channel-card { cursor: pointer; transition: all .2s; }
.channel-card:hover { transform: translateY(-2px); box-shadow: 0 4px 12px rgba(0,0,0,.1); }
.card-inner { display: flex; flex-direction: column; gap: 16px; }
.card-top { display: flex; align-items: center; gap: 14px; }
.channel-icon {
  width: 48px; height: 48px; border-radius: 10px; background: #f0f2f5;
  display: flex; align-items: center; justify-content: center; color: #409eff; flex-shrink: 0;
}
.channel-info { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.channel-name { font-weight: 600; font-size: 15px; }
.channel-key { font-size: 12px; color: #bbb; font-family: monospace; }
.card-bottom { display: flex; justify-content: space-between; align-items: center; }

/* QR Code 扫码样式 */
.qrcode-block { text-align: center; margin-top: 12px; }
.qrcode-img { width: 200px; height: 200px; }
.qrcode-hint {
  margin-top: 8px; font-size: 12px; color: rgba(0,0,0,0.45);
}
</style>