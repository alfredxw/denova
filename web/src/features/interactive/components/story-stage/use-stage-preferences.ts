import { useCallback, useEffect, useState } from 'react'
import { fetchSettings } from '@/features/settings/api'

const DEFAULT_STAGE_LINE_HEIGHT = 1.78

export function useStagePreferences() {
  const [preferences, setPreferences] = useState({ lineHeight: DEFAULT_STAGE_LINE_HEIGHT })

  const load = useCallback(async () => {
    try {
      const settings = await fetchSettings()
      const effective = settings.effective || {}
      setPreferences({
        lineHeight: clampNumber(effective.interactive_stage_line_height, 1.35, 2.4, DEFAULT_STAGE_LINE_HEIGHT),
      })
    } catch (error) {
      console.warn('[interactive-stage] 加载故事舞台显示设置失败', error)
      setPreferences({ lineHeight: DEFAULT_STAGE_LINE_HEIGHT })
    }
  }, [])

  useEffect(() => {
    void load()
    window.addEventListener('nova:settings-updated', load)
    return () => window.removeEventListener('nova:settings-updated', load)
  }, [load])

  return preferences
}

function clampNumber(value: unknown, min: number, max: number, fallback: number) {
  const numberValue = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(numberValue)) return fallback
  return Math.min(max, Math.max(min, numberValue))
}
