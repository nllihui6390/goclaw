<script setup>
import { ref, onMounted, onUnmounted, provide } from 'vue'
import Sidebar from '@/components/layout/Sidebar.vue'
import Header from '@/components/layout/Header.vue'

const collapsed = ref(false)    // 折叠为仅图标
const mobileOpen = ref(false)   // 手机模式浮层开关
const isMobile = ref(false)

function onResize() {
  const w = window.innerWidth
  isMobile.value = w < 768
  collapsed.value = w < 1024 && !isMobile.value
  if (w >= 768) mobileOpen.value = false
}

onMounted(() => {
  onResize()
  window.addEventListener('resize', onResize)
})
onUnmounted(() => window.removeEventListener('resize', onResize))

function toggleCollapse() {
  collapsed.value = !collapsed.value
}

function toggleMobile() {
  mobileOpen.value = !mobileOpen.value
}

provide('sidebarCollapsed', collapsed)
provide('isMobile', isMobile)
provide('toggleMobile', toggleMobile)
provide('toggleCollapse', toggleCollapse)
</script>

<template>
  <div class="app-layout">
    <!-- 手机模式遮罩层 -->
    <div
      v-if="isMobile && mobileOpen"
      class="mobile-overlay"
      @click="mobileOpen = false"
    />
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
.app-layout { display: flex; height: 100vh; }
.app-main { flex: 1; overflow: hidden; display: flex; flex-direction: column; min-width: 0; }
.app-content { flex: 1; overflow: auto; }

.mobile-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0,0,0,.4);
  z-index: 90;
}
</style>
