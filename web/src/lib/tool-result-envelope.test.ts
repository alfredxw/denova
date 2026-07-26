import { describe, expect, it } from 'vitest'
import { decodeToolResultEnvelope } from './tool-result-envelope'

describe('decodeToolResultEnvelope', () => {
  it('decodes process metadata from the first line without parsing payload output', () => {
    const result = [
      JSON.stringify({
        schema: 'process.result.v1',
        status: 'failed',
        exit_code: 2,
        output_truncated: false,
        recovery: { retryable: true, suggestion: 'Correct the command. / 修正命令。' },
      }),
      'stderr payload',
    ].join('\n')

    expect(decodeToolResultEnvelope(result)).toMatchObject({
      schema: 'process.result.v1',
      status: 'failed',
      severity: 'error',
      exitCode: 2,
      recovery: 'Correct the command. / 修正命令。',
    })
  })

  it('surfaces read/search continuation metadata as a warning', () => {
    expect(decodeToolResultEnvelope(`${JSON.stringify({
      schema: 'resource.read.v1',
      status: 'partial',
      limits: { truncated: true, next_offset: 80 },
    })}\ncontent`)).toMatchObject({
      severity: 'warning',
      truncated: true,
      continuation: { kind: 'offset', value: '80' },
    })

    expect(decodeToolResultEnvelope(`${JSON.stringify({
      schema: 'workspace.search.v1',
      status: 'partial',
      limits: { truncated: true, next_cursor: 'opaque-cursor' },
    })}\nmatch`)).toMatchObject({
      severity: 'warning',
      continuation: { kind: 'cursor', value: 'opaque-cursor' },
    })
  })

  it('ignores unknown and oversized envelopes', () => {
    expect(decodeToolResultEnvelope('{"schema":"custom.v1","status":"failed"}')).toBeNull()
    expect(decodeToolResultEnvelope(`{"schema":"process.result.v1","padding":"${'x'.repeat(70_000)}"}\npayload`)).toBeNull()
  })
})
