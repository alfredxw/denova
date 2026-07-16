import { useEffect, useState } from 'react'

// The desktop workbench combines a resizable activity bar, project tree, editor,
// and review/agent panel. Tablet widths cannot satisfy those minimum sizes, so
// they use the existing compact shell instead of clipping interactive controls.
const DEFAULT_MOBILE_QUERY = '(max-width: 1023px)'

export function useIsMobile(query = DEFAULT_MOBILE_QUERY) {
  const [matches, setMatches] = useState(() => {
    if (typeof window === 'undefined') return false
    return window.matchMedia(query).matches
  })

  useEffect(() => {
    if (typeof window === 'undefined') return
    const mediaQuery = window.matchMedia(query)
    const updateMatches = () => setMatches(mediaQuery.matches)

    updateMatches()
    mediaQuery.addEventListener('change', updateMatches)
    return () => mediaQuery.removeEventListener('change', updateMatches)
  }, [query])

  return matches
}
