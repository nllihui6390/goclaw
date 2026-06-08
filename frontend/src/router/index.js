import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/', redirect: '/chat' },
  { path: '/chat', name: 'Chat', component: () => import('@/views/chat/Chat.vue') },
  { path: '/inbox', name: 'Inbox', component: () => import('@/views/inbox/Inbox.vue') },
  { path: '/channels', name: 'Channels', component: () => import('@/views/control/Channels.vue') },
  { path: '/sessions', name: 'Sessions', component: () => import('@/views/control/Sessions.vue') },
  { path: '/cron-jobs', name: 'CronJobs', component: () => import('@/views/control/CronJobs.vue') },
  { path: '/agent-config', name: 'AgentConfig', component: () => import('@/views/agent/AgentConfig.vue') },
  { path: '/workspace', name: 'Workspace', component: () => import('@/views/agent/Workspace.vue') },
  { path: '/skills', name: 'Skills', component: () => import('@/views/agent/Skills.vue') },
  { path: '/files', name: 'Files', component: () => import('@/views/agent/FileManager.vue') },
  { path: '/tools', name: 'Tools', component: () => import('@/views/agent/Tools.vue') },
  { path: '/mcp', name: 'Mcp', component: () => import('@/views/agent/Mcp.vue') },
  { path: '/models', name: 'Models', component: () => import('@/views/settings/Models.vue') },
  { path: '/security', name: 'Security', component: () => import('@/views/settings/Security.vue') },
  { path: '/debug', name: 'Debug', component: () => import('@/views/settings/Debug.vue') },

]

const router = createRouter({
  history: createWebHistory(),
  routes
})

export default router
