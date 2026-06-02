import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useAgentStore = defineStore('agent', () => {
  const savedAgent = localStorage.getItem('goclaw_selected_agent')
  const selectedAgent = ref(savedAgent || 'default')

  function setAgent(name) {
    selectedAgent.value = name
    localStorage.setItem('goclaw_selected_agent', name)
  }

  return { selectedAgent, setAgent }
})
