import { parseJsonEventStream, uiMessageChunkSchema, type UIMessageChunk } from 'ai'
import type { SSEEvent } from './types'
import i18next from '@/i18n'
import { toast } from 'sonner'

export const jsonHeaders = { 'Content-Type': 'application/json' }
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
  readonly payload: Record<string, unknown>

  constructor(message: string, options: { status: number; code?: string; details?: Record<string, unknown>; payload?: Record<string, unknown> }) {
    super(message)
    this.name = 'APIError'
    this.status = options.status
    this.code = options.code
    this.details = options.details
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
    const message = typeof data.error === 'string' && data.error ? data.error : `HTTP ${res.status}`
    const code = typeof data.code === 'string' && data.code ? data.code : undefined
    const details = data.details && typeof data.details === 'object' && !Array.isArray(data.details)
      ? data.details as Record<string, unknown>
      : undefined
    throw new APIError(message, { status: res.status, code, details, payload: data })
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
  let scanOffset = 0

  return new ReadableStream<T>({
    async pull(controller) {
      while (true) {
        const { done, value } = await reader.read()
        if (done) {
          buffer += decoder.decode()
          enqueueSSEBlocks(controller, true)
          controller.close()
          reader.releaseLock()
          return
        }
        buffer += decoder.decode(value, { stream: true })
        if (enqueueSSEBlocks(controller, false) > 0) return
      }
    },
    async cancel(reason) {
      await reader.cancel(reason)
      reader.releaseLock()
    },
  })

  function enqueueSSEBlocks(controller: ReadableStreamDefaultController<T>, flushRemainder: boolean) {
    let enqueued = 0
    while (true) {
      const boundary = findSSEBoundary(buffer, scanOffset)
      if (!boundary) break
      const block = buffer.slice(0, boundary.index)
      buffer = buffer.slice(boundary.index + boundary.length)
      scanOffset = 0
      if (enqueueSSEBlock(controller, block)) enqueued += 1
    }
    if (flushRemainder) {
      if (enqueueSSEBlock(controller, buffer)) enqueued += 1
      buffer = ''
      scanOffset = 0
    } else {
      // Only rescan the suffix that can contain a separator split across chunks.
      scanOffset = Math.max(0, buffer.length - 3)
    }
    return enqueued
  }
}

function findSSEBoundary(value: string, fromIndex: number): { index: number; length: number } | null {
  const lineFeed = value.indexOf('\n\n', fromIndex)
  const carriageReturn = value.indexOf('\r\r', fromIndex)
  const crlf = value.indexOf('\r\n\r\n', fromIndex)
  const candidates = [
    lineFeed >= 0 ? { index: lineFeed, length: 2 } : null,
    carriageReturn >= 0 ? { index: carriageReturn, length: 2 } : null,
    crlf >= 0 ? { index: crlf, length: 4 } : null,
  ].filter((candidate): candidate is { index: number; length: number } => candidate !== null)
  if (candidates.length === 0) return null
  return candidates.reduce((earliest, candidate) => candidate.index < earliest.index ? candidate : earliest)
}

function enqueueSSEBlock<T extends SSEEvent>(controller: ReadableStreamDefaultController<T>, block: string) {
  if (!block.trim()) return false
  let event = ''
  const data: string[] = []
  for (const line of block.split(/\r\n|\r|\n/)) {
    if (!line || line.startsWith(':')) continue
    const separator = line.indexOf(':')
    const field = separator >= 0 ? line.slice(0, separator) : line
    const rawValue = separator >= 0 ? line.slice(separator + 1) : ''
    const value = rawValue.startsWith(' ') ? rawValue.slice(1) : rawValue
    if (field === 'event') event = value
    else if (field === 'data') data.push(value)
  }
  if (!event) return false
  controller.enqueue({ event, data: data.join('\n') } as T)
  return true
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
