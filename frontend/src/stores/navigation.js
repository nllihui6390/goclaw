import { defineStore } from 'pinia'
import { computed } from 'vue'
import { buildMenuGroups, getPageTitle } from '@/router/routes'

export const useNavigationStore = defineStore('navigation', () => {
  const menuGroups = computed(() => buildMenuGroups())

  function pageTitle(path) {
    return getPageTitle(path) || 'go-claw'
  }

  return { menuGroups, pageTitle }
})
