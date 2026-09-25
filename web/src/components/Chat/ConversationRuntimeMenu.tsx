import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { errorMessage } from '@/lib/error-diagnostics'
import { useEffect, useState } from 'react'
import { Check, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { DropdownMenuGroup, DropdownMenuItem, DropdownMenuSub, DropdownMenuSubContent, DropdownMenuSubTrigger } from '@/components/ui/dropdown-menu'
import { checkAgentEngine, fetchAgentEngines, fetchEngineModels } from '@/features/agent-runtime/api'
import { runtimeProfileOptions } from '@/features/agent-runtime/api-profiles'
import type { AgentEngineID, RuntimeSelection } from '@/features/agent-runtime/types'
import type { ConversationConfigController } from '@/features/conversation-config/types'
import { fetchModelCatalog, fetchProjectSettings, fetchSettings } from '@/features/settings/api'
import { modelProfilesWithDefault } from '@/features/settings/model-profiles'
import { useIsMobile } from '@/hooks/useIsMobile'

type RuntimeChoice = { kind: AgentEngineID; selection?: RuntimeSelection; reason?: string; unchecked?: boolean }

/** Resolves effective defaults without changing Agent preferences. API profiles do
 * not require CLI authentication; the server validates admission before commit. */
async function loadChoices(controller: ConversationConfigController): Promise<RuntimeChoice[]> {
  const projectID = controller.binding?.project_id
  const [settings, { items }] = await Promise.all([
    projectID ? fetchProjectSettings(projectID) : fetchSettings(), fetchAgentEngines(),
  ])
  const { effective } = settings
  const snapshot = controller.snapshot
  const agentKind = snapshot?.agent_kind
  const preferences = snapshot?.custom_agent_id
    ? effective.custom_agents?.find(agent => agent.id === snapshot.custom_agent_id)?.runtime
    : agentKind === 'ide' || agentKind === 'general' || agentKind === 'interactive_story'
      ? effective.agent_runtimes?.[agentKind] : undefined
  return Promise.all((['native', 'codex', 'claude'] as const).map(async (kind): Promise<RuntimeChoice> => {
    try {
      if (kind === 'native') {
        const profile = modelProfilesWithDefault(effective).find(item => item.id === (snapshot?.profile_id || 'default'))
        return profile?.model ? { kind, selection: { kind } } : { kind, reason: 'agentRuntime.modelUnavailable' }
      }
      const engine = items.find(item => item.id === kind)
      const status = engine?.status ?? 'unchecked'
      let model = preferences?.[kind]
      if (status === 'unchecked') return { kind, reason: 'agentRuntime.status.unchecked', unchecked: true }
      if (status !== 'ready' && !(model?.profile_id && status === 'auth_required')) {
        return { kind, reason: engine?.reason_key || `agentRuntime.status.${status}` }
      }
      if (model?.profile_id) {
        const profileID = model.profile_id
        const catalog = await fetchModelCatalog()
        if (!runtimeProfileOptions(kind, settings, catalog).some(item => item.id === `profile:${profileID}`)) {
          return { kind, reason: 'agentRuntime.apiProfileUnavailable' }
        }
      } else {
        const catalog = await fetchEngineModels(kind)
        const modelID = model?.model || catalog.default_id || catalog.items[0]?.id
        const found = catalog.items.find(item => item.id === modelID)
        if (!found || (model?.effort && !found.efforts.includes(model.effort))) return { kind, reason: 'agentRuntime.modelUnavailable' }
        model = model || { model: found.id }
      }
      return { kind, selection: kind === 'codex' ? { kind, codex: model } : { kind, claude: model } }
    } catch (cause) {
      console.warn('[conversation-config] load runtime choice failed', { kind, cause })
      return { kind, reason: 'agentRuntime.connectionFailed' }
    }
  }))
}

/** Model-menu selector. Opening or checking never switches the bound session. */
export function ConversationRuntimeMenu({ controller, runActive, disabled = false, onConfigure, configurationDisabled = false }: {
  controller: ConversationConfigController
  runActive: boolean
  disabled?: boolean
  onConfigure: () => void
  configurationDisabled?: boolean
}) {
  const { t } = useTranslation()
  const narrow = useIsMobile('(max-width: 700px)')
  const [open, setOpen] = useState(false)
  const [choices, setChoices] = useState<RuntimeChoice[]>([])
  const [loading, setLoading] = useState(false)
  const [pending, setPending] = useState<AgentEngineID | null>(null)
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const current = controller.snapshot?.runtime?.kind ?? 'native'
  const draftStory = controller.binding?.mode === 'interactive' && !controller.binding.story_id
  const selectionDisabled = disabled || !controller.initialized || controller.loading || runActive || controller.saving || Boolean(pending) || loading || draftStory

  useEffect(() => {
    if (!open) return
    let active = true
    setLoading(true)
    setError('')
    void loadChoices(controller).then(next => { if (active) setChoices(next) }).catch(cause => {
      console.warn('[conversation-config] load runtime choices failed', { cause })
      if (active) { setChoices([]); setError(errorMessage(cause, t('agentRuntime.connectionFailed'))) }
    }).finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [open, revision, controller.binding, controller.snapshot?.custom_agent_id, controller.snapshot?.agent_kind, controller.snapshot?.profile_id, t])

  const act = async (choice: RuntimeChoice) => {
    if (selectionDisabled || choice.kind === current) return
    setPending(choice.kind)
    setError('')
    try {
      if (choice.unchecked) {
        try { await checkAgentEngine(choice.kind) } finally { setRevision(value => value + 1) }
      } else if (choice.selection) {
        // Re-resolve on submission so changed settings cannot apply stale defaults.
        const fresh = (await loadChoices(controller)).find(item => item.kind === choice.kind)
        if (!fresh?.selection) {
          setError(t(fresh?.reason || 'agentRuntime.notReady'))
          setRevision(value => value + 1)
          return
        }
        if (await controller.patch({ runtime: fresh.selection })) {
          console.info('[conversation-config] runtime switched', { from: current, to: choice.kind, binding: controller.binding })
          setOpen(false)
        }
      }
    } catch (cause) {
      console.warn('[conversation-config] switch or check runtime failed', { kind: choice.kind, cause })
      setError(errorMessage(cause, t('agentRuntime.connectionFailed')))
    } finally { setPending(null) }
  }

  return <DropdownMenuGroup className="flex min-w-0 items-center gap-1">
    <DropdownMenuSub open={open} onOpenChange={setOpen}>
      <DropdownMenuSubTrigger className="min-w-0 flex-1 cursor-pointer text-xs" disabled={disabled || !controller.initialized}>
        <span className="truncate">{t('agentRuntime.currentRuntime', { runtime: t(`agentRuntime.${current}`) })}</span>
      </DropdownMenuSubTrigger>
      {/* Narrow screens overlay the parent menu to keep all choices in view. */}
      <DropdownMenuSubContent avoidCollisions={!narrow} className="max-h-(--radix-dropdown-menu-content-available-height) overflow-y-auto w-64 max-w-[calc(100vw-1rem)] border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-1.5 text-[var(--nova-text)] max-[700px]:w-56 max-[700px]:[translate:calc(-1*var(--radix-popper-anchor-width))_0]">
        <DropdownMenuGroup>
          {(loading ? [] : choices).map(choice => <div key={choice.kind}>
            <DropdownMenuItem disabled={selectionDisabled || choice.kind === current || (!choice.selection && !choice.unchecked)}
              aria-label={t(`agentRuntime.${choice.kind}`)} aria-current={choice.kind === current ? 'true' : undefined}
              onSelect={event => { event.preventDefault(); void act(choice) }}>
              <span className="min-w-0 flex-1">{t(`agentRuntime.${choice.kind}`)}</span>
              {pending === choice.kind ? <Loader2 className="animate-spin" /> : choice.kind === current ? <Check /> : choice.unchecked ? <span className="text-xs">{t('agentRuntime.check')}</span> : null}
            </DropdownMenuItem>
            {choice.reason && <p className="px-1.5 pb-1 text-xs text-muted-foreground">{t(choice.reason)}</p>}
          </div>)}
          {loading && <p role="status" className="px-1.5 py-1 text-xs">{t('agentRuntime.checking')}</p>}
          <DropdownMenuItem disabled={configurationDisabled} onSelect={onConfigure}>{t('agentRuntime.configure')}</DropdownMenuItem>
        </DropdownMenuGroup>
        <p className="whitespace-normal px-1.5 py-1 text-[11px] text-muted-foreground">
          {t(draftStory ? 'agentRuntime.switchAfterStoryCreated' : runActive ? 'agentRuntime.switchWhenIdle' : 'agentRuntime.switchConversationHint')}
        </p>
        {(error || controller.error) && <InlineErrorNotice message={error || controller.error} />}
      </DropdownMenuSubContent>
    </DropdownMenuSub>
    <DropdownMenuItem disabled={configurationDisabled} className="shrink-0 cursor-pointer text-xs text-muted-foreground" onSelect={onConfigure}>
      {t('agentRuntime.configure')}
    </DropdownMenuItem>
  </DropdownMenuGroup>
}
