import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

export const useDebugStore = defineStore('debug', () => {
  const saved = localStorage.getItem('goclaw_log_autorefresh')
  const autoRefresh = ref(saved === 'true')

  // 变化时自动持久化到 localStorage
  watch(autoRefresh, (val) => {
    localStorage.setItem('goclaw_log_autorefresh', String(val))
  })

  return { autoRefresh }
})
