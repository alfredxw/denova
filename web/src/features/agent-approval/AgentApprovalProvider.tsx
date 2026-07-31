import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Loader2, ShieldAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { fetchSettings, updateAgentApprovalMode } from '@/features/settings/api'
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

export function AgentApprovalProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const [mode, setModeState] = useState<AgentApprovalMode>(DEFAULT_AGENT_APPROVAL_MODE)
  const [initialized, setInitialized] = useState(false)
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)
  const [loadFailed, setLoadFailed] = useState(false)
  const [loadAttempt, setLoadAttempt] = useState(0)
  const savingRef = useRef(false)

  useEffect(() => {
    let active = true
    setInitialized(false)
    setLoading(true)
    setLoadFailed(false)
    void fetchSettings().then((settings) => {
      if (!active) return
      setModeState(resolveAgentApprovalMode(settings))
      setInitialized(true)
    }).catch((reason) => {
      if (!active) return
      console.error('[agent-approval] failed to load the user safety mode', reason)
      setInitialized(false)
      setLoadFailed(true)
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => { active = false }
  }, [loadAttempt])

  const persistMode = useCallback(async (next: AgentApprovalMode) => {
    if (savingRef.current) return false
    savingRef.current = true
    setSaving(true)
    try {
      const settings = await updateAgentApprovalMode(next)
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
      void fetchSettings().then((settings) => {
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
      <Dialog open={loadFailed}>
        <DialogContent
          showCloseButton={false}
          className="max-w-[min(calc(100vw-1.5rem),28rem)] gap-5 p-5 sm:p-6"
          onEscapeKeyDown={(event) => event.preventDefault()}
          onPointerDownOutside={(event) => event.preventDefault()}
          onInteractOutside={(event) => event.preventDefault()}
        >
          <DialogHeader className="gap-2">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-red-500/25 bg-red-500/10 text-red-500">
              <ShieldAlert className="h-5 w-5" />
            </div>
            <DialogTitle className="text-lg">{t('agentApproval.loadErrorTitle')}</DialogTitle>
            <DialogDescription className="leading-6">{t('agentApproval.loadFailed')}</DialogDescription>
          </DialogHeader>
          <DialogFooter className="-mx-5 -mb-5 px-5 sm:-mx-6 sm:-mb-6 sm:px-6">
            <Button type="button" disabled={loading} onClick={() => setLoadAttempt((attempt) => attempt + 1)}>
              {loading && <Loader2 className="h-4 w-4 animate-spin" />}
              {t('agentApproval.retry')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
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
