const AUTO_UPDATE_CHECKED_AT_KEY = 'nova.update.lastAutoCheckAt'

export const AUTO_UPDATE_CHECK_INTERVAL_MS = 60 * 60 * 1000

export function shouldRunAutoUpdateCheck(now = Date.now(), storage = browserStorage()) {
  if (!storage) return true
  const raw = storage.getItem(AUTO_UPDATE_CHECKED_AT_KEY)
  if (!raw) return true
  const lastCheckedAt = Number(raw)
  if (!Number.isFinite(lastCheckedAt) || lastCheckedAt <= 0) return true
  return now - lastCheckedAt >= AUTO_UPDATE_CHECK_INTERVAL_MS
}

export function markAutoUpdateChecked(now = Date.now(), storage = browserStorage()) {
  if (!storage) return
  storage.setItem(AUTO_UPDATE_CHECKED_AT_KEY, String(now))
}

function browserStorage() {
  if (typeof window === 'undefined') return null
  return window.localStorage
}
