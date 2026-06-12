import { createContext, useContext, useEffect, useMemo } from 'react'
import type { ReactNode } from 'react'
import { MotionConfig, useReducedMotion } from 'motion/react'

export type MotionIntensity = 'system' | 'full' | 'reduced' | 'off'

interface NovaMotionSettings {
  intensity: MotionIntensity
  disabled: boolean
  reduced: boolean
}

const NovaMotionContext = createContext<NovaMotionSettings>({
  intensity: 'system',
  disabled: false,
  reduced: false,
})

export function normalizeMotionIntensity(value?: string | null): MotionIntensity {
  if (value === 'full' || value === 'reduced' || value === 'off' || value === 'system') return value
  return 'system'
}

export function NovaMotionProvider({
  intensity,
  children,
}: {
  intensity?: string | null
  children: ReactNode
}) {
  const normalized = normalizeMotionIntensity(intensity)
  const systemReduced = useReducedMotion()
  const disabled = normalized === 'off'
  const reduced = disabled || normalized === 'reduced' || (normalized === 'system' && Boolean(systemReduced))
  const reducedMotion = normalized === 'full' ? 'never' : (reduced ? 'always' : 'user')
  const settings = useMemo<NovaMotionSettings>(() => ({
    intensity: normalized,
    disabled,
    reduced,
  }), [disabled, normalized, reduced])

  useEffect(() => {
    if (typeof document === 'undefined') return
    document.documentElement.dataset.novaMotion = normalized
  }, [normalized])

  return (
    <NovaMotionContext.Provider value={settings}>
      <MotionConfig reducedMotion={reducedMotion}>
        {children}
      </MotionConfig>
    </NovaMotionContext.Provider>
  )
}

export function useNovaMotion() {
  return useContext(NovaMotionContext)
}
