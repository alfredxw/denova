import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { BrainCircuit, ImagePlus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { DropdownMenuItem } from '@/components/ui/dropdown-menu'
import {
  createSettingsMergePatch,
  fetchProjectSettings,
  fetchSettings,
  patchProjectSettings,
  patchSettings,
  refreshProjectSettings,
  refreshSettings,
} from '@/features/settings/api'
import {
  DEFAULT_IMAGE_API_PROFILE_ID,
  imageAPIProfileID,
  imageAPIProfileLabel,
  imageAPIProfilesWithDefault,
} from '@/features/settings/image-profiles'
import {
  DEFAULT_MODEL_PROFILE_ID,
  modelProfileID,
  modelProfileLabel,
  modelProfilesWithDefault,
} from '@/features/settings/model-profiles'
import type { LayeredSettings, Settings } from '@/features/settings/types'
import { saveWithRevisionRecovery } from '@/lib/revision-conflict'
import { PersistedSettingsMenuSub, type PersistedSettingsMenuOption } from './PersistedSettingsMenuSub'

interface ImageAgentModelSettingsMenuProps {
  disabled?: boolean
  projectId?: string
}

type ImageAgentModelSelection =
  | { kind: 'language'; value: string }
  | { kind: 'image'; value: string }

interface ModelMenuOption extends PersistedSettingsMenuOption {
  currentLabel: string
}

let nextImageAgentModelMenuSourceID = 1

/** Shared composer shortcuts for the Image Agent's reasoning and output models. */
export function ImageAgentModelSettingsMenu({ disabled = false, projectId = '' }: ImageAgentModelSettingsMenuProps) {
  const { t } = useTranslation()
  const selector = useImageAgentModelSettings(projectId)
  const languageOptions = useMemo(
    () => buildLanguageModelOptions(selector.settings, t),
    [selector.settings, t],
  )
  const imageOptions = useMemo(
    () => buildImageModelOptions(selector.settings, t),
    [selector.settings, t],
  )
  const currentLanguageModel = selector.saving?.kind === 'language'
    ? selector.saving.value
    : resolveImageAgentLanguageModelID(selector.settings)
  const currentImageModel = selector.saving?.kind === 'image'
    ? selector.saving.value
    : resolveImageAgentOutputModelID(selector.settings)
  const controlsDisabled = disabled || !selector.settings

  return (
    <>
      <PersistedSettingsMenuSub
        icon={BrainCircuit}
        label={t('chat.imageAgentModels.language')}
        currentLabel={optionCurrentLabel(languageOptions, currentLanguageModel)}
        value={currentLanguageModel}
        options={languageOptions}
        saving={selector.saving?.kind === 'language'}
        disabled={controlsDisabled}
        emptyLabel={t('chat.imageAgentModels.languageEmpty')}
        onValueChange={(value) => selector.select({ kind: 'language', value })}
      />
      <PersistedSettingsMenuSub
        icon={ImagePlus}
        label={t('chat.imageAgentModels.image')}
        currentLabel={optionCurrentLabel(imageOptions, currentImageModel)}
        value={currentImageModel}
        options={imageOptions}
        saving={selector.saving?.kind === 'image'}
        disabled={controlsDisabled}
        emptyLabel={t('chat.imageAgentModels.imageEmpty')}
        onValueChange={(value) => selector.select({ kind: 'image', value })}
      />
      {selector.error ? (
        <DropdownMenuItem disabled className="text-xs text-[var(--nova-danger)]">
          {t('chat.imageAgentModels.saveFailed', { error: selector.error })}
        </DropdownMenuItem>
      ) : null}
    </>
  )
}

function useImageAgentModelSettings(projectId: string) {
  const [settings, setSettings] = useState<LayeredSettings | null>(null)
  const [saving, setSaving] = useState<ImageAgentModelSelection | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [eventSource] = useState(() => {
    const source = `image-agent-model-menu-${nextImageAgentModelMenuSourceID}`
    nextImageAgentModelMenuSourceID += 1
    return source
  })
  const mountedRef = useRef(true)
  const loadSequenceRef = useRef(0)

  const load = useCallback(async (fresh = false) => {
    const sequence = loadSequenceRef.current + 1
    loadSequenceRef.current = sequence
    try {
      const next = projectId
        ? await (fresh ? refreshProjectSettings(projectId) : fetchProjectSettings(projectId))
        : await (fresh ? refreshSettings() : fetchSettings())
      if (!mountedRef.current || sequence !== loadSequenceRef.current) return null
      setSettings(next)
      setError(null)
      return next
    } catch (cause) {
      if (!mountedRef.current || sequence !== loadSequenceRef.current) return null
      const message = cause instanceof Error ? cause.message : String(cause)
      console.error('[ImageAgentModelSettingsMenu.tsx] failed to load Image Agent model settings', cause)
      setError(message)
      return null
    }
  }, [projectId])

  useEffect(() => {
    mountedRef.current = true
    void load()
    return () => {
      mountedRef.current = false
      loadSequenceRef.current += 1
    }
  }, [load])

  useEffect(() => {
    const onSettingsUpdated = (event: Event) => {
      const detail = (event as CustomEvent<{ source?: string; projectId?: string }>).detail
      if (detail?.source === eventSource) return
      if (detail?.projectId && detail.projectId !== projectId) return
      void load(true)
    }
    window.addEventListener('nova:settings-updated', onSettingsUpdated)
    return () => window.removeEventListener('nova:settings-updated', onSettingsUpdated)
  }, [eventSource, load])

  const select = useCallback(async (selection: ImageAgentModelSelection) => {
    if (!settings || saving || selectionMatchesSettings(settings, selection)) return

    setSaving(selection)
    setError(null)
    const layer = selectionSettingsLayer(settings, selection, projectId)
    const baseline = layer === 'workspace' ? settings.workspace : settings.user
    let patchBaseline = baseline
    try {
      const next = await saveWithRevisionRecovery({
        baseline,
        draft: applySelection(baseline, selection),
        revision: settings.revisions?.[layer],
        save: (draft, revision) => projectId
          ? patchProjectSettings(projectId, layer, createSettingsMergePatch(patchBaseline, draft), revision)
          : patchSettings(layer, createSettingsMergePatch(patchBaseline, draft), revision),
        loadLatest: async () => {
          const latest = projectId ? await refreshProjectSettings(projectId) : await refreshSettings()
          return {
            value: layer === 'workspace' ? latest.workspace : latest.user,
            revision: latest.revisions?.[layer],
          }
        },
        rebase: (_previousBaseline, _previousDraft, latest) => {
          patchBaseline = latest
          return applySelection(latest, selection)
        },
      })
      if (mountedRef.current) {
        loadSequenceRef.current += 1
        setSettings(next)
      }
      window.dispatchEvent(new CustomEvent('nova:settings-updated', {
        detail: {
          source: eventSource,
          projectId: layer === 'workspace' ? projectId : undefined,
        },
      }))
    } catch (cause) {
      if (!mountedRef.current) return
      const message = cause instanceof Error ? cause.message : String(cause)
      console.error('[ImageAgentModelSettingsMenu.tsx] failed to save Image Agent model selection', {
        kind: selection.kind,
        value: selection.value,
        error: cause,
      })
      setError(message)
    } finally {
      if (mountedRef.current) setSaving(null)
    }
  }, [eventSource, projectId, saving, settings])

  return { settings, saving, error, select }
}

function applySelection(settings: Settings, selection: ImageAgentModelSelection): Settings {
  if (selection.kind === 'image') {
    return { ...settings, default_image_api_profile_id: selection.value }
  }
  return {
    ...settings,
    agent_models: {
      ...settings.agent_models,
      image: {
        ...settings.agent_models?.image,
        profile_id: selection.value,
      },
    },
  }
}

function selectionMatchesSettings(settings: LayeredSettings, selection: ImageAgentModelSelection): boolean {
  return selection.kind === 'language'
    ? resolveImageAgentLanguageModelID(settings) === selection.value
    : resolveImageAgentOutputModelID(settings) === selection.value
}

function selectionSettingsLayer(
  settings: LayeredSettings,
  selection: ImageAgentModelSelection,
  projectId: string,
) {
  if (!projectId || selection.kind === 'language') return 'user' as const
  const workspaceHasDefault = Boolean(settings.workspace.default_image_api_profile_id?.trim())
  const workspaceDefinesProfile = settings.workspace.image_api_profiles?.some(
    (profile) => imageAPIProfileID(profile) === selection.value,
  )
  return workspaceHasDefault || workspaceDefinesProfile ? 'workspace' as const : 'user' as const
}

function resolveImageAgentLanguageModelID(settings: LayeredSettings | null): string {
  return settings?.effective.agent_models?.image?.profile_id?.trim()
    || settings?.effective.agent_models?.default?.profile_id?.trim()
    || DEFAULT_MODEL_PROFILE_ID
}

function resolveImageAgentOutputModelID(settings: LayeredSettings | null): string {
  return settings?.effective.default_image_api_profile_id?.trim() || DEFAULT_IMAGE_API_PROFILE_ID
}

function buildLanguageModelOptions(
  settings: LayeredSettings | null,
  t: (key: string, options?: Record<string, unknown>) => string,
): ModelMenuOption[] {
  if (!settings) return []
  const options = modelProfilesWithDefault(settings.effective).map((profile) => {
    const id = modelProfileID(profile)
    const label = modelProfileLabel(profile) || id
    return {
      id,
      currentLabel: label,
      label: id === DEFAULT_MODEL_PROFILE_ID
        ? t('chat.modelProfile.defaultProfile', { label })
        : t('chat.modelProfile.profile', { id, label }),
      meta: profile.model?.trim(),
    }
  }).filter((option) => Boolean(option.id))
  return includeCurrentOption(options, resolveImageAgentLanguageModelID(settings))
}

function buildImageModelOptions(
  settings: LayeredSettings | null,
  t: (key: string, options?: Record<string, unknown>) => string,
): ModelMenuOption[] {
  if (!settings) return []
  const options = imageAPIProfilesWithDefault(settings.effective).map((profile) => {
    const id = imageAPIProfileID(profile)
    const label = imageAPIProfileLabel(profile) || id
    return {
      id,
      currentLabel: label,
      label: id === DEFAULT_IMAGE_API_PROFILE_ID
        ? t('chat.modelProfile.defaultProfile', { label })
        : t('chat.modelProfile.profile', { id, label }),
      meta: profile.openai_model?.trim(),
    }
  }).filter((option) => Boolean(option.id))
  return includeCurrentOption(options, resolveImageAgentOutputModelID(settings))
}

function includeCurrentOption(options: ModelMenuOption[], currentID: string): ModelMenuOption[] {
  if (!currentID || options.some((option) => option.id === currentID)) return options
  return [{ id: currentID, label: currentID, currentLabel: currentID }, ...options]
}

function optionCurrentLabel(options: ModelMenuOption[], currentID: string): string {
  return options.find((option) => option.id === currentID)?.currentLabel || currentID
}
