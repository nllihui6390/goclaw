<script setup>
import { ref, inject, onMounted, computed, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRouter } from 'vue-router'
import { useAgentStore } from '@/stores/agent'

const api = inject('api')
const router = useRouter()
const agentStore = useAgentStore()

const loading = ref(false)
const sessions = ref([])

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

// 查看会话 → 跳转到聊天页面加载该会话
function viewSession(session) {
  router.push({ path: '/', query: { session: session.id, agent: session.agent } })
}

// 删除会话
async function deleteSession(session) {
  try {
    await ElMessageBox.confirm(`确定删除会话 "${session.name || session.id}"？`, '确认删除', {
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
        <el-table-column align="left" prop="id" label="会话ID" min-width="180" />
        <el-table-column align="left" label="会话名称" min-width="160">
          <template #default="{ row }">
            <span class="session-name">{{ row.name || row.id }}</span>
          </template>
        </el-table-column>
        <el-table-column align="center" prop="agent" label="Agent" width="100" />
        <el-table-column align="center" prop="channel" label="渠道" width="120" />
        <el-table-column align="center" prop="user_id" label="用户" width="120" />
        <el-table-column align="center" label="创建时间" width="170">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column align="center" label="更新时间" width="170">
          <template #default="{ row }">{{ formatTime(row.updated_at) }}</template>
        </el-table-column>
        <el-table-column align="center" fixed="right" label="操作" width="120">
          <template #default="{ row }">
            <el-button link type="primary" @click="viewSession(row)">查看</el-button>
            <el-button link type="danger" @click="deleteSession(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-if="!sessions.length && !loading" description="暂无会话" />
    </el-card>
  </div>
</template>

<style scoped>
.page { padding: 24px; }
.card-header { display: flex; justify-content: space-between; align-items: center; }
.session-name { font-weight: 500; }
</style>