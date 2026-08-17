import { describe, expect, it } from 'vitest'
import { decodeToolResultEnvelope } from './tool-result-envelope'

describe('decodeToolResultEnvelope', () => {
  it('treats a completed non-zero process exit as a warning rather than a tool error', () => {
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
      severity: 'warning',
      exitCode: 2,
      recovery: 'Correct the command. / 修正命令。',
    })
  })

  it('treats bounded read/search pages with continuations as successful results', () => {
    expect(decodeToolResultEnvelope(`${JSON.stringify({
      schema: 'resource.read.v1',
      status: 'partial',
      limits: { truncated: true, next_offset: 80 },
    })}\ncontent`)).toMatchObject({
      severity: 'success',
      truncated: true,
      continuation: { kind: 'offset', value: '80' },
    })

    expect(decodeToolResultEnvelope(`${JSON.stringify({
      schema: 'workspace.search.v1',
      status: 'partial',
      limits: { truncated: true, next_cursor: 'opaque-cursor' },
    })}\nmatch`)).toMatchObject({
      severity: 'success',
      continuation: { kind: 'cursor', value: 'opaque-cursor' },
    })
  })

  it('keeps partial results without a continuation and truncated process output as warnings', () => {
    expect(decodeToolResultEnvelope(`${JSON.stringify({
      schema: 'resource.read.v1',
      status: 'partial',
      limits: { truncated: true },
    })}\ncontent`)).toMatchObject({
      severity: 'warning',
      truncated: true,
    })

    expect(decodeToolResultEnvelope(`${JSON.stringify({
      schema: 'process.result.v1',
      status: 'success',
      exit_code: 0,
      output_truncated: true,
    })}\noutput`)).toMatchObject({
      severity: 'warning',
      truncated: true,
    })
  })

  it('ignores unknown and oversized envelopes', () => {
    expect(decodeToolResultEnvelope('{"schema":"custom.v1","status":"failed"}')).toBeNull()
    expect(decodeToolResultEnvelope(`{"schema":"process.result.v1","padding":"${'x'.repeat(70_000)}"}\npayload`)).toBeNull()
  })
})
