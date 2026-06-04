import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useAgentStore = defineStore('agent', () => {
  const savedAgent = localStorage.getItem('goclaw_selected_agent')
  const selectedAgent = ref(savedAgent || 'default')
  const agentList = ref([]) // 共享的 agent 列表，供 Header 和其他组件使用

  function setAgent(name) {
    selectedAgent.value = name
    localStorage.setItem('goclaw_selected_agent', name)
  }

  function setAgentList(list) {
    agentList.value = list || []
  }

  return { selectedAgent, agentList, setAgent, setAgentList }
})
