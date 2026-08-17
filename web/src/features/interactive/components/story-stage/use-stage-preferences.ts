import { useCallback, useEffect, useState } from 'react'
import { fetchProjectSettings, refreshProjectSettings } from '@/features/settings/api'

const DEFAULT_STAGE_LINE_HEIGHT = 1.78

export function useStagePreferences(projectId: string) {
  const [preferences, setPreferences] = useState({ lineHeight: DEFAULT_STAGE_LINE_HEIGHT })

  const load = useCallback(async (fresh = false) => {
    try {
      const settings = await (fresh ? refreshProjectSettings(projectId) : fetchProjectSettings(projectId))
      const effective = settings.effective || {}
      setPreferences({
        lineHeight: clampNumber(effective.interactive_stage_line_height, 1.35, 2.4, DEFAULT_STAGE_LINE_HEIGHT),
      })
    } catch (error) {
      console.warn('[use-stage-preferences.ts] failed to load story stage display settings', error)
      setPreferences({ lineHeight: DEFAULT_STAGE_LINE_HEIGHT })
    }
  }, [projectId])

  useEffect(() => {
    void load()
    const onSettingsUpdated = (event: Event) => {
      const changedProjectId = (event as CustomEvent<{ projectId?: string }>).detail?.projectId
      if (!changedProjectId || changedProjectId === projectId) void load(true)
    }
    window.addEventListener('nova:settings-updated', onSettingsUpdated)
    return () => window.removeEventListener('nova:settings-updated', onSettingsUpdated)
  }, [load, projectId])

  return preferences
}

function clampNumber(value: unknown, min: number, max: number, fallback: number) {
  const numberValue = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(numberValue)) return fallback
  return Math.min(max, Math.max(min, numberValue))
}
