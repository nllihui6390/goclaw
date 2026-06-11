<script setup>
import { ref, inject, onMounted, onBeforeUnmount, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useAgentStore } from '@/stores/agent'

const api = inject('api')
const agentStore = useAgentStore()
const channels = ref([])
const loading = ref(false)
const saving = ref(false)
const dialogVisible = ref(false)
const editChannel = ref(null)
const editConfig = ref({})
const qrcodeImg = ref('')
const qrcodeLoading = ref(false)
const qrcodePollToken = ref('')
const qrcodePollTimer = ref(null)
const qrcodeStatus = ref('')

const currentAgent = computed(() => agentStore.selectedAgent || 'default')

const channelDefs = {
  console: { icon: 'Monitor', fields: [
      { key: 'bot_prefix', label: 'Bot 前缀', type: 'text', placeholder: '机器人命令前缀' }
    ] },
  lark: {
    icon: 'ChatDotSquare',
    qrcode: true,
    qrcodeConfig: {
      successStatus: 'success',
      successCredentialKey: 'app_id',
      pollInterval: 2000,
      credentialMap: { app_id: 'app_id', app_secret: 'app_secret' },
    },
    fields: [
      { key: 'app_id', label: 'App ID', type: 'text', placeholder: '飞书应用 ID' },
      { key: 'app_secret', label: 'App Secret', type: 'password', placeholder: '飞书应用密钥' },
      { key: 'bot_prefix', label: 'Bot 前缀', type: 'text', placeholder: '机器人命令前缀，如 @机器人' }
    ]
  },
  dingtalk: {
    icon: 'ChatLineSquare',
    qrcode: true,
    qrcodeConfig: {
      successStatus: 'success',
      successCredentialKey: 'client_id',
      pollInterval: 5000,
      credentialMap: { client_id: 'client_id', client_secret: 'client_secret' },
    },
    fields: [
      { key: 'client_id', label: 'Client ID', type: 'text', placeholder: '钉钉应用 Client ID' },
      { key: 'client_secret', label: 'Client Secret', type: 'password', placeholder: '钉钉应用密钥' },
      { key: 'bot_prefix', label: 'Bot 前缀', type: 'text', placeholder: '机器人命令前缀，如 @机器人' }
    ]
  },
  wecom: {
    icon: 'ChatRound',
    qrcode: true,
    qrcodeConfig: {
      successStatus: 'success',
      successCredentialKey: 'bot_id',
      pollInterval: 3000,
      credentialMap: { bot_id: 'bot_id', secret: 'secret' },
    },
    fields: [
      { key: 'bot_id', label: 'Bot ID', type: 'text', placeholder: '企业微信机器人 ID' },
      { key: 'secret', label: 'Secret', type: 'password', placeholder: '企业微信机器人密钥' },
      { key: 'bot_prefix', label: 'Bot 前缀', type: 'text', placeholder: '机器人命令前缀' }
    ]
  },
  wechat: {
    icon: 'ChatDotRound',
    qrcode: true,
    qrcodeConfig: {
      successStatus: 'confirmed',
      successCredentialKey: 'bot_token',
      pollInterval: 2000,
      credentialMap: { bot_token: 'bot_token', base_url: 'base_url' },
    },
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

// 监听 agent 切换时重新加载
watch(() => agentStore.selectedAgent, loadData)

async function loadData() {
  loading.value = true
  try {
    const list = await api.getChannels(currentAgent.value)
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

async function toggleChannel(channel, val) {
  const newConfig = { ...channel.config, enabled: val }
  try {
    await api.updateChannel(currentAgent.value, channel.key, newConfig)
    channel.config = newConfig
    channel.enabled = val
    ElMessage.success(`${channel.name} 已${val ? '启用' : '禁用'}`)
  } catch (e) {
    ElMessage.error('保存失败: ' + e.message)
  }
}

function openEdit(channel) {
  stopQrcodePoll()
  editChannel.value = channel
  editConfig.value = JSON.parse(JSON.stringify(channel.config))
  dialogVisible.value = true
}

async function saveChannel() {
  saving.value = true
  try {
    await api.updateChannel(currentAgent.value, editChannel.value.key, editConfig.value)
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

async function fetchQRCode() {
  stopQrcodePoll()
  qrcodeImg.value = ''
  qrcodeStatus.value = ''
  qrcodeLoading.value = true
  try {
    const chKey = editChannel.value?.key
    const data = await api.getChannelQRCode(chKey)
    if (data.error) { ElMessage.error('获取二维码失败: ' + data.error); return }
    if (!data.qrcode_img) { ElMessage.error('获取二维码失败: 未返回二维码图片'); return }
    qrcodeImg.value = data.qrcode_img
    qrcodePollToken.value = data.poll_token
    qrcodeStatus.value = 'waiting'
    scheduleQrcodePoll()
  } catch (e) {
    ElMessage.error('获取二维码失败: ' + e.message)
  } finally {
    qrcodeLoading.value = false
  }
}

function scheduleQrcodePoll() {
  stopQrcodePoll()
  const chKey = editChannel.value?.key
  const def = channelDefs[chKey]
  const cfg = def?.qrcodeConfig || channelDefs.wechat.qrcodeConfig
  const pollInterval = cfg.pollInterval || 2000

  qrcodePollTimer.value = setTimeout(async () => {
    try {
      const result = await api.getChannelQRCodeStatus(chKey, qrcodePollToken.value)
      if (result.error) { scheduleQrcodePoll(); return }
      qrcodeStatus.value = result.status
      if (result.status === cfg.successStatus && result.credentials?.[cfg.successCredentialKey]) {
        qrcodeImg.value = ''
        qrcodeStatus.value = ''
        // 自动填入凭据
        if (cfg.credentialMap) {
          for (const [field, key] of Object.entries(cfg.credentialMap)) {
            if (result.credentials[key]) editConfig.value[field] = result.credentials[key]
          }
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
    } catch {}
    scheduleQrcodePoll()
  }, pollInterval)
}

function stopQrcodePoll() {
  if (qrcodePollTimer.value) {
    clearTimeout(qrcodePollTimer.value)
    qrcodePollTimer.value = null
  }
}

function getQrcodeStatusText() {
  const chKey = editChannel.value?.key
  switch (qrcodeStatus.value) {
    case 'waiting': return '等待扫描...'
    case 'scanned': return '已扫描，请在手机上确认登录'
    case 'confirmed':
    case 'success': return '扫码成功！'
    case 'expired': return '二维码已过期'
    default: return ''
  }
}

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
    <!-- Page header -->
    <div class="page-header">
      <div class="header-left">
        <h2>频道管理</h2>
        <el-tag size="small">{{ currentAgent }}</el-tag>
      </div>
      <div class="header-actions">
        <el-button @click="loadData" :loading="loading">
          <el-icon><Refresh /></el-icon>刷新
        </el-button>
      </div>
    </div>

    <!-- Channel cards grid -->
    <div class="channels-grid">
      <div
        v-for="channel in channels"
        :key="channel.key"
        class="channel-card"
        @click="openEdit(channel)"
      >
        <div class="card-inner">
          <div class="card-top">
            <div class="channel-icon-wrap">
              <el-icon :size="24"><component :is="channel.icon" /></el-icon>
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
      </div>
    </div>

    <!-- Edit dialog -->
    <el-dialog
      v-model="dialogVisible"
      :title="editChannel?.name + ' 配置'"
      width="500px"
      :close-on-click-modal="false"
      @close="closeDialog"
    >
      <el-form :model="editConfig" label-width="100px" v-if="editChannel">
        <el-form-item label="状态">
          <el-switch v-model="editConfig.enabled" active-text="启用" inactive-text="禁用" />
        </el-form-item>

        <el-form-item v-if="editChannel.qrcode" label="扫码登录">
          <el-button type="primary" :loading="qrcodeLoading" @click="fetchQRCode">
            获取二维码
          </el-button>
          <div v-if="qrcodeImg" class="qrcode-block">
            <img :src="'data:image/png;base64,' + qrcodeImg" :alt="editChannel.name + ' QR Code'" class="qrcode-img" />
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

        <!-- 显示配置 -->
        <el-divider content-position="left" style="margin: 16px 0;">显示配置</el-divider>

        <el-form-item label="工具消息">
          <el-switch v-model="editConfig.show_tool_messages" />
          <span class="field-hint">显示工具调用过程和输出结果</span>
        </el-form-item>

        <el-form-item label="思考内容">
          <el-switch v-model="editConfig.show_thinking" />
          <span class="field-hint">显示模型推理/思考过程</span>
        </el-form-item>

        <el-form-item label="流式输出">
          <el-switch v-model="editConfig.stream_output" />
          <span class="field-hint">逐字流式输出响应内容</span>
        </el-form-item>
      </el-form>
      <el-empty v-if="!editChannel" description="请选择一个频道" :image-size="50" />
      <template #footer>
        <el-button @click="closeDialog">取消</el-button>
        <el-button type="primary" @click="saveChannel" :loading="saving">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style lang="scss" scoped>
@use '@/styles/variables.scss' as *;

.page {
  padding: 32px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 28px;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-left h2 {
  margin: 0;
  font-size: $font-size-xl;
  font-weight: 600;
  color: $text-primary;
}

.header-actions {
  display: flex;
  gap: 8px;
}

.channels-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 16px;
}

.channel-card {
  @include glass-panel;
  border-radius: $radius-lg;
  cursor: pointer;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  padding: 20px;
  @include stagger-entrance(8, 0.05s);

  &:hover {
    border-color: $accent-cyan-dim;
    box-shadow: $shadow-glow-cyan;
    transform: translateY(-3px);
  }
}

.card-inner {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.card-top {
  display: flex;
  align-items: center;
  gap: 16px;
}

.channel-icon-wrap {
  width: 56px;
  height: 56px;
  border-radius: $radius-md;
  background: linear-gradient(135deg, $accent-cyan-dim, rgba(0, 212, 255, 0.05));
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  border: 1px solid rgba(0, 212, 255, 0.2);
  transition: all 0.3s;

  .el-icon {
    color: $accent-cyan;
  }

  .channel-card:hover & {
    background: linear-gradient(135deg, rgba(0, 212, 255, 0.2), rgba(0, 212, 255, 0.08));
    transform: scale(1.05);
  }
}

.channel-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.channel-name {
  font-weight: 600;
  font-size: $font-size-lg;
  color: $text-primary;
}

.channel-key {
  font-size: $font-size-xs;
  color: $text-muted;
  font-family: $font-display;
}

.card-bottom {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.qrcode-block {
  text-align: center;
  margin-top: 12px;
}

.qrcode-img {
  width: 200px;
  height: 200px;
  border-radius: $radius-md;
  border: 1px solid $border-default;
}

.qrcode-hint {
  margin-top: 8px;
  font-size: $font-size-sm;
  color: $text-secondary;
  font-family: $font-display;
}

.field-hint {
  display: block;
  margin-left: 12px;
  font-size: $font-size-xs;
  color: $text-muted;
  font-family: $font-display;
}
</style>