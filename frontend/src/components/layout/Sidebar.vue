<script setup>
import { useRoute, useRouter } from 'vue-router'
import { computed } from 'vue'
import logoImage from '@/assets/logo.png'

const route = useRoute()
const router = useRouter()

const menuGroups = [
  {
    label: '对话',
    items: [
      { path: '/', icon: 'ChatDotRound', label: 'Chat' },
      // { path: '/inbox', icon: 'Bell', label: 'Inbox' },
    ]
  },
  {
    label: '控制',
    items: [
      { path: '/channels', icon: 'Connection', label: '渠道管理' },
      { path: '/sessions', icon: 'ChatLineSquare', label: '会话管理' },
      { path: '/cron-jobs', icon: 'Timer', label: '定时任务' },
    ]
  },
  {
    label: 'Agent',
    items: [
      { path: '/agent-config', icon: 'Setting', label: 'Agent 配置' },
      // { path: '/workspace', icon: 'Folder', label: '工作空间' },
      { path: '/skills', icon: 'MagicStick', label: '技能管理' },
      { path: '/tools', icon: 'SetUp', label: '工具列表' },
      // { path: '/mcp', icon: 'Link', label: 'MCP 集成' },
    ]
  },
  {
    label: '设置',
    items: [
      { path: '/models', icon: 'Cpu', label: '模型/供应商' },
      // { path: '/security', icon: 'Lock', label: '安全' },
      { path: '/debug', icon: 'Monitor', label: '调试/日志' },
    ]
  },
]

const activePath = computed(() => route.path)

function navigate(path) {
  router.push(path)
}
</script>

<template>
  <aside class="sidebar">
    <div class="sidebar-header" @click="navigate('/')">
      <img :src="logoImage" alt="go-claw" class="logo-icon" />
    </div>
    <div class="sidebar-menu">
      <template v-for="group in menuGroups" :key="group.label">
        <div class="menu-group-label">{{ group.label }}</div>
        <div
          v-for="item in group.items"
          :key="item.path"
          class="menu-item"
          :class="{ active: activePath === item.path }"
          @click="navigate(item.path)"
        >
          <el-icon><component :is="item.icon" /></el-icon>
          <span class="menu-item-text">{{ item.label }}</span>
        </div>
      </template>
    </div>
  </aside>
</template>

<style lang="scss" scoped>
.sidebar {
  width: 220px;
  background: #1d1e2b;
  color: #bfcbd9;
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  user-select: none;
}
.sidebar-header {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 8px 10px;
  cursor: pointer;
  border-bottom: 1px solid rgba(255,255,255,.08);
}
.logo-icon {
  width: 180px;
  max-height: 44px;
  object-fit: contain;
}
.sidebar-menu {
  flex: 1;
  overflow-y: auto;
  padding: 8px 0;
}
.menu-group-label {
  padding: 12px 20px 4px;
  font-size: 11px;
  text-transform: uppercase;
  color: rgba(255,255,255,.35);
  letter-spacing: 1px;
}
.menu-item {
  display: flex;
  align-items: center;
  padding: 10px 20px;
  cursor: pointer;
  transition: all .15s;
  &:hover { background: rgba(255,255,255,.06); color: #fff; }
  &.active { background: rgba(64,158,255,.15); color: #409eff; }
  .el-icon { margin-right: 10px; font-size: 18px; }
}
.menu-item-text { font-size: 14px; }
</style>
