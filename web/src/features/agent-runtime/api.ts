import { jsonHeaders, requestJSON } from '@/lib/api-client'
import type { AgentEngineID, EngineDescriptor, EngineModels } from './types'

export const fetchAgentEngines = () => requestJSON<{ items: EngineDescriptor[] }>('/api/agent-runtimes')
export const fetchEngineModels = (id: AgentEngineID) => requestJSON<EngineModels>(`/api/agent-runtimes/${id}/models`)
export const checkAgentEngine = (id: AgentEngineID) => requestJSON<EngineDescriptor>(`/api/agent-runtimes/${id}/check`, {
  method: 'POST', headers: jsonHeaders, body: '{}',
})
