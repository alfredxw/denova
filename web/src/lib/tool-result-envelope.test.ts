import { describe, expect, it } from 'vitest'
import { decodeToolResultEnvelope } from './tool-result-envelope'

describe('decodeToolResultEnvelope', () => {
  it('treats a bounded directory read without continuation as successful', () => {
    const result = decodeToolResultEnvelope(JSON.stringify({
      schema: 'resource.read.v1',
      status: 'partial',
      source: { kind: 'directory', path: '.' },
      limits: { returned: 200, truncated: true },
      recovery: { retryable: true, suggestion: 'Narrow the resource path or requested limit.' },
    }))

    expect(result).toMatchObject({
      schema: 'resource.read.v1',
      status: 'partial',
      severity: 'success',
      truncated: true,
    })
  })

  it('keeps an incomplete search without continuation as a warning', () => {
    const result = decodeToolResultEnvelope(JSON.stringify({
      schema: 'workspace.search.v1',
      status: 'partial',
      limits: { returned: 200, truncated: true },
    }))

    expect(result?.severity).toBe('warning')
  })
})
