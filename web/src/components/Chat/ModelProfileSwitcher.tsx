import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Check, Cpu, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
} from '@/components/ui/dropdown-menu'
import { fetchSettings, updateWorkspaceSettings } from '@/features/settings/api'
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
}

export function ModelProfileSwitcher({ agentKey, workspace, disabled = false }: ModelProfileSwitcherProps) {
  const selector = useModelProfileSelector({ agentKey, workspace, disabled })

  if (!selector.enabled) return null

  return (
    <>
      <DropdownMenuSub>
        <DropdownMenuSubTrigger
          disabled={disabled || !selector.settings}
          className="cursor-pointer text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
        >
          <Cpu className="h-3.5 w-3.5" />
          <span className="min-w-0 flex-1">{selector.t('chat.modelProfile.action')}</span>
          <span className="max-w-36 truncate text-[10px] text-[var(--nova-text-faint)]">{selector.currentLabel}</span>
        </DropdownMenuSubTrigger>
        <DropdownMenuSubContent className="w-72 border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 text-[var(--nova-text)]">
          <ModelProfileOptions selector={selector} />
        </DropdownMenuSubContent>
      </DropdownMenuSub>
      <DropdownMenuSeparator className="bg-[var(--nova-border-soft)]" />
    </>
  )
}

interface ModelProfileSelectorInput extends ModelProfileSwitcherProps {}

interface ModelProfileSelector {
  t: (key: string, options?: Record<string, unknown>) => string
  enabled: boolean
  settings: LayeredSettings | null
  options: ModelProfileOption[]
  currentProfile: string
  currentLabel: string
  savingProfile: string | null
  error: string | null
  selectProfile: (profileID: string) => Promise<void>
}

function useModelProfileSelector({ agentKey, workspace, disabled = false }: ModelProfileSelectorInput): ModelProfileSelector {
  const { t } = useTranslation()
  const [settings, setSettings] = useState<LayeredSettings | null>(null)
  const [savingProfile, setSavingProfile] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const selectingRef = useRef<string | null>(null)
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
  const currentLabel = options.find((option) => option.id === currentProfile)?.label || currentProfile

  const selectProfile = async (profileID: string) => {
    if (!agentKey || disabled || savingProfile || selectingRef.current || profileID === currentProfile) return
    const previousSettings = settings
    selectingRef.current = profileID
    setSavingProfile(profileID)
    setError(null)
    try {
      const latest = await fetchSettings()
      const nextWorkspace = withAgentModelProfile(latest.workspace, agentKey, profileID)
      const saved = await updateWorkspaceSettings(nextWorkspace)
      setSettings(saved)
      window.dispatchEvent(new CustomEvent('nova:settings-updated'))
    } catch (err) {
      setSettings(previousSettings)
      const message = err instanceof Error ? err.message : t('chat.modelProfile.saveFailed')
      console.warn('[model-profile-switcher] save failed', err)
      setError(message)
    } finally {
      selectingRef.current = null
      setSavingProfile(null)
    }
  }

  return {
    t,
    enabled,
    settings,
    options,
    currentProfile,
    currentLabel,
    savingProfile,
    error,
    selectProfile,
  }
}

function ModelProfileOptions({ selector }: { selector: ModelProfileSelector }) {
  const { t, options, currentProfile, savingProfile, error, selectProfile } = selector
  return (
    <>
      {options.map((option) => (
        <DropdownMenuItem
          key={option.id}
          disabled={Boolean(savingProfile)}
          onSelect={(event) => {
            event.preventDefault()
            void selectProfile(option.id)
          }}
          onClick={() => void selectProfile(option.id)}
          className="cursor-pointer text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
        >
          {savingProfile === option.id ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className={`h-3.5 w-3.5 ${option.id === currentProfile ? 'opacity-100' : 'opacity-0'}`} />}
          <span className="min-w-0 flex-1 truncate">{option.label}</span>
        </DropdownMenuItem>
      ))}
      {options.length === 0 ? (
        <DropdownMenuItem disabled className="text-xs">
          {t('chat.modelProfile.empty')}
        </DropdownMenuItem>
      ) : null}
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
    label: id === 'default'
      ? t('chat.modelProfile.defaultProfile', { label })
      : t('chat.modelProfile.profile', { id, label }),
  }))
}

export function resolveCurrentProfileID(settings: Settings, agentKey: VisibleAgentKey, options: ModelProfileOption[]): string {
  const merged = mergeAgentModelOverride(settings.agent_models?.default ?? {}, settings.agent_models?.[agentKey] ?? {})
  const profileID = merged.profile_id || 'default'
  return options.some((option) => option.id === profileID) ? profileID : 'default'
}

function mergeAgentModelOverride(parent: AgentModelOverride, child: AgentModelOverride): AgentModelOverride {
  return {
    profile_id: child.profile_id || parent.profile_id,
    temperature: child.temperature ?? parent.temperature,
    enable_thinking: child.enable_thinking ?? parent.enable_thinking,
    reasoning_effort: child.reasoning_effort || parent.reasoning_effort,
  }
}

function withAgentModelProfile(settings: Settings, agentKey: VisibleAgentKey, profileID: string): Settings {
  return {
    ...settings,
    agent_models: {
      ...(settings.agent_models ?? {}),
      [agentKey]: {
        ...(settings.agent_models?.[agentKey] ?? {}),
        profile_id: profileID,
      },
    },
  }
}
