import { createContext, useContext, type ReactNode } from 'react'
import type { ConversationConfigBinding } from '@/features/conversation-config/types'

export type ToolNavigationTarget =
  | { kind: 'workspace_file'; path: string }
  | { kind: 'lore_item'; id?: string; name?: string }
  | { kind: 'config_resource'; resource: string; id?: string; scope?: string; section?: 'runtime'; conversation?: ConversationConfigBinding }

export interface ToolNavigationIntent {
  target: Exclude<ToolNavigationTarget, { kind: 'workspace_file' }>
  nonce: number
}

export interface ToolNavigationValue {
  workspace: string
  open: (target: ToolNavigationTarget) => void
}

const ToolNavigationContext = createContext<ToolNavigationValue | null>(null)

export function ToolNavigationProvider({ value, children }: { value: ToolNavigationValue; children: ReactNode }) {
  return <ToolNavigationContext.Provider value={value}>{children}</ToolNavigationContext.Provider>
}

export function useToolNavigation() {
  return useContext(ToolNavigationContext)
}
