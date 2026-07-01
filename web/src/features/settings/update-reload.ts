import { fetchAPI } from '@/lib/api-client'

export const UPDATE_RELOAD_POLL_INTERVAL_MS = 1000

type ReloadOptions = {
  pollBackend?: () => Promise<boolean>
  reload?: (href: string) => void
  setTimer?: (handler: () => void, delay: number) => unknown
  now?: () => number
  currentHref?: string
}

export function scheduleFrontendReloadAfterUpdate(version: string | undefined, options: ReloadOptions = {}) {
  if (typeof window === 'undefined' && !options.setTimer) return
  const pollBackend = options.pollBackend ?? defaultPollBackend
  const reload = options.reload ?? ((href: string) => window.location.replace(href))
  const setTimer = options.setTimer ?? ((handler: () => void, delay: number) => window.setTimeout(handler, delay))
  const now = options.now ?? (() => Date.now())
  const currentHref = () => options.currentHref ?? window.location.href

  const poll = () => {
    pollBackend()
      .then((ready) => {
        if (ready) {
          reload(buildFrontendReloadURL(version, now(), currentHref()))
          return
        }
        setTimer(poll, UPDATE_RELOAD_POLL_INTERVAL_MS)
      })
      .catch(() => {
        setTimer(poll, UPDATE_RELOAD_POLL_INTERVAL_MS)
      })
  }

  setTimer(poll, UPDATE_RELOAD_POLL_INTERVAL_MS)
}

export function buildFrontendReloadURL(version: string | undefined, nonce: number, currentHref: string) {
  const url = new URL(currentHref)
  const label = sanitizeReloadLabel(version) || 'update'
  url.searchParams.set('denova_reload', `${label}-${nonce}`)
  return url.toString()
}

async function defaultPollBackend() {
  const res = await fetchAPI(`/api/status?denova_reload_probe=${Date.now()}`, {
    cache: 'no-store',
    suppressBackendUnavailableToast: true,
  })
  return res.ok
}

function sanitizeReloadLabel(value: string | undefined) {
  return (value || '').trim().replace(/[^a-zA-Z0-9._-]+/g, '-').replace(/^-+|-+$/g, '')
}
