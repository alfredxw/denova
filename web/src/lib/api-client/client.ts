import { parseJsonEventStream, uiMessageChunkSchema, type UIMessageChunk } from 'ai'
import i18next from '@/i18n'
import { toast } from 'sonner'
import { queryClient } from '@/lib/query-client'

export { parseSSEStream } from './sse'

export const jsonHeaders = { 'Content-Type': 'application/json' }
const REQUEST_ID_HEADER = 'X-Request-ID'
const BACKEND_UNAVAILABLE_TOAST_ID = 'nova-backend-unavailable'
const BACKEND_UNAVAILABLE_STATUS = new Set([502, 503, 504])
// Give mobile foreground recovery and query retries time to restore the connection.
const BACKEND_CHECK_DELAY_MS = 1500
const BACKEND_CHECK_TIMEOUT_MS = 5000
let backendCheck: AbortController | undefined
let backendCheckTimer: ReturnType<typeof setTimeout> | undefined
let backendUnavailableShown = false

if (typeof document !== 'undefined') {
  document.addEventListener('visibilitychange', clearBackendUnavailable)
}

type APIRequestInit = RequestInit & {
  suppressBackendUnavailableToast?: boolean
}

/** HTTP/API domain failure with transport and machine-readable backend context intact. */
export class APIError extends Error {
  readonly status: number
  readonly code?: string
  readonly details?: Record<string, unknown>
  readonly requestID?: string
  readonly payload: Record<string, unknown>

  constructor(message: string, options: { status: number; code?: string; details?: Record<string, unknown>; requestID?: string; payload?: Record<string, unknown> }) {
    super(formatAPIErrorMessage(message, options.requestID))
    this.name = 'APIError'
    this.status = options.status
    this.code = options.code
    this.details = options.details
    this.requestID = options.requestID
    this.payload = options.payload ?? {}
  }
}

export async function fetchAPI(input: RequestInfo | URL, init?: APIRequestInit): Promise<Response> {
  const { suppressBackendUnavailableToast = false, ...baseInit } = init ?? {}
  const requestInit = baseInit
  try {
    const res = await fetch(input, requestInit)
    if (res.status < 500 && isLocalAPIRequest(input)) clearBackendUnavailable()
    if (!suppressBackendUnavailableToast) notifyBackendUnavailableIfNeeded(input, res.status)
    notifyRemoteAccessRequiredIfNeeded(input, res)
    return res
  } catch (error) {
    if (!suppressBackendUnavailableToast && shouldNotifyBackendUnavailable(input, error)) notifyBackendUnavailable()
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
    throw apiErrorFromPayload(res.status, data, res.headers.get(REQUEST_ID_HEADER))
  }
  return data as T
}

/** Preserve status and structured error details for streaming HTTP requests. */
export async function responseAPIError(res: Response): Promise<APIError> {
  const text = await res.text()
  let payload: Record<string, unknown> = {}
  if (text) {
    try {
      payload = JSON.parse(text) as Record<string, unknown>
    } catch {
      payload = { error: text }
    }
  }
  return apiErrorFromPayload(res.status, payload, res.headers.get(REQUEST_ID_HEADER))
}

function apiErrorFromPayload(status: number, payload: Record<string, unknown>, responseRequestID?: string | null): APIError {
  const message = typeof payload.error === 'string' && payload.error ? payload.error : `HTTP ${status}`
  const code = typeof payload.code === 'string' && payload.code ? payload.code : undefined
  const details = payload.details && typeof payload.details === 'object' && !Array.isArray(payload.details)
    ? payload.details as Record<string, unknown>
    : undefined
  const payloadRequestID = typeof payload.request_id === 'string' ? payload.request_id.trim() : ''
  const requestID = responseRequestID?.trim() || payloadRequestID || undefined
  return new APIError(message, { status, code, details, requestID, payload })
}

function formatAPIErrorMessage(message: string, requestID?: string): string {
  const normalized = requestID?.trim()
  if (!normalized) return message
  return `${message} · ${i18next.t('common.logId')}: ${normalized}`
}

/** Keeps localized UI copy while retaining the server correlation ID for support. */
export function withErrorLogID(message: string, source: unknown): string {
  const requestID = requestIDFromError(source)
  if (!requestID || message.includes(requestID)) return message
  return formatAPIErrorMessage(message, requestID)
}

function requestIDFromError(source: unknown, seen = new Set<object>()): string | undefined {
  if (typeof source === 'string') return requestIDFromText(source)
  if (!source || typeof source !== 'object' || seen.has(source)) return undefined
  seen.add(source)

  const record = source as Record<string, unknown>
  for (const key of ['requestID', 'requestId', 'request_id', 'logID', 'logId', 'log_id']) {
    const value = record[key]
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  if (typeof record.message === 'string') {
    const requestID = requestIDFromText(record.message)
    if (requestID) return requestID
  }
  for (const key of ['payload', 'details', 'cause']) {
    const requestID = requestIDFromError(record[key], seen)
    if (requestID) return requestID
  }
  return undefined
}

function requestIDFromText(value: string): string | undefined {
  const match = value.match(/(?:日志\s*ID(?:\s*\/\s*Log\s*ID)?|Log\s*ID|request_id)\s*[:=]\s*([A-Za-z0-9][A-Za-z0-9._:-]*)/i)
  return match?.[1]
}

export async function readErrorMessage(res: Response): Promise<string> {
  let message = `HTTP ${res.status}`
  let requestID = res.headers.get(REQUEST_ID_HEADER)?.trim() || undefined
  notifyBackendUnavailableIfNeeded(res.url || '/api', res.status)
  try {
    const data = await res.json()
    message = data.error || message
    requestID ||= (typeof data.request_id === 'string' && data.request_id.trim()) || undefined
  } catch {
    // keep HTTP fallback
  }
  return formatAPIErrorMessage(message, requestID)
}

export function parseUIMessageStream(body: ReadableStream<Uint8Array>): ReadableStream<UIMessageChunk> {
  return parseJsonEventStream({
    stream: body,
    schema: uiMessageChunkSchema,
  }).pipeThrough(new TransformStream({
    transform(chunk, controller) {
      if (!chunk.success) throw chunk.error
      controller.enqueue(chunk.value)
    },
  }))
}

/** Recheck the cookie with the server so a late 401 cannot discard a newer login. */
export function handleRemoteAccessChallenge() {
  void queryClient.invalidateQueries({ queryKey: ['remote-access'] })
}

function notifyBackendUnavailableIfNeeded(input: RequestInfo | URL, status: number) {
  if (!BACKEND_UNAVAILABLE_STATUS.has(status) || !isLocalAPIRequest(input)) return
  notifyBackendUnavailable()
}

function notifyRemoteAccessRequiredIfNeeded(input: RequestInfo | URL, res: Response) {
  if (res.status !== 401 || !isLocalAPIRequest(input)) return
  if (res.headers.get('X-Denova-Auth') !== 'required') return
  handleRemoteAccessChallenge()
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
  if (backendCheck || document.visibilityState === 'hidden') return
  const controller = new AbortController()
  backendCheck = controller
  backendCheckTimer = setTimeout(() => { void checkBackendAvailability(controller) }, BACKEND_CHECK_DELAY_MS)
}

async function checkBackendAvailability(controller: AbortController) {
  if (backendCheck !== controller) return
  if (document.visibilityState === 'hidden') {
    clearBackendUnavailable()
    return
  }
  const timeout = setTimeout(() => controller.abort(), BACKEND_CHECK_TIMEOUT_MS)
  let failure: unknown
  try {
    // Verify reachability independently; never replay a failed write or stream.
    // Auth status is available before login and must not be served from cache.
    const response = await fetch('/api/auth/status', { cache: 'no-store', signal: controller.signal })
    if (backendCheck !== controller) return
    if (response.status < 500) {
      clearBackendUnavailable()
      // Recovery may arrive after the startup query exhausted its retry.
      void queryClient.invalidateQueries({ queryKey: ['remote-access'], predicate: query => query.state.status === 'error' })
      return
    }
    failure = `HTTP ${response.status}`
  } catch (error) {
    failure = error
  } finally {
    clearTimeout(timeout)
  }
  if (backendCheck !== controller) return
  backendCheck = undefined
  if (document.hidden) return
  if (!backendUnavailableShown) console.warn('[api] Backend connection verification failed', failure)
  backendUnavailableShown = true
  toast.error(i18next.t('common.backendUnavailable.title'), {
    id: BACKEND_UNAVAILABLE_TOAST_ID,
    description: i18next.t('common.backendUnavailable.description'),
  })
}

function clearBackendUnavailable() {
  clearTimeout(backendCheckTimer)
  backendCheckTimer = undefined
  backendCheck?.abort()
  backendCheck = undefined
  if (backendUnavailableShown) {
    toast.dismiss(BACKEND_UNAVAILABLE_TOAST_ID)
    backendUnavailableShown = false
  }
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
