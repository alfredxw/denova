import { useCallback, useEffect, useState } from 'react'

/** Aggregates editor-section validity and resets when the active preset changes. */
export function usePresetSectionValidity(resetKey: string, onValidityChange?: (valid: boolean) => void) {
  const [validity, setValidity] = useState<Record<string, boolean>>({})

  useEffect(() => {
    setValidity({})
  }, [resetKey])

  useEffect(() => {
    onValidityChange?.(Object.values(validity).every((valid) => valid !== false))
  }, [onValidityChange, validity])

  return useCallback((section: string, valid: boolean) => {
    setValidity((current) => {
      if (current[section] === valid) return current
      return { ...current, [section]: valid }
    })
  }, [])
}
