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
import { normalizeThinkingLevel, THINKING_LEVEL_SELECTIONS } from '@/features/settings/thinking-levels'
import type { ThinkingLevelSelection } from '@/features/settings/thinking-levels'
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

interface SavingSelection {
  kind: 'profile' | 'thinking'
  value: string
}

export function ModelProfileSwitcher({ agentKey, workspace, disabled = false }: ModelProfileSwitcherProps) {
  const selector = useModelProfileSelector({ agentKey, workspace, disabled })
  const [open, setOpen] = useState(false)

  if (!selector.enabled) return null

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          disabled={disabled || !selector.settings}
          className="group flex h-8 max-w-44 shrink-0 items-center gap-1.5 rounded-md border-0 bg-transparent px-1.5 text-xs leading-none text-[var(--nova-text)] outline-none transition-colors hover:text-[var(--nova-text)] focus-visible:bg-[var(--nova-hover)] disabled:pointer-events-none disabled:opacity-50"
          aria-label={selector.t('chat.modelProfile.switch', { model: selector.currentSelectionLabel })}
          title={selector.t('chat.modelProfile.switch', { model: selector.currentSelectionLabel })}
          data-model-profile-trigger="true"
          data-current-model={selector.currentModelLabel}
          data-current-thinking-level={selector.currentThinkingLevel}
        >
          <span className="min-w-0 truncate">{selector.settings ? selector.currentModelLabel : selector.t('chat.modelProfile.loading')}</span>
          {selector.currentThinkingLevelLabel ? (
            <span className="shrink-0 font-normal text-[var(--nova-text-faint)]">{selector.currentThinkingLevelLabel}</span>
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
        <ModelProfileOptions
          selector={selector}
          onThinkingLevelSelect={(level) => {
            setOpen(false)
            void selector.selectThinkingLevel(level)
          }}
        />
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
  currentThinkingLevel: ThinkingLevelSelection
  currentThinkingLevelLabel: string
  currentSelectionLabel: string
  savingSelection: SavingSelection | null
  error: string | null
  selectProfile: (profileID: string) => Promise<void>
  selectThinkingLevel: (level: ThinkingLevelSelection) => Promise<void>
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
  const currentThinkingLevel = useMemo(
    () => agentKey ? resolveCurrentThinkingLevel(settings?.effective ?? {}, agentKey) : '',
    [agentKey, settings?.effective],
  )
  const currentThinkingLevelLabel = currentThinkingLevel
    ? t(`chat.modelProfile.thinking.${currentThinkingLevel}`)
    : ''
  const currentSelectionLabel = [currentModelLabel, currentThinkingLevelLabel].filter(Boolean).join(' ')

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

  const selectThinkingLevel = async (level: ThinkingLevelSelection) => {
    if (!agentKey || level === currentThinkingLevel) return
    await saveAgentModelSelection(
      { kind: 'thinking', value: level },
      (latest) => withAgentModelSelection(latest, agentKey, { thinkingLevel: level }),
    )
  }

  return {
    t,
    enabled,
    settings,
    options,
    currentProfile,
    currentModelLabel,
    currentThinkingLevel,
    currentThinkingLevelLabel,
    currentSelectionLabel,
    savingSelection,
    error,
    selectProfile,
    selectThinkingLevel,
  }
}

function ModelProfileOptions({
  selector,
  onThinkingLevelSelect,
}: {
  selector: ModelProfileSelector
  onThinkingLevelSelect: (level: ThinkingLevelSelection) => void
}) {
  const {
    t,
    options,
    currentProfile,
    currentThinkingLevel,
    savingSelection,
    error,
    selectProfile,
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
        {t('chat.modelProfile.thinkingSection')}
      </div>
      <div
        role="group"
        aria-label={t('chat.modelProfile.thinkingSection')}
        className="grid grid-cols-4 gap-1 px-1 pb-1"
      >
        {THINKING_LEVEL_SELECTIONS.map((level) => {
          const selected = level === currentThinkingLevel
          const label = level
            ? t(`chat.modelProfile.thinking.${level}`)
            : t('chat.modelProfile.thinking.inherit')
          return (
            <button
              key={level || 'inherit'}
              type="button"
              disabled={Boolean(savingSelection)}
              aria-pressed={selected}
              onClick={() => onThinkingLevelSelect(level)}
              className={`flex h-7 min-w-0 items-center justify-center rounded-md border px-1 text-[11px] transition-colors disabled:opacity-50 ${
                selected
                  ? 'border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)]'
                  : 'border-transparent text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
              }`}
            >
              {savingSelection?.kind === 'thinking' && savingSelection.value === level
                ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
                : <span className="truncate">{label}</span>}
            </button>
          )
        })}
      </div>
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

function resolveCurrentThinkingLevel(settings: Settings, agentKey: VisibleAgentKey): ThinkingLevelSelection {
  return normalizeThinkingLevel(resolveAgentModelOverride(settings, agentKey).thinking_level) ?? ''
}

function resolveAgentModelOverride(settings: Settings, agentKey: VisibleAgentKey): AgentModelOverride {
  return mergeAgentModelOverride(settings.agent_models?.default ?? {}, settings.agent_models?.[agentKey] ?? {})
}

function mergeAgentModelOverride(parent: AgentModelOverride, child: AgentModelOverride): AgentModelOverride {
  return {
    profile_id: child.profile_id || parent.profile_id,
    temperature: child.temperature ?? parent.temperature,
    thinking_level: child.thinking_level || parent.thinking_level,
  }
}

function withAgentModelSelection(
  settings: Settings,
  agentKey: VisibleAgentKey,
  selection: { profileID?: string; thinkingLevel?: ThinkingLevelSelection },
): Settings {
  const nextModel = { ...(settings.agent_models?.[agentKey] ?? {}) }
  if (selection.profileID !== undefined) nextModel.profile_id = selection.profileID
  if (selection.thinkingLevel !== undefined) {
    if (selection.thinkingLevel) nextModel.thinking_level = selection.thinkingLevel
    else delete nextModel.thinking_level
  }
  return {
    ...settings,
    agent_models: {
      ...(settings.agent_models ?? {}),
      [agentKey]: nextModel,
    },
  }
}
