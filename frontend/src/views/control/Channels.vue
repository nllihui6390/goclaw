<script setup>
import { ref, inject, onMounted, computed } from 'vue'
import { ElMessage } from 'element-plus'

const api = inject('api')
const config = ref(null)
const channelsStatus = ref([])
const loading = ref(false)
const saving = ref(false)

// Channel type definitions with their config fields
const channelDefs = {
  console: {
    name: '控制台',
    type: 'console',
    fields: [] // No config fields, just enabled toggle
  },
  lark: {
    name: '飞书',
    type: 'lark',
    fields: [
      { key: 'app_id', label: 'App ID', type: 'text', placeholder: '飞书应用 ID' },
      { key: 'app_secret', label: 'App Secret', type: 'password', placeholder: '飞书应用密钥' }
    ]
  },
  dingtalk: {
    name: '钉钉',
    type: 'dingtalk',
    fields: [
      { key: 'client_id', label: 'Client ID', type: 'text', placeholder: '钉钉应用 Client ID' },
      { key: 'client_secret', label: 'Client Secret', type: 'password', placeholder: '钉钉应用密钥' }
    ]
  },
  wecom: {
    name: '企业微信',
    type: 'wecom',
    fields: [
      { key: 'bot_id', label: 'Bot ID', type: 'text', placeholder: '企业微信机器人 ID' },
      { key: 'secret', label: 'Secret', type: 'password', placeholder: '企业微信机器人密钥' }
    ]
  },
  wechat: {
    name: '微信',
    type: 'wechat',
    fields: [
      { key: 'base_url', label: 'Base URL', type: 'text', placeholder: '微信回调地址' },
      { key: 'bot_prefix', label: 'Bot 前缀', type: 'text', placeholder: '机器人命令前缀' },
      { key: 'bot_token', label: 'Bot Token', type: 'password', placeholder: '微信机器人 Token' },
      { key: 'bot_token_file', label: 'Token 文件', type: 'text', placeholder: 'clawdata/wechat_bot_token' },
      { key: 'media_dir', label: '媒体目录', type: 'text', placeholder: 'clawdata/media/wechat' }
    ]
  }
}

const channelList = computed(() => {
  if (!config.value?.channels) return []
  return Object.keys(channelDefs).map(key => ({
    key,
    ...channelDefs[key],
    config: config.value.channels[key] || { enabled: false },
    status: channelsStatus.value.find(s => s.name === key) || { connected: false }
  }))
})

onMounted(async () => {
  await loadData()
})

async function loadData() {
  loading.value = true
  try {
    const [cfg, status] = await Promise.all([
      api.getConfig(),
      api.getChannels()
    ])
    config.value = cfg
    channelsStatus.value = status
  } catch (e) {
    ElMessage.error('加载失败: ' + e.message)
  }
  loading.value = false
}

async function saveAll() {
  saving.value = true
  try {
    await api.saveConfig(config.value)
    ElMessage.success('保存成功')
  } catch (e) {
    ElMessage.error('保存失败: ' + e.message)
  }
  saving.value = false
}

function getStatusType(channel) {
  if (!channel.config.enabled) return 'info'
  return channel.status.connected ? 'success' : 'danger'
}

function getStatusText(channel) {
  if (!channel.config.enabled) return '未启用'
  return channel.status.connected ? '已连接' : '未连接'
}
</script>

<template>
  <div class="page" v-loading="loading">
    <div class="page-header">
      <h2>渠道管理</h2>
      <el-button type="primary" @click="saveAll" :loading="saving">保存配置</el-button>
    </div>

    <div class="channels-grid">
      <el-card v-for="channel in channelList" :key="channel.key" class="channel-card">
        <template #header>
          <div class="card-header">
            <div class="channel-title">
              <span class="channel-name">{{ channel.name }}</span>
              <el-tag size="small" :type="getStatusType(channel)">{{ getStatusText(channel) }}</el-tag>
            </div>
            <el-switch
              v-model="channel.config.enabled"
              active-text="启用"
              inactive-text="禁用"
            />
          </div>
        </template>

        <el-form label-width="100px" size="small" v-if="channel.fields.length > 0">
          <el-form-item
            v-for="field in channel.fields"
            :key="field.key"
            :label="field.label"
          >
            <el-input
              v-model="channel.config[field.key]"
              :type="field.type === 'password' ? 'password' : 'text'"
              :placeholder="field.placeholder"
              :disabled="!channel.config.enabled"
              clearable
            />
          </el-form-item>
        </el-form>

        <div v-else class="no-config">
          <span class="text-muted">无额外配置项</span>
        </div>
      </el-card>
    </div>
  </div>
</template>

<style scoped>
.page {
  padding: 24px;
}
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}
.page-header h2 {
  margin: 0;
  font-weight: 500;
}
.channels-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(400px, 1fr));
  gap: 16px;
}
.channel-card {
  min-width: 0;
}
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.channel-title {
  display: flex;
  align-items: center;
  gap: 8px;
}
.channel-name {
  font-weight: 500;
}
.no-config {
  padding: 16px 0;
  text-align: center;
}
.text-muted {
  color: #999;
  font-size: 13px;
}
</style>