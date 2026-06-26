<script setup>
import { ref, onMounted, onUnmounted, provide } from 'vue'
import { useThemeStore } from '@/stores/theme'
import Sidebar from '@/components/layout/Sidebar.vue'
import Header from '@/components/layout/Header.vue'

const themeStore = useThemeStore()

const collapsed = ref(false)
const mobileOpen = ref(false)
const isMobile = ref(false)

function onResize() {
  const w = window.innerWidth
  isMobile.value = w < 768
  collapsed.value = w < 1024 && !isMobile.value
  if (w >= 768) mobileOpen.value = false
}

onMounted(() => {
  // Theme already applied in index.html before paint;
  // only set up listeners and skin here.
  themeStore.init()
  onResize()
  window.addEventListener('resize', onResize)
})
onUnmounted(() => window.removeEventListener('resize', onResize))

function toggleCollapse() { collapsed.value = !collapsed.value }
function toggleMobile() { mobileOpen.value = !mobileOpen.value }

provide('sidebarCollapsed', collapsed)
provide('isMobile', isMobile)
provide('toggleMobile', toggleMobile)
provide('toggleCollapse', toggleCollapse)
</script>

<template>
  <div class="app-layout">
    <div v-if="isMobile && mobileOpen" class="mobile-overlay" @click="mobileOpen = false" />

    <Sidebar
      :class="{
        collapsed: collapsed && !isMobile,
        'mobile-open': isMobile && mobileOpen,
        'mobile-hidden': isMobile && !mobileOpen
      }"
    />

    <div class="app-main">
      <Header />
      <div class="app-content">
        <router-view />
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
@use '@/styles/variables.scss' as *;

.app-layout { display: flex; height: 100vh; overflow: hidden; }
.app-main { flex: 1; overflow: hidden; display: flex; flex-direction: column; min-width: 0; }
.app-content { flex: 1; overflow: auto; background: $bg-deep; transition: background 0.3s; }

.mobile-overlay {
  position: fixed; inset: 0; background: rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(4px); z-index: 90; animation: fade-in 0.2s ease-out;
}

@keyframes fade-in { from { opacity: 0; } to { opacity: 1; } }
</style>