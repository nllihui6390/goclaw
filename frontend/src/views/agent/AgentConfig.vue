<script setup>
import { ref, inject, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'

const api = inject('api')
const agents = ref([])
const loading = ref(false)
const editing = ref(false)
const currentAgent = ref(null)
const dialogVisible = ref(false)

const toolsOptions = [
  'weather', 'exec', 'write_file', 'read_file', 'edit_file', 'append_file',
  'send_file', 'browser_use', 'get_current_time', 'set_user_timezone', 'cron_status'
]

onMounted(async () => {
  loading.value = true
  try {
    agents.value = await api.getAgents()
  } catch (e) {
    ElMessage.error('加载失败: ' + e.message)
  }
  loading.value = false
})

function openEdit(agent) {
  currentAgent.value = { ...agent }
  editing.value = true
  dialogVisible.value = true
}

async function saveAgent() {
  try {
    await api.updateAgent(currentAgent.value.name, currentAgent.value)
    ElMessage.success('保存成功')
    agents.value = await api.getAgents()
    dialogVisible.value = false
  } catch (e) {
    ElMessage.error('保存失败: ' + e.message)
  }
}
</script>

<template>
  <div class="page">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>Agent 配置</span>
          <el-button type="primary" size="small" @click="openEdit({name:'',provider:'',model:'',system_prompt:'',tools:[],max_iterations:20})">新增</el-button>
        </div>
      </template>
      <el-table :data="agents" v-loading="loading" stripe>
        <el-table-column prop="name" label="名称" width="120" />
        <el-table-column prop="provider" label="供应商" width="100" />
        <el-table-column prop="model" label="模型" width="150" />
        <el-table-column prop="max_iterations" label="最大迭代" width="80" />
        <el-table-column label="工具" min-width="200">
          <template #default="{ row }">
            <el-tag v-for="t in row.tools?.slice(0,3)" :key="t" size="small" class="mr1">{{ t }}</el-tag>
            <span v-if="row.tools?.length > 3">+{{ row.tools.length - 3 }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="80">
          <template #default="{ row }">
            <el-tag :type="row.status === 'running' ? 'success' : 'info'">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" title="编辑 Agent" width="600px">
      <el-form :model="currentAgent" label-width="100px" v-if="currentAgent">
        <el-form-item label="名称">
          <el-input v-model="currentAgent.name" :disabled="!!currentAgent.name" />
        </el-form-item>
        <el-form-item label="供应商">
          <el-input v-model="currentAgent.provider" />
        </el-form-item>
        <el-form-item label="模型">
          <el-input v-model="currentAgent.model" />
        </el-form-item>
        <el-form-item label="最大迭代">
          <el-input-number v-model="currentAgent.max_iterations" :min="1" :max="50" />
        </el-form-item>
        <el-form-item label="系统提示词">
          <el-input v-model="currentAgent.system_prompt" type="textarea" :rows="4" />
        </el-form-item>
        <el-form-item label="工具">
          <el-checkbox-group v-model="currentAgent.tools">
            <el-checkbox v-for="t in toolsOptions" :key="t" :value="t">{{ t }}</el-checkbox>
          </el-checkbox-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveAgent">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page { padding: 24px; }
.card-header { display: flex; justify-content: space-between; align-items: center; }
.mr1 { margin-right: 4px; }
</style>