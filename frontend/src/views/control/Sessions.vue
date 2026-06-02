<script setup>
import { ref, inject, onMounted, computed, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useAgentStore } from '@/stores/agent'

const api = inject('api')
const agentStore = useAgentStore()

const loading = ref(false)
const sessions = ref([])
const detailVisible = ref(false)
const detailSession = ref(null)
const detailMessages = ref([])
const detailLoading = ref(false)

// 按当前 agent 过滤会话列表
const filteredSessions = computed(() => {
  return sessions.value.filter(s => s.agent === agentStore.selectedAgent)
})

// 加载会话列表
async function loadSessions() {
  loading.value = true
  try {
    sessions.value = await api.getSessions() || []
  } catch (e) {
    ElMessage.error('加载失败: ' + e.message)
  }
  loading.value = false
}

// 查看会话详情
async function viewSession(session) {
  detailVisible.value = true
  detailSession.value = session
  detailMessages.value = []
  detailLoading.value = true
  try {
    const history = await api.getChatHistory(session.id, session.agent)
    detailMessages.value = history || []
  } catch (e) {
    detailMessages.value = []
  }
  detailLoading.value = false
}

// 删除会话
async function deleteSession(session) {
  try {
    await ElMessageBox.confirm(`确定删除会话 "${session.id}"？`, '确认删除', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消'
    })
    await api.deleteSession(session.id)
    ElMessage.success('删除成功')
    await loadSessions()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error('删除失败: ' + e.message)
  }
}

function formatTime(ts) {
  if (!ts) return '-'
  return new Date(ts).toLocaleString('zh-CN')
}

onMounted(loadSessions)
// 切换 agent 时刷新
watch(() => agentStore.selectedAgent, loadSessions)
</script>

<template>
  <div class="page" v-loading="loading">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>会话管理</span>
          <el-button type="primary" size="small" @click="loadSessions">
            <el-icon><Refresh /></el-icon>刷新
          </el-button>
        </div>
      </template>

      <el-table :data="filteredSessions" stripe>
        <el-table-column prop="id" label="会话 ID" min-width="200">
          <template #default="{ row }">
            <span class="mono">{{ row.id }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="agent" label="Agent" width="100" />
        <el-table-column label="更新时间" width="170">
          <template #default="{ row }">{{ formatTime(row.updated_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="180">
          <template #default="{ row }">
            <el-button link type="primary" @click="viewSession(row)">查看</el-button>
            <el-button link type="danger" @click="deleteSession(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-if="!sessions.length && !loading" description="暂无会话" />
    </el-card>

    <!-- 会话详情对话框 -->
    <el-dialog v-model="detailVisible" title="会话详情" width="700px" top="5vh">
      <div class="dialog-header">
        <span class="dialog-session">{{ detailSession?.id }}</span>
        <span class="dialog-agent">{{ detailSession?.agent }}</span>
      </div>
      <div class="messages-list" v-loading="detailLoading">
        <div v-for="(msg, i) in detailMessages" :key="i" class="message-item" :class="msg.role">
          <div class="message-role">{{ msg.role === 'user' ? '用户' : '助手' }}</div>
          <div class="message-content">{{ msg.content }}</div>
        </div>
        <el-empty v-if="!detailMessages.length && !detailLoading" description="暂无消息" :image-size="50" />
      </div>
      <template #footer>
        <el-button @click="detailVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page { padding: 24px; }
.card-header { display: flex; justify-content: space-between; align-items: center; }
.mono { font-family: 'Consolas', 'Monaco', monospace; font-size: 12px; }

.dialog-header { display: flex; gap: 12px; align-items: center; margin-bottom: 16px; }
.dialog-session { font-size: 14px; font-weight: 600; }
.dialog-agent { font-size: 12px; color: #909399; background: #f0f2f5; padding: 2px 8px; border-radius: 4px; }

.messages-list { max-height: 60vh; overflow-y: auto; }
.message-item { margin-bottom: 12px; padding: 10px 14px; border-radius: 8px; }
.message-item.user { background: #e8f4ff; }
.message-item.assistant { background: #f5f7fa; }
.message-role { font-size: 12px; color: #909399; margin-bottom: 4px; font-weight: 500; }
.message-content { font-size: 14px; line-height: 1.6; white-space: pre-wrap; word-break: break-word; }
</style>