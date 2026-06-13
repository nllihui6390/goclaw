<script setup>
import { ref, inject, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useAgentStore } from '@/stores/agent'

const api = inject('api')
const agentStore = useAgentStore()

const loading = ref(false)
const servers = ref([])
const toolsDialogVisible = ref(false)
const createDialogVisible = ref(false)
const jsonImportVisible = ref(false)
const currentTools = ref([])
const currentServerName = ref('')

// 创建模式: 'form' | 'json'
const createMode = ref('form')

// JSON 导入文本
const importJson = ref('')

// 表单
const form = ref({
  key: '',
  name: '',
  description: '',
  transport: 'stdio',
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
  try {
    const tools = await api.getMCPServerTools(row.name)
    currentTools.value = tools || []
    toolsDialogVisible.value = true
  } catch (e) {
    ElMessage.error('获取工具列表失败')
  }
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

// JSON 导入处理
async function handleJsonImport() {
  if (!importJson.value.trim()) {
    ElMessage.warning('请输入 JSON 配置')
    return
  }

  let parsed
  try {
    parsed = JSON.parse(importJson.value)
  } catch {
    ElMessage.error('无效的 JSON 格式')
    return
  }

  const clientsToCreate = []

  if (parsed.mcpServers && typeof parsed.mcpServers === 'object') {
    // 格式1: { "mcpServers": { "key": {...} } }
    Object.entries(parsed.mcpServers).forEach(([key, data]) => {
      if (typeof data === 'object' && data !== null) {
        clientsToCreate.push({ key, data: normalizeClientData(key, data) })
      }
    })
  } else if (parsed.key && (parsed.command || parsed.url || parsed.baseUrl)) {
    // 格式2: { "key": "...", "command": "..." }
    const { key, ...clientData } = parsed
    clientsToCreate.push({ key, data: normalizeClientData(key, clientData) })
  } else {
    // 格式3: { "key1": {...}, "key2": {...} }
    Object.entries(parsed).forEach(([key, data]) => {
      if (typeof data === 'object' && data !== null && (data.command || data.url || data.baseUrl)) {
        clientsToCreate.push({ key, data: normalizeClientData(key, data) })
      }
    })
  }

  if (clientsToCreate.length === 0) {
    ElMessage.warning('未找到有效的 MCP Server 配置')
    return
  }

  let created = 0
  let failed = 0
  for (const { data } of clientsToCreate) {
    try {
      await api.createMCPServer(agent.value, {
        name: data.name,
        command: data.command,
        url: data.url,
        args: data.args,
        env: data.env,
        enabled: data.enabled,
      })
      created++
    } catch {
      failed++
      ElMessage.error(`创建 "${data.name}" 失败`)
    }
  }

  if (created > 0) {
    ElMessage.success(`成功创建 ${created} 个 MCP Server${failed > 0 ? `，${failed} 个失败` : ''}`)
    jsonImportVisible.value = false
    importJson.value = ''
    loadServers()
  }
}

// 表单创建处理
async function handleCreate() {
  if (!form.value.key && !form.value.name) {
    ElMessage.warning('请输入 name')
    return
  }
  const serverName = form.value.key || form.value.name

  if (form.value.transport === 'stdio' && !form.value.command) {
    ElMessage.warning('Stdio 模式需要填写命令')
    return
  }
  if (form.value.transport !== 'stdio' && !form.value.url) {
    ElMessage.warning('HTTP/SSE 模式需要填写 URL')
    return
  }

  const config = {
    name: serverName,
    command: form.value.transport === 'stdio' ? form.value.command : '',
    url: form.value.transport !== 'stdio' ? form.value.url : '',
    args: parseArgs(form.value.args),
    env: parseEnv(form.value.env),
    enabled: true,
  }

  try {
    await api.createMCPServer(agent.value, config)
    ElMessage.success('创建成功')
    createDialogVisible.value = false
    resetForm()
    loadServers()
  } catch (e) {
    ElMessage.error('创建失败')
  }
}

function resetForm() {
  form.value = {
    key: '', name: '', description: '',
    transport: 'stdio', command: '', url: '',
    args: '', env: '', cwd: '', headers: ''
  }
}

function openCreate() {
  createMode.value = 'form'
  resetForm()
  createDialogVisible.value = true
}

function openJsonImport() {
  importJson.value = ''
  jsonImportVisible.value = true
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
        <el-button @click="openJsonImport">JSON 导入</el-button>
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
            <span class="server-name">{{ server.name }}</span>
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
            <span class="label">状态:</span>
            <el-tag :type="server.connected ? 'success' : 'info'" size="small">
              {{ server.connected ? '已连接' : '未连接' }}
            </el-tag>
          </div>
          <div class="info-row">
            <span class="label">工具数:</span>
            <span class="value">{{ server.tools_count || 0 }}</span>
          </div>
        </div>

        <div class="card-actions">
          <el-button size="small" @click="showTools(server)">查看工具</el-button>
          <el-button size="small" type="danger" @click="handleDelete(server)">删除</el-button>
        </div>
      </el-card>
    </div>

    <!-- 空状态 -->
    <el-empty v-else description="暂无 MCP Server" />

    <!-- 工具列表弹窗 -->
    <el-dialog v-model="toolsDialogVisible" :title="`${currentServerName} - 工具列表`" width="600px">
      <el-table :data="currentTools" size="small" stripe>
        <el-table-column prop="name" label="名称" width="180" />
        <el-table-column prop="description" label="描述" />
      </el-table>
    </el-dialog>

    <!-- 创建弹窗 -->
    <el-dialog v-model="createDialogVisible" title="新建 MCP Server" width="560px">
      <!-- 模式切换 -->
      <div class="mode-tabs">
        <el-radio-group v-model="createMode" size="small">
          <el-radio-button value="form">表单创建</el-radio-button>
          <el-radio-button value="json">JSON 导入</el-radio-button>
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

      <!-- JSON 导入模式 -->
      <div v-else class="json-import-section">
        <p class="json-hint">
          支持三种格式：<br/>
          ① mcpServers 包装：<code>{"mcpServers":{"key":{"command":"..."}}}</code><br/>
          ② 单条格式：<code>{"key":"...","name":"...","command":"..."}</code><br/>
          ③ 键值对格式：<code>{"key":{"command":"..."}}</code>
        </p>
        <el-input v-model="importJson" type="textarea" :rows="12"
          placeholder='{"mcpServers":{"filesystem":{"command":"npx","args":["-y","@modelcontextprotocol/server-filesystem","/path"]}}}'
        />
        <el-button type="primary" @click="handleJsonImport" style="margin-top:12px;width:100%">
          导入并创建
        </el-button>
      </div>

      <template #footer>
        <el-button @click="createDialogVisible = false">取消</el-button>
        <el-button v-if="createMode === 'form'" type="primary" @click="handleCreate">创建</el-button>
      </template>
    </el-dialog>

    <!-- JSON 导入弹窗 -->
    <el-dialog v-model="jsonImportVisible" title="JSON 导入 MCP Server" width="560px">
      <p class="json-hint">
        支持三种格式：<br/>
        ① mcpServers 包装：<code>{"mcpServers":{"key":{"command":"..."}}}</code><br/>
        ② 单条格式：<code>{"key":"...","name":"...","command":"..."}</code><br/>
        ③ 键值对格式：<code>{"key":{"command":"..."}}</code>
      </p>
      <el-input v-model="importJson" type="textarea" :rows="14"
        placeholder='{"mcpServers":{"filesystem":{"command":"npx","args":["-y","@modelcontextprotocol/server-filesystem","/path"]}}}'
      />
      <template #footer>
        <el-button @click="jsonImportVisible = false">取消</el-button>
        <el-button type="primary" @click="handleJsonImport">导入并创建</el-button>
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

.mode-tabs {
  margin-bottom: 16px;
}

.json-import-section {
  .json-hint {
    margin-bottom: 12px;
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