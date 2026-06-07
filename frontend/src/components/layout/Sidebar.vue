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
      { path: '/channels', icon: 'Connection', label: '频道管理' },
      { path: '/sessions', icon: 'ChatLineSquare', label: '会话管理' },
      { path: '/cron-jobs', icon: 'Timer', label: '定时任务' },
    ]
  },
  {
    label: 'Agent',
    items: [
      { path: '/agent-config', icon: 'Setting', label: 'Agent配置' },
      { path: '/skills', icon: 'MagicStick', label: '技能管理' },
      { path: '/tools', icon: 'SetUp', label: '工具管理' },
      { path: '/files', icon: 'Document', label: '文件管理' },
    ]
  },
  {
    label: '设置',
    items: [
      { path: '/models', icon: 'Cpu', label: '模型管理' },
      { path: '/debug', icon: 'Monitor', label: '调试日志' },
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
    <!-- Logo header -->
    <div class="sidebar-header" @click="navigate('/')">
      <img :src="currentLogo" alt="go-claw" class="logo-icon" />
    </div>

    <!-- Navigation menu -->
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
          <el-icon class="menu-icon"><component :is="item.icon" /></el-icon>
          <span class="menu-item-text">{{ item.label }}</span>
        </div>
      </template>
    </div>

    <!-- Footer branding -->
    <div class="sidebar-footer" v-if="!sidebarCollapsed">
      <div class="footer-line"></div>
      <span class="footer-text">v1.0</span>
    </div>
  </aside>
</template>

<style lang="scss" scoped>
@use '@/styles/variables.scss' as *;

.sidebar {
  width: $sidebar-width;
  background: $bg-surface;
  color: $text-secondary;
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  user-select: none;
  transition: width 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  z-index: 100;
  border-right: 1px solid $border-subtle;

  // ──── Collapsed mode ────
  &.collapsed {
    width: $sidebar-collapsed;
    .logo-icon { width: 36px; }
    .logo-text { display: none; }
    .sidebar-header { padding: 16px 18px; justify-content: center; }
    .menu-group-label {
      text-align: center;
      font-size: 10px;
    }
    .menu-item { padding: 12px 20px; justify-content: center; }
    .menu-item-text { display: none; }
    .menu-icon { margin-right: 0; }
    .sidebar-footer { display: none; }
  }

  // ──── Mobile hidden ────
  &.mobile-hidden {
    width: 0;
    overflow: hidden;
    border-right: none;
  }

  // ──── Mobile overlay ────
  &.mobile-open {
    position: fixed;
    top: 0;
    left: 0;
    bottom: 0;
    z-index: 100;
    box-shadow: 8px 0 30px rgba(0, 0, 0, 0.5);
    border-right: 1px solid $border-default;
  }
}

// ──── Logo ────
.sidebar-header {
  height: 68px;
  display: flex;
  align-items: center;
  padding: 8px 20px;
  cursor: pointer;
  border-bottom: 1px solid $border-subtle;
  transition: all 0.3s;
  gap: 12px;
  justify-content: center;
  &:hover {
    background: $accent-cyan-dim;
  }
}

.logo-icon {
  width: 180px;
  max-height: 50px;
  object-fit: contain;
  transition: width 0.3s;
}

.logo-text {
  font-family: $font-display;
  font-size: $font-size-lg;
  font-weight: 700;
  color: $accent-cyan;
}

// ──── Menu ────
.sidebar-menu {
  flex: 1;
  overflow-y: auto;
  padding: 12px 0;
}

.menu-group-label {
  padding: 16px 24px 6px;
  font-size: $font-size-xs;
  font-family: $font-display;
  text-transform: uppercase;
  color: $text-muted;
  letter-spacing: 1.5px;
  transition: all 0.3s;
  white-space: nowrap;
  overflow: hidden;
}

.menu-item {
  display: flex;
  align-items: center;
  padding: 12px 24px;
  margin: 2px 12px;
  border-radius: $radius-md;
  cursor: pointer;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  white-space: nowrap;
  position: relative;

  .el-icon {
    margin-right: 12px;
    font-size: 18px;
    flex-shrink: 0;
    color: $text-secondary;
    transition: color 0.2s, transform 0.2s;
  }

  &:hover {
    background: $bg-glass-light;
    color: $text-primary;

    .el-icon { color: $text-primary; transform: scale(1.1); }
  }

  &.active {
    background: $accent-cyan-dim;
    color: $accent-cyan;

    .el-icon { color: $accent-cyan; }
    .menu-item-text { font-weight: 500; }

    // 左侧指示条
    &::before {
      content: '';
      position: absolute;
      left: 0;
      top: 50%;
      transform: translateY(-50%);
      width: 3px;
      height: 20px;
      background: $accent-cyan;
      border-radius: 0 3px 3px 0;
      box-shadow: 0 0 8px rgba(0, 212, 255, 0.4);
    }
  }
}

.menu-item-text {
  font-size: $font-size-base;
  font-family: $font-ui;
}

// ──── Footer ────
.sidebar-footer {
  padding: 16px 24px;
  border-top: 1px solid $border-subtle;
}

.footer-line {
  width: 40px;
  height: 1px;
  background: linear-gradient(90deg, $accent-cyan, transparent);
  margin-bottom: 8px;
}

.footer-text {
  font-family: $font-display;
  font-size: $font-size-xs;
  color: $text-muted;
}
</style>