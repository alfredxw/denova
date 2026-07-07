import { useEffect } from 'react'
import type { ReactNode } from 'react'
import { MotionConfig, useReducedMotion } from 'motion/react'

export type MotionIntensity = 'system' | 'full' | 'reduced' | 'off'

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

  useEffect(() => {
    if (typeof document === 'undefined') return
    document.documentElement.dataset.novaMotion = normalized
  }, [normalized])

  return (
    <MotionConfig reducedMotion={reducedMotion}>
      {children}
    </MotionConfig>
  )
}
