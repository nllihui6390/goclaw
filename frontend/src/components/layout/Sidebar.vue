<script setup>
import { useRoute, useRouter } from 'vue-router'
import { computed, inject } from 'vue'
import logoImage from '@/assets/logo.png'
import logo1Image from '@/assets/logo1.png'

const route = useRoute()
const router = useRouter()
const sidebarCollapsed = inject('sidebarCollapsed')

const currentLogo = computed(() => sidebarCollapsed.value ? logoImage : logo1Image)

const menuGroups = [
  {
    label: '对话',
    items: [
      { path: '/', icon: 'ChatDotRound', label: '聊天' },
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
      { path: '/skills', icon: 'MagicStick', label: '技能管理' },
      { path: '/tools', icon: 'SetUp', label: '工具列表' },
    ]
  },
  {
    label: '设置',
    items: [
      { path: '/models', icon: 'Cpu', label: '模型/供应商' },
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
      <img :src="currentLogo" alt="go-claw" class="logo-icon" />
    </div>
    <div class="sidebar-menu">
      <template v-for="group in menuGroups" :key="group.label">
        <div class="menu-group-label">{{ group.label }}</div>
        <div
          v-for="item in group.items"
          :key="item.path"
          class="menu-item"
          :class="{ active: activePath === item.path }"
          :title="item.label"
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
  transition: width .2s;
  z-index: 100;

  // 折叠模式：仅图标
  &.collapsed {
    width: 64px;
    .logo-icon { width: 40px; }
    .sidebar-header { padding: 8px 12px; }
    .menu-group-label { text-align: center; font-size: 0; &::first-letter { font-size: 11px; } }
    .menu-item { padding: 10px 18px; justify-content: center; }
    .menu-item-text { display: none; }
    .menu-item .el-icon { margin-right: 0; }
  }

  // 手机模式：隐藏
  &.mobile-hidden {
    width: 0;
    overflow: hidden;
  }

  // 手机模式展开：浮层
  &.mobile-open {
    position: fixed;
    top: 0;
    left: 0;
    bottom: 0;
    z-index: 100;
    box-shadow: 4px 0 12px rgba(0,0,0,.3);
  }
}
.sidebar-header {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 8px 10px;
  cursor: pointer;
  border-bottom: 1px solid rgba(255,255,255,.08);
  transition: padding .2s;
}
.logo-icon {
  width: 180px;
  max-height: 44px;
  object-fit: contain;
  transition: width .2s;
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
  transition: all .2s;
  white-space: nowrap;
  overflow: hidden;
}
.menu-item {
  display: flex;
  align-items: center;
  padding: 10px 20px;
  cursor: pointer;
  transition: all .15s;
  white-space: nowrap;
  &:hover { background: rgba(255,255,255,.06); color: #fff; }
  &.active { background: rgba(64,158,255,.15); color: #409eff; }
  .el-icon { margin-right: 10px; font-size: 18px; flex-shrink: 0; }
}
.menu-item-text { font-size: 14px; }
</style>
