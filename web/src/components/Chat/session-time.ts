import { useEffect, useState } from 'react'
import { getResolvedLocale } from '@/i18n'

const SECOND_MS = 1_000
const MINUTE_MS = 60 * SECOND_MS
const HOUR_MS = 60 * MINUTE_MS
const DAY_MS = 24 * HOUR_MS
const COMPACT_DAY_LIMIT = 7

/** Compact relative time shared by dense session selectors. */
export function formatCompactSessionTime(
  value: string | Date | number | undefined | null,
  now = Date.now(),
): string {
  if (!value) return ''
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return ''

  const elapsed = Math.max(0, now - date.getTime())
  if (elapsed < MINUTE_MS) return `${Math.floor(elapsed / SECOND_MS)}s`
  if (elapsed < HOUR_MS) return `${Math.floor(elapsed / MINUTE_MS)}m`
  if (elapsed < DAY_MS) return `${Math.floor(elapsed / HOUR_MS)}h`
  if (elapsed < COMPACT_DAY_LIMIT * DAY_MS) return `${Math.floor(elapsed / DAY_MS)}d`

  return new Intl.DateTimeFormat(getResolvedLocale(), {
    month: 'short',
    day: 'numeric',
  }).format(date)
}

/** Keeps visible relative times current without coupling them to session refresh requests. */
export function useSessionTimeNow(active = true): number {
  const [now, setNow] = useState(Date.now)

  useEffect(() => {
    if (!active) return
    setNow(Date.now())
    const interval = window.setInterval(() => setNow(Date.now()), SECOND_MS)
    return () => window.clearInterval(interval)
  }, [active])

  return now
}
