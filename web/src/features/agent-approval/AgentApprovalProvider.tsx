import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Check, Loader2, PencilLine, ShieldQuestion, TriangleAlert, Zap } from 'lucide-react'
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
import type { AgentApprovalMode } from '@/features/settings/types'

interface AgentApprovalContextValue {
  mode: AgentApprovalMode
  initialized: boolean
  saving: boolean
  setMode: (mode: AgentApprovalMode) => Promise<boolean>
}

const AgentApprovalContext = createContext<AgentApprovalContextValue>({
  mode: 'ask',
  initialized: false,
  saving: false,
  setMode: async () => false,
})

const modes: Array<{
  id: AgentApprovalMode
  icon: typeof ShieldQuestion
  tone: string
}> = [
  { id: 'ask', icon: ShieldQuestion, tone: 'text-amber-500' },
  { id: 'write', icon: PencilLine, tone: 'text-emerald-500' },
  { id: 'yolo', icon: Zap, tone: 'text-red-500' },
]

export function AgentApprovalProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const [mode, setModeState] = useState<AgentApprovalMode>('ask')
  const [choice, setChoice] = useState<AgentApprovalMode>('write')
  const [initialized, setInitialized] = useState(false)
  const [mustChoose, setMustChoose] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const savingRef = useRef(false)

  useEffect(() => {
    let active = true
    void fetchSettings().then((settings) => {
      if (!active) return
      const userMode = validMode(settings.user?.agent_approval_mode)
      const effectiveMode = validMode(settings.effective?.agent_approval_mode) || 'ask'
      setModeState(userMode || effectiveMode)
      setChoice(userMode || 'write')
      setMustChoose(!userMode)
      setInitialized(true)
      setError('')
    }).catch((reason) => {
      if (!active) return
      console.error('[agent-approval] failed to load the user safety mode', reason)
      setModeState('ask')
      setChoice('write')
      setMustChoose(true)
      setInitialized(false)
      setError(t('agentApproval.loadFailed'))
    })
    return () => { active = false }
  }, [t])

  const persistMode = useCallback(async (next: AgentApprovalMode) => {
    if (savingRef.current) return false
    savingRef.current = true
    setSaving(true)
    setError('')
    try {
      const settings = await updateAgentApprovalMode(next)
      const saved = validMode(settings.user?.agent_approval_mode)
      if (!saved) throw new Error('server response did not include the saved user mode')
      setModeState(saved)
      setChoice(saved)
      setMustChoose(false)
      setInitialized(true)
      window.dispatchEvent(new CustomEvent('nova:settings-updated', { detail: { source: 'agent-approval-provider' } }))
      return true
    } catch (reason) {
      console.error('[agent-approval] failed to save the user safety mode', reason)
      setError(t('agentApproval.saveFailed'))
      return false
    } finally {
      savingRef.current = false
      setSaving(false)
    }
  }, [t])

  useEffect(() => {
    const reloadExternalChange = (event: Event) => {
      const source = (event as CustomEvent<{ source?: string }>).detail?.source
      if (source === 'agent-approval-provider') return
      void fetchSettings().then((settings) => {
        const saved = validMode(settings.user?.agent_approval_mode)
        if (!saved) return
        setModeState(saved)
        setChoice(saved)
        setMustChoose(false)
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
      <Dialog open={mustChoose}>
        <DialogContent
          showCloseButton={false}
          className="max-w-[min(calc(100vw-1.5rem),48rem)] gap-5 p-5 sm:p-6"
          onEscapeKeyDown={(event) => event.preventDefault()}
          onPointerDownOutside={(event) => event.preventDefault()}
          onInteractOutside={(event) => event.preventDefault()}
        >
          <DialogHeader className="gap-2">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text)]">
              <ShieldQuestion className="h-5 w-5" />
            </div>
            <DialogTitle className="text-lg">{t('agentApproval.onboarding.title')}</DialogTitle>
            <DialogDescription className="max-w-2xl leading-6">
              {t('agentApproval.onboarding.description')}
            </DialogDescription>
          </DialogHeader>

          <div role="radiogroup" aria-label={t('agentApproval.onboarding.title')} className="grid gap-2.5 md:grid-cols-3">
            {modes.map(({ id, icon: Icon, tone }) => {
              const selected = choice === id
              return (
                <button
                  key={id}
                  type="button"
                  role="radio"
                  aria-checked={selected}
                  disabled={saving}
                  onClick={() => setChoice(id)}
                  className={`relative min-h-44 rounded-xl border p-4 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--nova-accent)] disabled:opacity-60 ${
                    selected
                      ? 'border-[var(--nova-text-muted)] bg-[var(--nova-hover)]'
                      : 'border-[var(--nova-border)] bg-[var(--nova-surface)] hover:bg-[var(--nova-surface-2)]'
                  }`}
                >
                  {selected && <Check className="absolute right-3 top-3 h-4 w-4 text-[var(--nova-text)]" />}
                  <Icon className={`mb-3 h-5 w-5 ${tone}`} />
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium text-[var(--nova-text)]">{t(`agentApproval.mode.${id}.label`)}</span>
                    {id === 'write' && (
                      <span className="rounded-full border border-emerald-500/30 bg-emerald-500/10 px-1.5 py-0.5 text-[10px] font-medium text-emerald-600 dark:text-emerald-400">
                        {t('agentApproval.recommended')}
                      </span>
                    )}
                  </div>
                  <p className="mb-0 mt-2 text-xs leading-5 text-[var(--nova-text-muted)]">
                    {t(`agentApproval.mode.${id}.description`)}
                  </p>
                </button>
              )
            })}
          </div>

          <div className="flex items-start gap-2 rounded-lg border border-amber-500/25 bg-amber-500/8 px-3 py-2.5 text-xs leading-5 text-[var(--nova-text-muted)]">
            <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0 text-amber-500" />
            <span>{t('agentApproval.guardrailNotice')}</span>
          </div>
          {error && <p role="alert" className="m-0 text-xs text-red-500">{error}</p>}

          <DialogFooter className="-mx-5 -mb-5 px-5 sm:-mx-6 sm:-mb-6 sm:px-6">
            <Button type="button" disabled={saving} onClick={() => void persistMode(choice)}>
              {saving && <Loader2 className="h-4 w-4 animate-spin" />}
              {saving ? t('agentApproval.saving') : t('agentApproval.onboarding.continue')}
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

function validMode(value: unknown): AgentApprovalMode | null {
  return value === 'ask' || value === 'write' || value === 'yolo' ? value : null
}
