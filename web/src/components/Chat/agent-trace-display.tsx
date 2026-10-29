import { createContext, useContext } from 'react'
import type { AgentTraceDisplay } from '@/features/settings/types'

/** User preference only; manual disclosures remain local to each conversation. */
const AgentTraceDisplayContext = createContext<AgentTraceDisplay>('compact')
export const AgentTraceDisplayProvider = AgentTraceDisplayContext.Provider
export const useAgentTraceDisplay = () => useContext(AgentTraceDisplayContext)
