import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { fetchSettings, patchSettings, refreshSettings } from '@/features/settings/api'
import type { LayeredSettings } from '@/features/settings/types'
import {
  DEFAULT_AGENT_APPROVAL_MODE,
  normalizeAgentApprovalMode,
  type AgentApprovalMode,
} from './modes'

interface AgentApprovalContextValue {
  mode: AgentApprovalMode
  initialized: boolean
  saving: boolean
  setMode: (mode: AgentApprovalMode) => Promise<boolean>
}

const AgentApprovalContext = createContext<AgentApprovalContextValue>({
  mode: DEFAULT_AGENT_APPROVAL_MODE,
  initialized: false,
  saving: false,
  setMode: async () => false,
})

const LOAD_FAILURE_APPROVAL_MODE: AgentApprovalMode = 'ask'

export function AgentApprovalProvider({ children }: { children: ReactNode }) {
  const [mode, setModeState] = useState<AgentApprovalMode>(DEFAULT_AGENT_APPROVAL_MODE)
  const [initialized, setInitialized] = useState(false)
  const [saving, setSaving] = useState(false)
  const savingRef = useRef(false)

  useEffect(() => {
    let active = true
    setInitialized(false)
    void fetchSettings().then((settings) => {
      if (!active) return
      setModeState(resolveAgentApprovalMode(settings))
      setInitialized(true)
    }).catch((reason) => {
      if (!active) return
      console.error('[features/agent-approval/AgentApprovalProvider.tsx] failed to load the user safety mode; falling back to Ask', reason)
      setModeState(LOAD_FAILURE_APPROVAL_MODE)
      setInitialized(true)
    })
    return () => { active = false }
  }, [])

  const persistMode = useCallback(async (next: AgentApprovalMode) => {
    if (savingRef.current) return false
    savingRef.current = true
    setSaving(true)
    try {
      const settings = await patchSettings('user', { agent_approval_mode: next })
      const saved = normalizeAgentApprovalMode(settings.user?.agent_approval_mode)
        || normalizeAgentApprovalMode(settings.effective?.agent_approval_mode)
      if (!saved) throw new Error('server response did not include the saved user mode')
      setModeState(saved)
      setInitialized(true)
      window.dispatchEvent(new CustomEvent('nova:settings-updated', { detail: { source: 'agent-approval-provider' } }))
      return true
    } catch (reason) {
      console.error('[agent-approval] failed to save the user safety mode', reason)
      return false
    } finally {
      savingRef.current = false
      setSaving(false)
    }
  }, [])

  useEffect(() => {
    const reloadExternalChange = (event: Event) => {
      const source = (event as CustomEvent<{ source?: string }>).detail?.source
      if (source === 'agent-approval-provider') return
      void refreshSettings().then((settings) => {
        setModeState(resolveAgentApprovalMode(settings))
        setInitialized(true)
      }).catch((reason) => {
        console.warn('[agent-approval] failed to refresh the safety mode after a settings update', reason)
      })
    }
    window.addEventListener('nova:settings-updated', reloadExternalChange)
    return () => window.removeEventListener('nova:settings-updated', reloadExternalChange)
  }, [])

  const value = useMemo<AgentApprovalContextValue>(() => ({
    mode,
    initialized,
    saving,
    setMode: persistMode,
  }), [initialized, mode, persistMode, saving])

  return (
    <AgentApprovalContext.Provider value={value}>
      {children}
    </AgentApprovalContext.Provider>
  )
}

export function useAgentApprovalMode() {
  return useContext(AgentApprovalContext)
}

function resolveAgentApprovalMode(settings: Pick<LayeredSettings, 'user' | 'effective'>): AgentApprovalMode {
  return normalizeAgentApprovalMode(settings.user?.agent_approval_mode)
    || normalizeAgentApprovalMode(settings.effective?.agent_approval_mode)
    || DEFAULT_AGENT_APPROVAL_MODE
}
