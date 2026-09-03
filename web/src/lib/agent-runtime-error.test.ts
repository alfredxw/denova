import { describe, expect, it } from 'vitest'

import {
  localizeAgentRuntimeError,
  localizeAgentRuntimeReason,
  MODEL_CONTEXT_WINDOW_EXCEEDED_CODE,
  MODEL_OUTPUT_FILTERED_CODE,
  MODEL_OUTPUT_INCOMPLETE_CODE,
  MODEL_OUTPUT_TRUNCATED_CODE,
} from './agent-runtime-error'

const t = (key: string) => key

describe('agent runtime error localization', () => {
  it('localizes truncated model output from both live and recovered terminals', () => {
    expect(localizeAgentRuntimeError({ code: MODEL_OUTPUT_TRUNCATED_CODE, message: 'internal' }, 'fallback', t))
      .toBe('common.modelOutputTruncated')
    expect(localizeAgentRuntimeReason(MODEL_OUTPUT_TRUNCATED_CODE, 'fallback', t))
      .toBe('common.modelOutputTruncated')
  })

  it.each([
    [MODEL_CONTEXT_WINDOW_EXCEEDED_CODE, 'common.modelContextWindowExceeded'],
    [MODEL_OUTPUT_FILTERED_CODE, 'common.modelOutputFiltered'],
    [MODEL_OUTPUT_INCOMPLETE_CODE, 'common.modelOutputIncomplete'],
  ])('localizes distinct incomplete reason %s', (code, key) => {
    expect(localizeAgentRuntimeError({ code, message: 'internal' }, 'fallback', t)).toBe(key)
    expect(localizeAgentRuntimeReason(code, 'fallback', t)).toBe(key)
  })

  it('preserves ordinary runtime diagnostics', () => {
    expect(localizeAgentRuntimeError({ message: 'provider failed' }, 'fallback', t)).toBe('provider failed')
  })
})
