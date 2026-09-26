import { useCallback, useEffect, useState } from 'react'
import { fetchProjectSettings } from '@/features/settings/api'
import { projectSettingsTarget, subscribeSettingsTarget } from '@/features/settings/query'

const DEFAULT_STAGE_LINE_HEIGHT = 1.78
const DEFAULT_STAGE_PREFERENCES = { lineHeight: DEFAULT_STAGE_LINE_HEIGHT, scrimOpacity: 0.75 }

export function useStagePreferences(projectId: string) {
  const [preferences, setPreferences] = useState(DEFAULT_STAGE_PREFERENCES)
  const normalizedProjectId = projectId.trim()

  const applySettings = useCallback((settings: Awaited<ReturnType<typeof fetchProjectSettings>>) => {
    const effective = settings.effective || {}
    setPreferences({
      lineHeight: clampNumber(effective.interactive_stage_line_height, 1.35, 2.4, DEFAULT_STAGE_LINE_HEIGHT),
      scrimOpacity: clampNumber(effective.interactive_stage_scrim_opacity, 0, 1, 0.75),
    })
  }, [])

  const load = useCallback(async () => {
    try {
      applySettings(await fetchProjectSettings(normalizedProjectId))
    } catch (error) {
      console.warn('[use-stage-preferences.ts] failed to load story stage display settings', error)
      setPreferences(DEFAULT_STAGE_PREFERENCES)
    }
  }, [applySettings, normalizedProjectId])

  useEffect(() => {
    if (!normalizedProjectId) {
      setPreferences(DEFAULT_STAGE_PREFERENCES)
      return
    }
    void load()
    return subscribeSettingsTarget(projectSettingsTarget(normalizedProjectId), applySettings)
  }, [applySettings, load, normalizedProjectId])

  return preferences
}

function clampNumber(value: unknown, min: number, max: number, fallback: number) {
  if (value == null) return fallback
  const numberValue = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(numberValue)) return fallback
  return Math.min(max, Math.max(min, numberValue))
}
