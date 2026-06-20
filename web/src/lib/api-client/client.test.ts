import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'sonner'
import { setConfiguredLocale } from '@/i18n'
import { fetchAPI, requestJSON } from './client'

vi.mock('sonner', () => ({
  toast: {
    error: vi.fn(),
  },
}))

describe('api client backend availability toast', () => {
  beforeEach(() => {
    setConfiguredLocale('zh-CN')
    vi.mocked(toast.error).mockClear()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows a deduped backend-unavailable toast for local API gateway failures', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('bad gateway', { status: 502 })))

    await expect(requestJSON('/api/workspace/current')).rejects.toThrow('bad gateway')

    expect(toast.error).toHaveBeenCalledWith('后端未启动', {
      id: 'nova-backend-unavailable',
      description: '请先启动或重启 Nova 后端服务，然后再继续操作。',
    })
  })

  it('shows the same backend-unavailable toast for local API network failures', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => {
      throw new TypeError('Failed to fetch')
    }))

    await expect(fetchAPI('/api/books')).rejects.toThrow('Failed to fetch')

    expect(toast.error).toHaveBeenCalledWith('后端未启动', {
      id: 'nova-backend-unavailable',
      description: '请先启动或重启 Nova 后端服务，然后再继续操作。',
    })
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
})
