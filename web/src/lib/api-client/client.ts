import type { SSEEvent } from './types'
import i18next from '@/i18n'
import { toast } from 'sonner'

export const jsonHeaders = { 'Content-Type': 'application/json' }
const BACKEND_UNAVAILABLE_TOAST_ID = 'nova-backend-unavailable'
const BACKEND_UNAVAILABLE_STATUS = new Set([502, 503, 504])
const REMOTE_ACCESS_CREDENTIALS_KEY = 'nova.remoteAccess.credentials'
const REMOTE_ACCESS_REQUIRED_EVENT = 'nova:remote-access-required'

export async function fetchAPI(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const requestInit = withRemoteAccessAuth(input, init)
  try {
    const res = await fetch(input, requestInit)
    notifyBackendUnavailableIfNeeded(input, res.status)
    notifyRemoteAccessRequiredIfNeeded(input, res)
    return res
  } catch (error) {
    if (shouldNotifyBackendUnavailable(input, error)) notifyBackendUnavailable()
    throw error
  }
}

export async function requestJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetchAPI(url, init)
  const text = await res.text()
  let data: Record<string, any> = {}
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = { error: text }
    }
  }
  if (!res.ok) {
    throw new Error(data.error || `HTTP ${res.status}`)
  }
  return data as T
}

export async function readErrorMessage(res: Response): Promise<string> {
  let message = `HTTP ${res.status}`
  notifyBackendUnavailableIfNeeded(res.url || '/api', res.status)
  try {
    const data = await res.json()
    message = data.error || message
  } catch {
    // keep HTTP fallback
  }
  return message
}

export function parseSSEStream<T extends SSEEvent = SSEEvent>(body: ReadableStream<Uint8Array>): ReadableStream<T> {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  return new ReadableStream<T>({
    async pull(controller) {
      while (true) {
        const { done, value } = await reader.read()
        if (done) {
          controller.close()
          return
        }
        buffer += decoder.decode(value, { stream: true })

        const events = buffer.split('\n\n')
        buffer = events.pop() || ''

        for (const eventStr of events) {
          if (!eventStr.trim()) continue
          const lines = eventStr.split('\n')
          let event = ''
          let data = ''
          for (const line of lines) {
            if (line.startsWith('event: ')) event = line.slice(7)
            else if (line.startsWith('data: ')) data = line.slice(6)
          }
          if (event) {
            controller.enqueue({ event, data } as T)
          }
        }
      }
    },
  })
}

export function setRemoteAccessCredentials(username: string, password: string) {
  const credentials = { username: username.trim(), password }
  if (!credentials.username || !credentials.password) return
  window.sessionStorage.setItem(REMOTE_ACCESS_CREDENTIALS_KEY, JSON.stringify(credentials))
}

export function clearRemoteAccessCredentials() {
  window.sessionStorage.removeItem(REMOTE_ACCESS_CREDENTIALS_KEY)
}

function notifyBackendUnavailableIfNeeded(input: RequestInfo | URL, status: number) {
  if (!BACKEND_UNAVAILABLE_STATUS.has(status) || !isLocalAPIRequest(input)) return
  notifyBackendUnavailable()
}

function notifyRemoteAccessRequiredIfNeeded(input: RequestInfo | URL, res: Response) {
  if (res.status !== 401 || !isLocalAPIRequest(input)) return
  if (!res.headers.get('WWW-Authenticate')?.toLowerCase().includes('basic')) return
  clearRemoteAccessCredentials()
  window.dispatchEvent(new CustomEvent(REMOTE_ACCESS_REQUIRED_EVENT))
}

function withRemoteAccessAuth(input: RequestInfo | URL, init?: RequestInit): RequestInit | undefined {
  if (!isLocalAPIRequest(input)) return init
  const credentials = readRemoteAccessCredentials()
  if (!credentials) return init
  const headers = new Headers(init?.headers ?? requestHeaders(input))
  if (!headers.has('Authorization')) {
    headers.set('Authorization', `Basic ${encodeBasicAuth(credentials.username, credentials.password)}`)
  }
  return { ...init, headers }
}

function readRemoteAccessCredentials(): { username: string; password: string } | null {
  try {
    const raw = window.sessionStorage.getItem(REMOTE_ACCESS_CREDENTIALS_KEY)
    if (!raw) return null
    const value = JSON.parse(raw) as { username?: string; password?: string }
    if (!value.username || !value.password) return null
    return { username: value.username, password: value.password }
  } catch {
    clearRemoteAccessCredentials()
    return null
  }
}

function requestHeaders(input: RequestInfo | URL): HeadersInit | undefined {
  if (typeof input === 'object' && !(input instanceof URL)) return input.headers
  return undefined
}

function encodeBasicAuth(username: string, password: string): string {
  const value = `${username}:${password}`
  return window.btoa(String.fromCharCode(...new TextEncoder().encode(value)))
}

function shouldNotifyBackendUnavailable(input: RequestInfo | URL, error: unknown): boolean {
  if (!isLocalAPIRequest(input) || isAbortError(error)) return false
  if (!(error instanceof Error)) return true
  const message = error.message.toLowerCase()
  return message.includes('failed to fetch') ||
    message.includes('networkerror') ||
    message.includes('load failed') ||
    message.includes('network request failed')
}

function notifyBackendUnavailable() {
  toast.error(i18next.t('common.backendUnavailable.title'), {
    id: BACKEND_UNAVAILABLE_TOAST_ID,
    description: i18next.t('common.backendUnavailable.description'),
  })
}

function isLocalAPIRequest(input: RequestInfo | URL): boolean {
  const url = requestURL(input)
  if (!url) return false
  if (url.startsWith('/api')) return true
  if (typeof window === 'undefined') return false
  try {
    const parsed = new URL(url, window.location.origin)
    return parsed.origin === window.location.origin && parsed.pathname.startsWith('/api')
  } catch {
    return false
  }
}

function requestURL(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

function isAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === 'AbortError'
}
