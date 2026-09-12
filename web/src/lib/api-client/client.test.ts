import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'sonner'
import { queryClient } from '@/lib/query-client'
import { setConfiguredLocale } from '@/i18n'
import { APIError, fetchAPI, parseSSEStream, requestJSON, responseAPIError, withErrorLogID } from './client'

vi.mock('sonner', () => ({
  toast: {
    error: vi.fn(),
    dismiss: vi.fn(),
  },
}))

describe('api client backend availability toast', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    setConfiguredLocale('zh-CN')
    vi.mocked(toast.error).mockClear()
    window.sessionStorage.clear()
  })

  afterEach(async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{}')))
    await fetchAPI('/api/auth/status')
    vi.clearAllTimers()
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('shows a deduped backend-unavailable toast for local API gateway failures', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('bad gateway', { status: 502 })))

    await expect(requestJSON('/api/workspace/current')).rejects.toThrow('bad gateway')
    expect(toast.error).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1500)

    expect(toast.error).toHaveBeenCalledWith('无法连接 Denova', {
      id: 'nova-backend-unavailable',
      description: '请检查此设备的网络及 Denova 服务所在设备的连接，然后重试。',
    })
  })

  it('shows the same backend-unavailable toast for local API network failures', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => {
      throw new TypeError('Failed to fetch')
    }))

    await expect(fetchAPI('/api/books')).rejects.toThrow('Failed to fetch')
    expect(toast.error).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1500)

    expect(toast.error).toHaveBeenCalledWith('无法连接 Denova', {
      id: 'nova-backend-unavailable',
      description: '请检查此设备的网络及 Denova 服务所在设备的连接，然后重试。',
    })
  })

  it('does not report a backend outage when a PWA wakeup failure is followed by a reachable service', async () => {
    const fetchMock = vi.fn()
      .mockRejectedValueOnce(new TypeError('Load failed'))
      .mockResolvedValue(new Response('{}'))
    vi.stubGlobal('fetch', fetchMock)
    await expect(fetchAPI('/api/books')).rejects.toThrow('Load failed')
    await vi.advanceTimersByTimeAsync(1500)
    expect(fetchMock).toHaveBeenLastCalledWith('/api/auth/status', expect.objectContaining({ cache: 'no-store' }))
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('coalesces concurrent failures and cancels verification when another API request recovers', async () => {
    const fetchMock = vi.fn(async () => new Response('', { status: 503 }))
    vi.stubGlobal('fetch', fetchMock)
    await Promise.all([fetchAPI('/api/books'), fetchAPI('/api/settings')])
    await vi.advanceTimersByTimeAsync(1500)
    expect(fetchMock).toHaveBeenCalledTimes(3)
    expect(toast.error).toHaveBeenCalledTimes(1)
    fetchMock.mockImplementation(async () => new Response('{}'))
    await fetchAPI('/api/settings')
    expect(toast.dismiss).toHaveBeenCalledWith('nova-backend-unavailable')

    fetchMock.mockImplementationOnce(async () => new Response('', { status: 502 }))
    await fetchAPI('/api/books')
    await fetchAPI('/api/settings')
    await vi.advanceTimersByTimeAsync(1500)
    expect(fetchMock).toHaveBeenCalledTimes(6)
    expect(toast.error).toHaveBeenCalledTimes(1)
  })

  it('ignores late verification failures after a newer successful request', async () => {
    let rejectProbe!: (error: Error) => void
    const fetchMock = vi.fn()
      .mockRejectedValueOnce(new TypeError('Load failed'))
      .mockImplementationOnce(() => new Promise<Response>((_resolve, reject) => { rejectProbe = reject }))
      .mockResolvedValue(new Response('{}'))
    vi.stubGlobal('fetch', fetchMock)
    await expect(fetchAPI('/api/books')).rejects.toThrow('Load failed')
    await vi.advanceTimersByTimeAsync(1500)
    await fetchAPI('/api/settings')
    rejectProbe(new TypeError('Load failed'))
    await vi.advanceTimersByTimeAsync(5000)
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('does not queue stale outage notifications while the app is hidden', async () => {
    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    const fetchMock = vi.fn(async () => { throw new TypeError('Load failed') })
    vi.stubGlobal('fetch', fetchMock)
    await expect(fetchAPI('/api/books')).rejects.toThrow('Load failed')
    visibility.mockReturnValue('visible')
    await vi.advanceTimersByTimeAsync(10_000)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('treats an authentication challenge as a reachable server and never retries a mutation', async () => {
    const fetchMock = vi.fn()
      .mockRejectedValueOnce(new TypeError('Load failed'))
      .mockResolvedValue(new Response('', { status: 401 }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(fetchAPI('/api/chat', { method: 'POST', body: 'message' })).rejects.toThrow('Load failed')
    await vi.advanceTimersByTimeAsync(1500)
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(fetchMock.mock.calls[1]?.[0]).toBe('/api/auth/status')
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('reports a connection failure if the verification request times out', async () => {
    const fetchMock = vi.fn()
      .mockRejectedValueOnce(new TypeError('Load failed'))
      .mockImplementationOnce((_url, init) => new Promise<Response>((_resolve, reject) => {
        init.signal.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')), { once: true })
      }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(fetchAPI('/api/books')).rejects.toThrow('Load failed')
    await vi.advanceTimersByTimeAsync(6499)
    expect(toast.error).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    expect(toast.error).toHaveBeenCalledTimes(1)
  })

  it('cancels a pending notification when the PWA is backgrounded', async () => {
    const fetchMock = vi.fn(async () => { throw new TypeError('Load failed') })
    vi.stubGlobal('fetch', fetchMock)
    await expect(fetchAPI('/api/books')).rejects.toThrow('Load failed')
    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    document.dispatchEvent(new Event('visibilitychange'))
    visibility.mockReturnValue('visible')
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(6500)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('does not show backend-unavailable toast for cancelled or non-api requests', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => {
      throw new DOMException('aborted', 'AbortError')
    }))
    await expect(fetchAPI('/api/chat/stream')).rejects.toThrow('aborted')
    expect(toast.error).not.toHaveBeenCalled()

    vi.stubGlobal('fetch', vi.fn(async () => new Response('missing', { status: 502 })))
    await expect(fetchAPI('/assets/app.js')).resolves.toHaveProperty('status', 502)
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('can suppress backend-unavailable toast for expected API probes', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => {
      throw new TypeError('Failed to fetch')
    }))

    await expect(fetchAPI('/api/status', { suppressBackendUnavailableToast: true })).rejects.toThrow('Failed to fetch')

    expect(toast.error).not.toHaveBeenCalled()
  })

  it('uses browser cookies without storing or sending a Basic password', async () => {
    window.sessionStorage.setItem('nova.remoteAccess.credentials', JSON.stringify({ username: 'old', password: 'secret' }))
    const fetchMock = vi.fn(async () => new Response('{}', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await fetchAPI('/api/settings')
    const [, init] = (fetchMock.mock.calls as unknown as Array<[RequestInfo | URL, RequestInit]>)[0]
    expect(new Headers(init.headers).has('Authorization')).toBe(false)
  })

  it('revalidates authentication for a server cookie challenge', async () => {
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries').mockResolvedValue()
    vi.stubGlobal('fetch', vi.fn(async () => new Response('auth required', {
      status: 401, headers: { 'X-Denova-Auth': 'required' },
    })))
    await expect(requestJSON('/api/settings')).rejects.toThrow('auth required')
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['remote-access'] })
    invalidate.mockRestore()
  })

  it('preserves status, domain code and details for structured conflicts', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
      error: 'workspace revision changed',
      code: 'revision_conflict',
      request_id: '0198f2cb-e980-7a21-81ba-e4999869808c',
      details: { path: 'chapters/ch01.md', expected: 'sha256:old', actual: 'sha256:new' },
    }), { status: 409, headers: { 'Content-Type': 'application/json' } })))

    const error = await requestJSON('/api/projects/project-one/changes/groups/group-1/review').catch((reason) => reason)

    expect(error).toBeInstanceOf(APIError)
    if (!(error instanceof APIError)) {
      throw new TypeError('expected APIError')
    }
    expect(error).toMatchObject({
      requestID: '0198f2cb-e980-7a21-81ba-e4999869808c',
      status: 409,
      code: 'revision_conflict',
      details: { path: 'chapters/ch01.md', expected: 'sha256:old', actual: 'sha256:new' },
    })
    expect(error.message).toContain('workspace revision changed')
    expect(error.message).toContain('0198f2cb-e980-7a21-81ba-e4999869808c')
  })

  it('preserves status for streaming response failures', async () => {
    const error = await responseAPIError(new Response(JSON.stringify({
      error: 'command rejected', code: 'agent_runtime.invalid_command',
    }), { status: 400, headers: { 'Content-Type': 'application/json' } }))

    expect(error).toBeInstanceOf(APIError)
    expect(error).toMatchObject({ status: 400, code: 'agent_runtime.invalid_command', message: 'command rejected' })
  })

  it('uses the response header as the request ID fallback', async () => {
    const error = await responseAPIError(new Response('gateway failed', {
      status: 502,
      headers: { 'X-Request-ID': '0198f2cb-e980-7a21-81ba-e4999869808d' },
    }))

    expect(error.requestID).toBe('0198f2cb-e980-7a21-81ba-e4999869808d')
    expect(error.message).toContain('0198f2cb-e980-7a21-81ba-e4999869808d')
  })

  it('keeps a request ID when localized UI copy replaces the backend message', () => {
    const requestID = '0198f2cb-e980-7a21-81ba-e4999869808e'

    expect(withErrorLogID('保存失败', { request_id: requestID })).toBe(`保存失败 · 日志 ID: ${requestID}`)
    expect(withErrorLogID('保存失败', new Error(`upstream failed · Log ID: ${requestID}`))).toBe(`保存失败 · 日志 ID: ${requestID}`)
    expect(withErrorLogID(`保存失败 · 日志 ID: ${requestID}`, { request_id: requestID })).toBe(`保存失败 · 日志 ID: ${requestID}`)
  })
})

describe('parseSSEStream', () => {
  it('preserves split boundaries, multiline data, CRLF, and a final unterminated event', async () => {
    const source = [
      'id: 41\nevent: tool\ndata: first',
      '\ndata: second\n\n',
      'event: chunk\r\ndata: third\r\n\r',
      '\nevent: done\ndata: {}',
    ]
    const encoder = new TextEncoder()
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        for (const chunk of source) controller.enqueue(encoder.encode(chunk))
        controller.close()
      },
    })

    const reader = parseSSEStream(body).getReader()
    const events = []
    while (true) {
      const result = await reader.read()
      if (result.done) break
      events.push(result.value)
    }

    expect(events).toEqual([
      { id: '41', event: 'tool', data: 'first\nsecond' },
      { event: 'chunk', data: 'third' },
      { event: 'done', data: '{}' },
    ])
  })
})
