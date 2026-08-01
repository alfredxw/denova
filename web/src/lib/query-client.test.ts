import { describe, expect, it } from 'vitest'
import { APIError } from '@/lib/api-client'
import { queryClient } from './query-client'

describe('query retry policy', () => {
  const retry = queryClient.getDefaultOptions().queries?.retry as (failureCount: number, error: unknown) => boolean

  it('does not repeat deterministic client failures', () => {
    expect(retry(0, new APIError('missing', { status: 404 }))).toBe(false)
    expect(retry(0, new APIError('conflict', { status: 409 }))).toBe(false)
  })

  it('retries one transient transport or server failure', () => {
    expect(retry(0, new Error('network unavailable'))).toBe(true)
    expect(retry(0, new APIError('unavailable', { status: 503 }))).toBe(true)
    expect(retry(1, new APIError('unavailable', { status: 503 }))).toBe(false)
  })
})
