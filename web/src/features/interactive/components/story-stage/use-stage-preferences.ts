import { useCallback, useEffect, useState } from 'react'
import { fetchProjectSettings } from '@/features/settings/api'
import { projectSettingsTarget, subscribeSettingsTarget } from '@/features/settings/query'

const DEFAULT_STAGE_LINE_HEIGHT = 1.78

export function useStagePreferences(projectId: string) {
  const [preferences, setPreferences] = useState({ lineHeight: DEFAULT_STAGE_LINE_HEIGHT })
  const normalizedProjectId = projectId.trim()

  const applySettings = useCallback((settings: Awaited<ReturnType<typeof fetchProjectSettings>>) => {
    const effective = settings.effective || {}
    setPreferences({
      lineHeight: clampNumber(effective.interactive_stage_line_height, 1.35, 2.4, DEFAULT_STAGE_LINE_HEIGHT),
    })
  }, [])

  const load = useCallback(async () => {
    try {
      applySettings(await fetchProjectSettings(normalizedProjectId))
    } catch (error) {
      console.warn('[use-stage-preferences.ts] failed to load story stage display settings', error)
      setPreferences({ lineHeight: DEFAULT_STAGE_LINE_HEIGHT })
    }
  }, [applySettings, normalizedProjectId])

  useEffect(() => {
    if (!normalizedProjectId) {
      setPreferences({ lineHeight: DEFAULT_STAGE_LINE_HEIGHT })
      return
    }
    void load()
    return subscribeSettingsTarget(projectSettingsTarget(normalizedProjectId), applySettings)
  }, [applySettings, load, normalizedProjectId])

  return preferences
}

function clampNumber(value: unknown, min: number, max: number, fallback: number) {
  const numberValue = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(numberValue)) return fallback
  return Math.min(max, Math.max(min, numberValue))
}
