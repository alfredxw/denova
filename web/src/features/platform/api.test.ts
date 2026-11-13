import { afterEach, describe, expect, it, vi } from 'vitest'
import i18n from '@/i18n'
import { APIError } from '@/lib/api-client/client'
import { queryClient } from '@/lib/query-client'
import { remoteAccessQuery } from '@/features/remote-access/api'
import { hostOnlyManagement, management, platformError } from './api'

const originalLanguage = i18n.language
afterEach(async () => { await i18n.changeLanguage(originalLanguage) })

describe('platform error diagnostics', () => {
  it.each(['zh-CN', 'en-US'])('keeps localized copy and actionable diagnostics in %s', async language => {
    await i18n.changeLanguage(language)
    const message = platformError(new APIError('HTTP 503', {
      status: 503, code: 'RUNTIME_FAILED', requestID: 'request-story',
      details: { operation: 'GET /api/platform/manage/instances', backend_version: 'dev', platform: 'darwin/arm64' },
      payload: {
        messageKey: 'platform.errors.RUNTIME_FAILED',
        diagnostic: 'model context batch identity conflict: persisted batch identity changed',
        document: 'private story content',
      },
    }))
    expect(message).toContain(i18n.t('platform.errors.RUNTIME_FAILED'))
    for (const detail of ['identity conflict', 'RUNTIME_FAILED', '503', 'request-story', '/api/platform/manage/instances', 'darwin/arm64']) {
      expect(message).toContain(detail)
    }
    expect(message).not.toContain('private story content')
  })

  it('sanitizes platform diagnostics and falls back for unknown message keys', () => {
    const message = platformError(new APIError('HTTP 503', {
      status: 503,
      payload: { messageKey: 'unknown.key', diagnostic: 'open /Users/alice/private/story.jsonl: denied; api_key=secret-value' },
    }))
    expect(message).toContain(i18n.t('platform.errors.RUNTIME_FAILED'))
    expect(message).toContain('denied')
    expect(message).not.toContain('/Users/alice')
    expect(message).not.toContain('secret-value')
  })

  it('retains browser errors without inventing a server cause', () => {
    expect(platformError(new Error('Connection closed'))).toContain('Connection closed')
    expect(platformError(undefined)).toBe(i18n.t('platform.errors.RUNTIME_FAILED'))
  })
})

describe('host-bound management actions', () => {
  afterEach(() => { queryClient.removeQueries({ queryKey: remoteAccessQuery.queryKey }) })

  it('keeps reads and project configuration available to LAN browsers', () => {
    for (const [path, method] of [
      ['/runtimes', 'GET'],
      ['/catalog', 'GET'],
      ['/development', 'GET'],
      ['/packages/plugin', 'GET'],
      ['/instances/one', 'PATCH'],
      ['/instances/one', 'DELETE'],
      ['/runtimes/one/consumers/two', 'DELETE'],
      ['/projects/p1/plugins/example/content', 'POST'],
      ['/instances/one/stop', 'POST'],
      ['/runtimes/one/stop', 'POST'],
      ['/game-preferences', 'PATCH'],
      ['/projects/p1/extensions', 'PUT'],
    ]) {
      expect(hostOnlyManagement(path, method), `${method} ${path}`).toBe(false)
    }
  })

  it('marks runtime starts and package installation as host-bound', () => {
    for (const [path, method] of [
      ['/', 'POST'],
      ['/new-action', 'POST'],
      ['/runtimes/plugin/extra', 'POST'],
      ['/projects/p1/extensions/extra', 'PUT'],
      ['/instances', 'POST'],
      ['/instances/one/open', 'POST'],
      ['/instances/one/upgrade', 'POST'],
      ['/plugin-actions/open', 'POST'],
      ['/runtimes/plugin', 'POST'],
      ['/packages/install', 'POST'],
      ['/packages/plugin/example', 'PATCH'],
      ['/development/link', 'POST'],
      ['/candidates/one', 'DELETE'],
    ]) {
      expect(hostOnlyManagement(path, method), `${method} ${path}`).toBe(true)
    }
  })

  it('fails fast with a clear reason instead of requesting from a remote browser', async () => {
    queryClient.setQueryData(remoteAccessQuery.queryKey, { local: false, authenticated: true })
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    try {
      await expect(management('/instances/one/open', 'POST', {})).rejects.toMatchObject({ code: 'LOCAL_HOST_ONLY' })
      expect(fetchMock).not.toHaveBeenCalled()
    } finally {
      vi.unstubAllGlobals()
    }
  })
})
