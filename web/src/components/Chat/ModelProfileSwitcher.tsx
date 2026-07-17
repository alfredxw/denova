import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Check, ChevronDown, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { fetchSettings, updateUserSettings } from '@/features/settings/api'
import type { AgentModelOverride, LayeredSettings, ModelProfileSettings, Settings } from '@/features/settings/types'
import { modelProfileID, modelProfileLabel, modelProfilesWithDefault } from '@/features/settings/model-profiles'
import type { VisibleAgentKey } from '@/features/agents/agent-registry'

interface ModelProfileSwitcherProps {
  agentKey?: VisibleAgentKey
  workspace?: string
  disabled?: boolean
}

interface ModelProfileOption {
  id: string
  label: string
  modelLabel: string
}

type ReasoningEffort = '' | 'low' | 'medium' | 'high'

interface SavingSelection {
  kind: 'profile' | 'effort'
  value: string
}

const REASONING_EFFORTS: readonly ReasoningEffort[] = ['', 'low', 'medium', 'high']

export function ModelProfileSwitcher({ agentKey, workspace, disabled = false }: ModelProfileSwitcherProps) {
  const selector = useModelProfileSelector({ agentKey, workspace, disabled })

  if (!selector.enabled) return null

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          disabled={disabled || !selector.settings}
          className="group flex h-8 max-w-44 shrink-0 items-center gap-1.5 rounded-md border-0 bg-transparent px-1.5 text-xs leading-none text-[var(--nova-text)] outline-none transition-colors hover:text-[var(--nova-text)] focus-visible:bg-[var(--nova-hover)] disabled:pointer-events-none disabled:opacity-50"
          aria-label={selector.t('chat.modelProfile.switch', { model: selector.currentSelectionLabel })}
          title={selector.t('chat.modelProfile.switch', { model: selector.currentSelectionLabel })}
          data-model-profile-trigger="true"
          data-current-model={selector.currentModelLabel}
          data-current-reasoning-effort={selector.currentReasoningEffort}
        >
          <span className="min-w-0 truncate">{selector.settings ? selector.currentModelLabel : selector.t('chat.modelProfile.loading')}</span>
          {selector.currentReasoningEffortLabel ? (
            <span className="shrink-0 font-normal text-[var(--nova-text-faint)]">{selector.currentReasoningEffortLabel}</span>
          ) : null}
          <ChevronDown className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)] transition-transform group-data-[state=open]:rotate-180" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        side="top"
        aria-label={selector.t('chat.modelProfile.action')}
        className="w-60 border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-1.5 text-[var(--nova-text)]"
      >
        <ModelProfileOptions selector={selector} />
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

interface ModelProfileSelectorInput extends ModelProfileSwitcherProps {}

interface ModelProfileSelector {
  t: (key: string, options?: Record<string, unknown>) => string
  enabled: boolean
  settings: LayeredSettings | null
  options: ModelProfileOption[]
  currentProfile: string
  currentModelLabel: string
  currentReasoningEffort: ReasoningEffort
  currentReasoningEffortLabel: string
  currentSelectionLabel: string
  savingSelection: SavingSelection | null
  error: string | null
  selectProfile: (profileID: string) => Promise<void>
  selectReasoningEffort: (effort: ReasoningEffort) => Promise<void>
}

function useModelProfileSelector({ agentKey, workspace, disabled = false }: ModelProfileSelectorInput): ModelProfileSelector {
  const { t } = useTranslation()
  const [settings, setSettings] = useState<LayeredSettings | null>(null)
  const [savingSelection, setSavingSelection] = useState<SavingSelection | null>(null)
  const [error, setError] = useState<string | null>(null)
  const savingRef = useRef(false)
  const enabled = Boolean(agentKey && workspace)

  const load = useCallback(() => {
    if (!enabled) {
      setSettings(null)
      return
    }
    fetchSettings()
      .then((next) => {
        setSettings(next)
        setError(null)
      })
      .catch((err) => {
        setError(err instanceof Error ? err.message : t('chat.modelProfile.loadFailed'))
      })
  }, [enabled, t])

  useEffect(() => {
    load()
  }, [load])

  useEffect(() => {
    if (!enabled) return
    const onSettingsUpdated = () => load()
    window.addEventListener('nova:settings-updated', onSettingsUpdated)
    return () => window.removeEventListener('nova:settings-updated', onSettingsUpdated)
  }, [enabled, load])

  const options = useMemo(
    () => buildModelProfileOptions(settings, t),
    [settings, t],
  )
  const currentProfile = useMemo(
    () => agentKey ? resolveCurrentProfileID(settings?.effective ?? {}, agentKey, options) : 'default',
    [agentKey, options, settings?.effective],
  )
  const currentModelLabel = options.find((option) => option.id === currentProfile)?.modelLabel || currentProfile
  const currentReasoningEffort = useMemo(
    () => agentKey ? resolveCurrentReasoningEffort(settings?.effective ?? {}, agentKey) : '',
    [agentKey, settings?.effective],
  )
  const currentReasoningEffortLabel = currentReasoningEffort
    ? t(`chat.modelProfile.reasoning.${currentReasoningEffort}`)
    : ''
  const currentSelectionLabel = [currentModelLabel, currentReasoningEffortLabel].filter(Boolean).join(' ')

  const saveAgentModelSelection = async (
    selection: SavingSelection,
    update: (latest: Settings) => Settings,
  ) => {
    if (!agentKey || disabled || savingRef.current) return
    const previousSettings = settings
    savingRef.current = true
    setSavingSelection(selection)
    setError(null)
    try {
      const latest = await fetchSettings()
      const saved = await updateUserSettings(update(latest.user), latest.revisions?.user)
      setSettings(saved)
      window.dispatchEvent(new CustomEvent('nova:settings-updated'))
    } catch (err) {
      setSettings(previousSettings)
      const message = err instanceof Error ? err.message : t('chat.modelProfile.saveFailed')
      console.warn('[model-profile-switcher] save failed', err)
      setError(message)
    } finally {
      savingRef.current = false
      setSavingSelection(null)
    }
  }

  const selectProfile = async (profileID: string) => {
    if (!agentKey || profileID === currentProfile) return
    await saveAgentModelSelection(
      { kind: 'profile', value: profileID },
      (latest) => withAgentModelSelection(latest, agentKey, { profileID }),
    )
  }

  const selectReasoningEffort = async (effort: ReasoningEffort) => {
    if (!agentKey || effort === currentReasoningEffort) return
    await saveAgentModelSelection(
      { kind: 'effort', value: effort },
      (latest) => withAgentModelSelection(latest, agentKey, { reasoningEffort: effort }),
    )
  }

  return {
    t,
    enabled,
    settings,
    options,
    currentProfile,
    currentModelLabel,
    currentReasoningEffort,
    currentReasoningEffortLabel,
    currentSelectionLabel,
    savingSelection,
    error,
    selectProfile,
    selectReasoningEffort,
  }
}

function ModelProfileOptions({ selector }: { selector: ModelProfileSelector }) {
  const {
    t,
    options,
    currentProfile,
    currentReasoningEffort,
    savingSelection,
    error,
    selectProfile,
    selectReasoningEffort,
  } = selector
  return (
    <>
      <div className="px-1.5 pb-1 pt-0.5 text-[10px] font-medium text-[var(--nova-text-faint)]">
        {t('chat.modelProfile.modelSection')}
      </div>
      {options.map((option) => (
        <DropdownMenuItem
          key={option.id}
          disabled={Boolean(savingSelection)}
          onSelect={() => void selectProfile(option.id)}
          className="cursor-pointer py-1.5 text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
        >
          {savingSelection?.kind === 'profile' && savingSelection.value === option.id
            ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
            : <Check className={`h-3.5 w-3.5 ${option.id === currentProfile ? 'opacity-100' : 'opacity-0'}`} />}
          <span className="min-w-0 flex-1 truncate">{option.label}</span>
        </DropdownMenuItem>
      ))}
      {options.length === 0 ? (
        <DropdownMenuItem disabled className="text-xs">
          {t('chat.modelProfile.empty')}
        </DropdownMenuItem>
      ) : null}
      <DropdownMenuSeparator className="bg-[var(--nova-border-soft)]" />
      <div className="px-1.5 pb-1 pt-0.5 text-[10px] font-medium text-[var(--nova-text-faint)]">
        {t('chat.modelProfile.reasoningSection')}
      </div>
      {REASONING_EFFORTS.map((effort) => {
        const label = effort
          ? t(`chat.modelProfile.reasoning.${effort}`)
          : t('chat.modelProfile.reasoning.inherit')
        return (
          <DropdownMenuItem
            key={effort || 'inherit'}
            disabled={Boolean(savingSelection)}
            onSelect={() => void selectReasoningEffort(effort)}
            className="cursor-pointer py-1.5 text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
          >
            {savingSelection?.kind === 'effort' && savingSelection.value === effort
              ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
              : <Check className={`h-3.5 w-3.5 ${effort === currentReasoningEffort ? 'opacity-100' : 'opacity-0'}`} />}
            <span className="min-w-0 flex-1 truncate">{label}</span>
          </DropdownMenuItem>
        )
      })}
      {error ? (
        <>
          <DropdownMenuSeparator className="bg-[var(--nova-border-soft)]" />
          <DropdownMenuItem disabled className="text-xs text-red-400">
            {error}
          </DropdownMenuItem>
        </>
      ) : null}
    </>
  )
}

export function buildModelProfileOptions(settings: LayeredSettings | null, t: (key: string, options?: Record<string, unknown>) => string): ModelProfileOption[] {
  if (!settings) return []
  const profiles = new Map<string, string>()
  const add = (profile?: ModelProfileSettings) => {
    const id = modelProfileID(profile)
    if (!id) return
    profiles.set(id, modelProfileLabel(profile))
  }
  modelProfilesWithDefault(settings.effective).forEach(add)
  if (!profiles.has('default')) profiles.set('default', t('chat.modelProfile.defaultModel'))
  return Array.from(profiles.entries()).map(([id, label]) => ({
    id,
    modelLabel: label,
    label: id === 'default'
      ? t('chat.modelProfile.defaultProfile', { label })
      : t('chat.modelProfile.profile', { id, label }),
  }))
}

export function resolveCurrentProfileID(settings: Settings, agentKey: VisibleAgentKey, options: ModelProfileOption[]): string {
  const merged = resolveAgentModelOverride(settings, agentKey)
  const profileID = merged.profile_id || 'default'
  return options.some((option) => option.id === profileID) ? profileID : 'default'
}

function resolveCurrentReasoningEffort(settings: Settings, agentKey: VisibleAgentKey): ReasoningEffort {
  const value = resolveAgentModelOverride(settings, agentKey).reasoning_effort?.trim().toLowerCase() ?? ''
  return REASONING_EFFORTS.includes(value as ReasoningEffort) ? value as ReasoningEffort : ''
}

function resolveAgentModelOverride(settings: Settings, agentKey: VisibleAgentKey): AgentModelOverride {
  return mergeAgentModelOverride(settings.agent_models?.default ?? {}, settings.agent_models?.[agentKey] ?? {})
}

function mergeAgentModelOverride(parent: AgentModelOverride, child: AgentModelOverride): AgentModelOverride {
  return {
    profile_id: child.profile_id || parent.profile_id,
    temperature: child.temperature ?? parent.temperature,
    enable_thinking: child.enable_thinking ?? parent.enable_thinking,
    reasoning_effort: child.reasoning_effort || parent.reasoning_effort,
  }
}

function withAgentModelSelection(
  settings: Settings,
  agentKey: VisibleAgentKey,
  selection: { profileID?: string; reasoningEffort?: ReasoningEffort },
): Settings {
  const nextModel = { ...(settings.agent_models?.[agentKey] ?? {}) }
  if (selection.profileID !== undefined) nextModel.profile_id = selection.profileID
  if (selection.reasoningEffort !== undefined) {
    if (selection.reasoningEffort) nextModel.reasoning_effort = selection.reasoningEffort
    else delete nextModel.reasoning_effort
  }
  return {
    ...settings,
    agent_models: {
      ...(settings.agent_models ?? {}),
      [agentKey]: nextModel,
    },
  }
}
