import { useCallback, useEffect, useState } from 'react'
import { fetchSettings, refreshSettings } from '@/features/settings/api'

const DEFAULT_STAGE_LINE_HEIGHT = 1.78

export function useStagePreferences() {
  const [preferences, setPreferences] = useState({ lineHeight: DEFAULT_STAGE_LINE_HEIGHT })

  const load = useCallback(async (fresh = false) => {
    try {
      const settings = await (fresh ? refreshSettings() : fetchSettings())
      const effective = settings.effective || {}
      setPreferences({
        lineHeight: clampNumber(effective.interactive_stage_line_height, 1.35, 2.4, DEFAULT_STAGE_LINE_HEIGHT),
      })
    } catch (error) {
      console.warn('[use-stage-preferences.ts] failed to load story stage display settings', error)
      setPreferences({ lineHeight: DEFAULT_STAGE_LINE_HEIGHT })
    }
  }, [])

  useEffect(() => {
    void load()
    const onSettingsUpdated = () => { void load(true) }
    window.addEventListener('nova:settings-updated', onSettingsUpdated)
    return () => window.removeEventListener('nova:settings-updated', onSettingsUpdated)
  }, [load])

  return preferences
}

function clampNumber(value: unknown, min: number, max: number, fallback: number) {
  const numberValue = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(numberValue)) return fallback
  return Math.min(max, Math.max(min, numberValue))
}
