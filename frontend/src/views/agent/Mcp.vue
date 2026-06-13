<script setup>
import { ref, inject, onMounted, computed, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useAgentStore } from '@/stores/agent'

const api = inject('api')
const agentStore = useAgentStore()

const loading = ref(false)
const servers = ref([])
const toolsDialogVisible = ref(false)
const toolsLoading = ref(false)
const createDialogVisible = ref(false)
const authDialogVisible = ref(false)
const currentTools = ref([])
const currentServerName = ref('')
const authServerUrl = ref('')

// 编辑状态
const isEditing = ref(false)
const editingOriginalName = ref('')

// 创建/编辑模式: 'form' | 'json'
const createMode = ref('form')

// JSON 编辑文本（与表单双向同步）
const formJson = ref('')

// 表单→JSON 转换
function formToJson() {
  const config = {
    name: form.value.key || form.value.name,
    command: form.value.transport === 'stdio' ? form.value.command : '',
    url: form.value.transport !== 'stdio' ? form.value.url : '',
    args: parseArgs(form.value.args),
    env: parseEnv(form.value.env),
    enabled: true,
  }
  if (form.value.description) config.description = form.value.description
  if (form.value.transport !== 'stdio') config.transport = form.value.transport
  formJson.value = JSON.stringify(config, null, 2)
}

// 表单
const form = ref({
  key: '',
  name: '',
  description: '',
  transport: 'streamable_http',
  command: '',
  url: '',
  args: '',
  env: '',
  cwd: '',
  headers: ''
})

const agent = computed(() => agentStore.selectedAgent || 'default')

async function loadServers() {
  loading.value = true
  try {
    const data = await api.getMCPServers(agent.value)
    servers.value = data || []
  } catch (e) {
    console.error('Failed to load MCP servers:', e)
    servers.value = []
  } finally {
    loading.value = false
  }
}

async function handleToggle(row) {
  try {
    await api.toggleMCPServer(agent.value, row.name)
    ElMessage.success(row.enabled ? '已禁用' : '已启用')
    loadServers()
  } catch (e) {
    ElMessage.error('操作失败')
  }
}

async function handleDelete(row) {
  try {
    await ElMessageBox.confirm(`确定删除 MCP Server "${row.name}" 吗？`, '提示', {
      type: 'warning'
    })
    await api.deleteMCPServer(agent.value, row.name)
    ElMessage.success('已删除')
    loadServers()
  } catch (e) {
    if (e !== 'cancel') {
      ElMessage.error('删除失败')
    }
  }
}

async function showTools(row) {
  currentServerName.value = row.name
  currentTools.value = []
  toolsLoading.value = true
  toolsDialogVisible.value = true
  try {
    const tools = await api.getMCPServerTools(row.name)
    currentTools.value = tools || []
  } catch (e) {
    ElMessage.error('获取工具列表失败')
  } finally {
    toolsLoading.value = false
  }
}

function openAuth(row) {
  currentServerName.value = row.name
  authServerUrl.value = row.url || ''
  authDialogVisible.value = true
}

function showSchema(row) {
  ElMessageBox.alert(JSON.stringify(row.inputSchema, null, 2), `${row.name} - 参数 Schema`).catch(() => {})
}

// 解析 HTTP headers
function parseHeaders(raw) {
  if (!raw) return {}
  try {
    return JSON.parse(raw)
  } catch {
    const result = {}
    raw.split('\n').forEach(line => {
      const idx = line.indexOf(':')
      if (idx > 0) result[line.slice(0, idx).trim()] = line.slice(idx + 1).trim()
    })
    return result
  }
}

// 解析环境变量
function parseEnv(raw) {
  if (!raw) return {}
  try {
    return JSON.parse(raw)
  } catch {
    const result = {}
    raw.split('\n').forEach(line => {
      const eq = line.indexOf('=')
      if (eq > 0) result[line.slice(0, eq).trim()] = line.slice(eq + 1).trim()
    })
    return result
  }
}

// 解析参数
function parseArgs(raw) {
  if (!raw) return []
  // 支持空格、逗号、换行分隔
  return raw.split(/[\s,\n]+/).filter(Boolean)
}

// 标准化 transport 值
function normalizeTransport(raw) {
  if (!raw) return undefined
  const v = raw.trim().toLowerCase()
  if (v === 'stdio') return 'stdio'
  if (v === 'sse') return 'sse'
  if (['streamablehttp', 'streamable_http', 'streamable-http', 'http'].includes(v)) return 'streamable_http'
  return undefined
}

// 标准化客户端数据
function normalizeClientData(key, raw) {
  const hasUrl = !!(raw.url || raw.baseUrl || raw.base_url)
  const transport = normalizeTransport(raw.transport || raw.type) || (hasUrl || !raw.command ? 'streamable_http' : 'stdio')

  const command = transport === 'stdio' ? (raw.command || '') : ''

  return {
    key: key,
    name: raw.name || key,
    description: raw.description || '',
    enabled: raw.enabled ?? raw.isActive ?? true,
    transport,
    url: (raw.url || raw.baseUrl || raw.base_url || '').toString(),
    command,
    args: Array.isArray(raw.args) ? raw.args : parseArgs(raw.args),
    env: raw.env && typeof raw.env === 'object' ? raw.env : parseEnv(raw.env),
    cwd: (raw.cwd || '').toString(),
    headers: raw.headers && typeof raw.headers === 'object' ? raw.headers : {},
  }
}

function buildConfigFromData() {
  if (createMode.value === 'json') {
    try {
      let parsed = JSON.parse(formJson.value || '{}')
      // 处理 mcpServers 包装格式：取第一个
      if (parsed.mcpServers && typeof parsed.mcpServers === 'object') {
        const entries = Object.entries(parsed.mcpServers)
        if (entries.length > 0) {
          parsed = { key: entries[0][0], ...entries[0][1] }
        }
      }
      // 处理键值对格式：取第一个有效条目
      if (!parsed.name && !parsed.key && !parsed.command && !parsed.url) {
        const entries = Object.entries(parsed).filter(([, v]) => typeof v === 'object' && v !== null && (v.command || v.url))
        if (entries.length > 0) {
          parsed = { key: entries[0][0], ...entries[0][1] }
        }
      }
      const key = parsed.name || parsed.key || ''
      const data = normalizeClientData(key, parsed)
      if (!data.name) {
        ElMessage.warning('请输入 name')
        return null
      }
      return buildFullConfig(data)
    } catch {
      ElMessage.error('JSON 格式无效')
      return null
    }
  } else {
    const name = form.value.key || form.value.name
    if (!name) {
      ElMessage.warning('请输入 name')
      return null
    }
    return buildFullConfig({
      key: name,
      name,
      description: form.value.description,
      transport: form.value.transport,
      command: form.value.transport === 'stdio' ? form.value.command : '',
      url: form.value.transport !== 'stdio' ? form.value.url : '',
      args: parseArgs(form.value.args),
      env: parseEnv(form.value.env),
      headers: parseHeaders(form.value.headers),
      cwd: form.value.cwd,
    })
  }
}

async function handleSave() {
  const config = buildConfigFromData()
  if (!config) return

  try {
    if (isEditing.value) {
      await api.updateMCPServer(agent.value, editingOriginalName.value, config)
      ElMessage.success('更新成功')
    } else {
      await api.createMCPServer(agent.value, config)
      ElMessage.success('创建成功')
    }
    createDialogVisible.value = false
    isEditing.value = false
    resetForm()
    loadServers()
  } catch (e) {
    ElMessage.error(isEditing.value ? '更新失败' : '创建失败')
  }
}

function resetForm() {
  form.value = {
    key: '', name: '', description: '',
    transport: 'streamable_http', command: '', url: '',
    args: '', env: '', cwd: '', headers: ''
  }
}

// 构建完整格式的 MCP 配置
function buildFullConfig(overrides = {}) {
  return {
    key: overrides.key || overrides.name || '',
    name: overrides.name || '',
    description: overrides.description || '',
    enabled: overrides.enabled ?? true,
    transport: overrides.transport || 'streamable_http',
    url: overrides.url || '',
    headers: overrides.headers || {},
    command: overrides.command || '',
    args: overrides.args || [],
    env: overrides.env || {},
    cwd: overrides.cwd || '',
    oauth_status: null,
  }
}

function openCreate() {
  isEditing.value = false
  editingOriginalName.value = ''
  createMode.value = 'json'
  resetForm()
  formJson.value = JSON.stringify(buildFullConfig(), null, 2)
  createDialogVisible.value = true
}

function openEdit(server) {
  isEditing.value = true
  editingOriginalName.value = server.name
  createMode.value = 'json'
  formJson.value = JSON.stringify(buildFullConfig({
    key: server.key || server.name,
    name: server.name,
    description: server.description || '',
    enabled: server.enabled,
    transport: server.transport || (server.command ? 'stdio' : 'streamable_http'),
    url: server.url || '',
    headers: server.headers || {},
    command: server.command || '',
    args: server.args || [],
    env: server.env || {},
    cwd: server.cwd || '',
  }), null, 2)
  createDialogVisible.value = true
}

onMounted(() => {
  loadServers()
})
</script>

<template>
  <div class="mcp-page">
    <!-- 头部 -->
    <div class="header">
      <h2>MCP 集成</h2>
      <div class="header-actions">
        <el-button type="primary" @click="openCreate">新建 Server</el-button>
      </div>
    </div>

    <!-- 加载状态 -->
    <div v-if="loading" class="loading">
      <el-icon class="is-loading"><Loading /></el-icon>
      <span>加载中...</span>
    </div>

    <!-- Server 列表 -->
    <div v-else-if="servers.length > 0" class="server-grid">
      <el-card v-for="server in servers" :key="server.name" class="server-card">
        <template #header>
          <div class="card-header">
            <div class="header-left">
              <span class="server-name">{{ server.name }}</span>
              <el-tag :type="server.connected ? 'success' : 'info'" size="small" class="status-tag">
                {{ server.connected ? '已连接' : '未连接' }}
              </el-tag>
            </div>
            <el-switch :model-value="server.enabled" @change="handleToggle(server)" />
          </div>
        </template>

        <div class="server-info">
          <div class="info-row">
            <span class="label">类型:</span>
            <span class="value">{{ server.command ? 'StdIO' : 'Remote' }}</span>
          </div>
          <div v-if="server.command" class="info-row">
            <span class="label">命令:</span>
            <span class="value code">{{ server.command }} {{ (server.args || []).join(' ') }}</span>
          </div>
          <div v-if="server.url" class="info-row">
            <span class="label">URL:</span>
            <span class="value code">{{ server.url }}</span>
          </div>
          <div class="info-row">
            <span class="label">工具数:</span>
            <span class="value">{{ server.tools_count || 0 }}</span>
          </div>
        </div>

        <div class="card-actions">
          <el-button size="small" @click="showTools(server)">工具</el-button>
          <el-button v-if="server.url" size="small" type="warning" @click="openAuth(server)">授权</el-button>
          <el-button size="small" type="primary" @click="openEdit(server)">编辑</el-button>
          <el-button size="small" type="danger" @click="handleDelete(server)">删除</el-button>
        </div>
      </el-card>
    </div>

    <!-- 空状态 -->
    <el-empty v-else description="暂无 MCP Server" />

    <!-- 工具列表弹窗 -->
    <el-dialog v-model="toolsDialogVisible" :title="`${currentServerName} - 工具列表`" width="700px">
      <div v-if="toolsLoading" class="loading-inline">
        <el-icon class="is-loading"><Loading /></el-icon>
        <span>加载中...</span>
      </div>
      <el-table v-else-if="currentTools.length > 0" :data="currentTools" size="small" stripe height="360">
        <el-table-column prop="name" label="名称" width="200" />
        <el-table-column prop="description" label="描述" />
        <el-table-column prop="inputSchema" label="参数" width="80">
          <template #default="{ row }">
            <el-button v-if="row.inputSchema" size="small" link type="primary" @click="showSchema(row)">查看</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-else description="暂无工具" />
    </el-dialog>

    <!-- 授权弹窗 -->
    <el-dialog v-model="authDialogVisible" :title="`${currentServerName} - 授权`" width="500px">
      <p class="auth-hint">Remote MCP Server 需要 OAuth 2.1 授权。</p>
      <p class="auth-hint">当前 Server URL: <code>{{ authServerUrl }}</code></p>
      <p class="auth-hint">可通过浏览器访问该 Server 完成授权后，刷新此页面。</p>
      <template #footer>
        <el-button @click="authDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- 创建/编辑弹窗 -->
    <el-dialog v-model="createDialogVisible" :title="isEditing ? '编辑 MCP Server' : '新建 MCP Server'" width="700px">
      <!-- 模式切换（仅新建时显示） -->
      <div v-if="!isEditing" class="mode-tabs">
        <el-radio-group v-model="createMode" size="small">
          <el-radio-button value="json">JSON</el-radio-button>
          <el-radio-button value="form">表单</el-radio-button>
        </el-radio-group>
      </div>

      <!-- 表单模式 -->
      <el-form v-if="createMode === 'form'" :model="form" label-width="80px">
        <el-form-item label="Key" required>
          <el-input v-model="form.key" placeholder="唯一标识符（可选，默认使用 name）" />
        </el-form-item>
        <el-form-item label="名称" required>
          <el-input v-model="form.name" placeholder="显示名称" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" placeholder="描述说明" />
        </el-form-item>
        <el-form-item label="传输类型">
          <el-select v-model="form.transport" style="width: 100%">
            <el-option label="Stdio" value="stdio" />
            <el-option label="Streamable HTTP" value="streamable_http" />
            <el-option label="SSE" value="sse" />
          </el-select>
        </el-form-item>

        <!-- Stdio 字段 -->
        <template v-if="form.transport === 'stdio'">
          <el-form-item label="命令" required>
            <el-input v-model="form.command" placeholder="如: npx" />
          </el-form-item>
          <el-form-item label="参数">
            <el-input v-model="form.args" placeholder="如: -y @modelcontextprotocol/server-filesystem /path" />
          </el-form-item>
          <el-form-item label="环境变量">
            <el-input v-model="form.env" type="textarea" :rows="3" placeholder="KEY=VALUE 格式，每行一个&#10;如: API_KEY=xxx" />
          </el-form-item>
          <el-form-item label="工作目录">
            <el-input v-model="form.cwd" placeholder="命令执行的工作目录" />
          </el-form-item>
        </template>

        <!-- HTTP/SSE 字段 -->
        <template v-else>
          <el-form-item label="URL" required>
            <el-input v-model="form.url" placeholder="如: http://localhost:3000/mcp" />
          </el-form-item>
          <el-form-item label="Headers">
            <el-input v-model="form.headers" type="textarea" :rows="2" placeholder='Header: Value 每行一个&#10;或 JSON: {"Authorization":"Bearer xxx"}' />
          </el-form-item>
        </template>
      </el-form>

      <!-- JSON 模式 -->
      <div v-else class="json-section">
        <p class="json-hint">
          支持三种格式：<br/>
          ① 单条格式：<code>{"name":"...","command":"..."}</code><br/>
          ② mcpServers 包装：<code>{"mcpServers":{"key":{"command":"..."}}}</code><br/>
          ③ 键值对格式：<code>{"key":{"command":"..."}}</code>
        </p>
        <el-input v-model="formJson" type="textarea" :rows="14"
          placeholder='{"name":"filesystem","command":"npx","args":["-y","@modelcontextprotocol/server-filesystem","/path"]}'
        />
      </div>

      <template #footer>
        <el-button @click="createDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleSave">{{ isEditing ? '保存' : '创建' }}</el-button>
      </template>
    </el-dialog>

    </div>
</template>

<style lang="scss" scoped>
@use '@/styles/variables.scss' as *;

.mcp-page {
  padding: 20px;
}

.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;

  h2 {
    color: $text-primary;
    font-weight: 500;
    margin: 0;
  }

  .header-actions {
    display: flex;
    gap: 8px;
  }
}

.loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 60px;
  color: $text-muted;
}

.loading-inline {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 80px;
  color: $text-muted;
}

.mode-tabs {
  margin-bottom: 16px;
}

.json-section {
  .json-hint {
    margin-bottom: 12px;
  }
}

.auth-hint {
  font-size: 13px;
  color: $text-muted;
  margin-bottom: 8px;
  line-height: 1.6;

  code {
    font-family: monospace;
    font-size: 12px;
    background: $bg-glass-light;
    padding: 1px 4px;
    border-radius: 3px;
    word-break: break-all;
  }
}

.json-hint {
  font-size: 13px;
  color: $text-muted;
  line-height: 1.8;

  code {
    font-family: monospace;
    font-size: 12px;
    background: rgba(0, 0, 0, 0.04);
    padding: 1px 4px;
    border-radius: 3px;
    white-space: nowrap;
  }
}

.server-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(380px, 1fr));
  gap: 16px;
}

.server-card {
  .card-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .header-left {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .status-tag {
    flex-shrink: 0;
  }

  .server-name {
    font-weight: 500;
    color: $text-primary;
  }

  .server-info {
    margin-bottom: 12px;
  }

  .info-row {
    display: flex;
    margin-bottom: 8px;
    font-size: 13px;

    .label {
      color: $text-muted;
      width: 60px;
      flex-shrink: 0;
    }

    .value {
      color: $text-secondary;
      flex: 1;
      word-break: break-all;

      &.code {
        font-family: monospace;
        font-size: 12px;
        line-height: 1.5;
        background: $bg-glass-light;
        padding: 2px 6px;
        border-radius: 4px;
      }
    }
  }

  .card-actions {
    display: flex;
    gap: 8px;
  }
}
</style>