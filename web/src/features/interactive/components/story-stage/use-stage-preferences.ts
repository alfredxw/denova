import { useCallback, useEffect, useState } from 'react'
import type { StageCharacterLayout, StageTheme } from '@/features/settings/types'
import { fetchSettings, fetchProjectSettings } from '@/features/settings/api'
import { GLOBAL_SETTINGS_TARGET, projectSettingsTarget, subscribeSettingsTarget } from '@/features/settings/query'

const DEFAULT_STAGE_LINE_HEIGHT = 1.78
const DEFAULT_STAGE_PREFERENCES = {
  theme: undefined as StageTheme | undefined,
  lineHeight: DEFAULT_STAGE_LINE_HEIGHT,
  scrimOpacity: 0.75,
  textOutline: false,
  textMaxWidth: 896,
  characterLayout: 'center' as StageCharacterLayout,
  characterSize: 0.7,
}

export function useStagePreferences(projectId: string) {
  const [preferences, setPreferences] = useState(DEFAULT_STAGE_PREFERENCES)
  const normalizedProjectId = projectId.trim()

  const applySettings = useCallback((settings: Awaited<ReturnType<typeof fetchProjectSettings>>) => {
    const effective = settings.effective || {}
    setPreferences({
      theme: effective.interactive_stage_theme ?? undefined,
      lineHeight: clampNumber(effective.interactive_stage_line_height, 1.35, 2.4, DEFAULT_STAGE_LINE_HEIGHT),
      scrimOpacity: clampNumber(effective.interactive_stage_scrim_opacity, 0, 1, 0.75),
      textOutline: effective.interactive_stage_text_outline ?? DEFAULT_STAGE_PREFERENCES.textOutline,
      characterLayout: effective.interactive_stage_character_layout || 'center',
      characterSize: clampNumber(effective.interactive_stage_character_size, 0.4, 1, 0.7),
      textMaxWidth: clampNumber(effective.interactive_stage_text_max_width, 480, 1600, 896),
    })
  }, [])

  const load = useCallback(async () => {
    try {
      applySettings(await (normalizedProjectId ? fetchProjectSettings(normalizedProjectId) : fetchSettings()))
    } catch (error) {
      console.warn('[use-stage-preferences.ts] failed to load story stage display settings', error)
      setPreferences(DEFAULT_STAGE_PREFERENCES)
    }
  }, [applySettings, normalizedProjectId])

  useEffect(() => {
    void load()
    return subscribeSettingsTarget(normalizedProjectId ? projectSettingsTarget(normalizedProjectId) : GLOBAL_SETTINGS_TARGET, applySettings)
  }, [applySettings, load, normalizedProjectId])

  return preferences
}

function clampNumber(value: unknown, min: number, max: number, fallback: number) {
  if (value == null) return fallback
  const numberValue = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(numberValue)) return fallback
  return Math.min(max, Math.max(min, numberValue))
}
