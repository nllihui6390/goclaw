export const HOME_PATH = '/chat'

const MENU_GROUP_ORDER = ['对话', '控制', 'Agent', '设置']

export const appRoutes = [
  { path: '/', redirect: HOME_PATH },
  {
    path: '/chat',
    name: 'Chat',
    component: () => import('@/views/chat/Chat.vue'),
    meta: { title: '聊天', menuLabel: '聊天', icon: 'ChatDotRound', menuGroup: '对话' }
  },
  {
    path: '/inbox',
    name: 'Inbox',
    component: () => import('@/views/inbox/Inbox.vue'),
    meta: { title: '收件箱' }
  },
  {
    path: '/channels',
    name: 'Channels',
    component: () => import('@/views/control/Channels.vue'),
    meta: { title: '频道管理', menuLabel: '频道管理', icon: 'Connection', menuGroup: '控制' }
  },
  {
    path: '/sessions',
    name: 'Sessions',
    component: () => import('@/views/control/Sessions.vue'),
    meta: { title: '会话管理', menuLabel: '会话管理', icon: 'ChatLineSquare', menuGroup: '控制' }
  },
  {
    path: '/cron-jobs',
    name: 'CronJobs',
    component: () => import('@/views/control/CronJobs.vue'),
    meta: { title: '定时任务', menuLabel: '定时任务', icon: 'Timer', menuGroup: '控制' }
  },
  {
    path: '/agent-config',
    name: 'AgentConfig',
    component: () => import('@/views/agent/AgentConfig.vue'),
    meta: { title: 'Agent 配置', menuLabel: 'Agent配置', icon: 'Setting', menuGroup: 'Agent' }
  },
  {
    path: '/workspace',
    name: 'Workspace',
    component: () => import('@/views/agent/Workspace.vue'),
    meta: { title: '工作空间' }
  },
  {
    path: '/skills',
    name: 'Skills',
    component: () => import('@/views/agent/Skills.vue'),
    meta: { title: '技能管理', menuLabel: '技能管理', icon: 'MagicStick', menuGroup: 'Agent' }
  },
  {
    path: '/files',
    name: 'Files',
    component: () => import('@/views/agent/FileManager.vue'),
    meta: { title: '文件管理', menuLabel: '文件管理', icon: 'Document', menuGroup: 'Agent' }
  },
  {
    path: '/tools',
    name: 'Tools',
    component: () => import('@/views/agent/Tools.vue'),
    meta: { title: '工具列表', menuLabel: '工具管理', icon: 'SetUp', menuGroup: 'Agent' }
  },
  {
    path: '/mcp',
    name: 'Mcp',
    component: () => import('@/views/agent/Mcp.vue'),
    meta: { title: 'MCP 集成' }
  },
  {
    path: '/models',
    name: 'Models',
    component: () => import('@/views/settings/Models.vue'),
    meta: { title: '模型供应商', menuLabel: '模型管理', icon: 'Cpu', menuGroup: '设置' }
  },
  {
    path: '/security',
    name: 'Security',
    component: () => import('@/views/settings/Security.vue'),
    meta: { title: '安全设置', menuLabel: '安全设置', icon: 'Lock', menuGroup: '设置' }
  },
  {
    path: '/env-vars',
    name: 'EnvVars',
    component: () => import('@/views/settings/EnvVars.vue'),
    meta: { title: '环境变量', menuLabel: '环境变量', icon: 'Key', menuGroup: '设置' }
  },
  {
    path: '/debug',
    name: 'Debug',
    component: () => import('@/views/settings/Debug.vue'),
    meta: { title: '调试日志', menuLabel: '调试日志', icon: 'Monitor', menuGroup: '设置' }
  },

]

export function buildMenuGroups() {
  const groups = new Map()
  const groupOrder = []

  for (const route of appRoutes) {
    const { menuGroup, menuLabel, title, icon } = route.meta || {}
    if (!menuGroup) continue

    if (!groups.has(menuGroup)) {
      groups.set(menuGroup, { label: menuGroup, items: [] })
      groupOrder.push(menuGroup)
    }

    groups.get(menuGroup).items.push({
      path: route.path,
      icon,
      label: menuLabel || title
    })
  }

  return MENU_GROUP_ORDER
    .filter((label) => groups.has(label))
    .map((label) => groups.get(label))
}

export function getPageTitle(path) {
  const route = appRoutes.find((r) => r.path === path)
  return route?.meta?.title
}
