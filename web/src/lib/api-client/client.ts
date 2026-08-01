import { parseJsonEventStream, uiMessageChunkSchema, type UIMessageChunk } from 'ai'
import i18next from '@/i18n'
import { toast } from 'sonner'

export { parseSSEStream } from './sse'

export const jsonHeaders = { 'Content-Type': 'application/json' }
const REQUEST_ID_HEADER = 'X-Request-ID'
const BACKEND_UNAVAILABLE_TOAST_ID = 'nova-backend-unavailable'
const BACKEND_UNAVAILABLE_STATUS = new Set([502, 503, 504])
const REMOTE_ACCESS_CREDENTIALS_KEY = 'nova.remoteAccess.credentials'
const REMOTE_ACCESS_REQUIRED_EVENT = 'nova:remote-access-required'

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
  const requestInit = withRemoteAccessAuth(input, baseInit)
  try {
    const res = await fetch(input, requestInit)
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

export function setRemoteAccessCredentials(username: string, password: string) {
  const credentials = { username: username.trim(), password }
  if (!credentials.username || !credentials.password) return
  window.sessionStorage.setItem(REMOTE_ACCESS_CREDENTIALS_KEY, JSON.stringify(credentials))
}

export function clearRemoteAccessCredentials() {
  window.sessionStorage.removeItem(REMOTE_ACCESS_CREDENTIALS_KEY)
}

/** Returns the tab-scoped Basic credential that a SharedWorker cannot read itself. */
export function getRemoteAccessAuthorization(): string | undefined {
  const credentials = readRemoteAccessCredentials()
  if (!credentials) return undefined
  return `Basic ${encodeBasicAuth(credentials.username, credentials.password)}`
}

/** Applies the same login challenge behavior used by regular API requests. */
export function handleRemoteAccessChallenge() {
  clearRemoteAccessCredentials()
  window.dispatchEvent(new CustomEvent(REMOTE_ACCESS_REQUIRED_EVENT))
}

function notifyBackendUnavailableIfNeeded(input: RequestInfo | URL, status: number) {
  if (!BACKEND_UNAVAILABLE_STATUS.has(status) || !isLocalAPIRequest(input)) return
  notifyBackendUnavailable()
}

function notifyRemoteAccessRequiredIfNeeded(input: RequestInfo | URL, res: Response) {
  if (res.status !== 401 || !isLocalAPIRequest(input)) return
  if (!res.headers.get('WWW-Authenticate')?.toLowerCase().includes('basic')) return
  handleRemoteAccessChallenge()
}

function withRemoteAccessAuth(input: RequestInfo | URL, init?: RequestInit): RequestInit | undefined {
  if (!isLocalAPIRequest(input)) return init
  const authorization = getRemoteAccessAuthorization()
  if (!authorization) return init
  const headers = new Headers(init?.headers ?? requestHeaders(input))
  if (!headers.has('Authorization')) {
    headers.set('Authorization', authorization)
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
