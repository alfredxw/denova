import { afterEach, describe, expect, it } from 'vitest'
import i18n from '@/i18n'
import { APIError } from '@/lib/api-client/client'
import { platformError } from './api'

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
