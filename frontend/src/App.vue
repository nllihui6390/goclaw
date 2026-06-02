<script setup>
import Sidebar from '@/components/layout/Sidebar.vue'
import Header from '@/components/layout/Header.vue'
</script>

<template>
  <div class="app-layout">
    <Sidebar />
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
.app-main { flex: 1; overflow: hidden; display: flex; flex-direction: column; }
.app-content { flex: 1; overflow: auto; }
</style>