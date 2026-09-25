import { describe, expect, it } from 'vitest'

import {
  localizeAgentRuntimeError,
  localizeAgentRuntimeReason,
  MODEL_CONTEXT_WINDOW_EXCEEDED_CODE,
  MODEL_IMAGE_INPUT_REJECTED_CODE,
  MODEL_REQUEST_TOO_LARGE_CODE,
  MODEL_OUTPUT_FILTERED_CODE,
  MODEL_OUTPUT_INCOMPLETE_CODE,
  MODEL_OUTPUT_TRUNCATED_CODE,
} from './agent-runtime-error'

const t = (key: string) => key

describe('agent runtime error localization', () => {
  it('localizes product runtime error keys for live and recovered errors', () => {
    expect(localizeAgentRuntimeError({ error_key: 'agentRuntime.operationFailed' }, 'fallback', t))
      .toBe('agentRuntime.operationFailed')
    expect(localizeAgentRuntimeError({ error_key: 'agentRuntime.interrupted', message: 'internal' }, 'fallback', t))
      .toBe('agentRuntime.interrupted')
  })

  it('localizes truncated model output from both live and recovered terminals', () => {
    expect(localizeAgentRuntimeError({ code: MODEL_OUTPUT_TRUNCATED_CODE, message: 'internal' }, 'fallback', t))
      .toContain('common.modelOutputTruncated')
    expect(localizeAgentRuntimeReason(MODEL_OUTPUT_TRUNCATED_CODE, 'fallback', t))
      .toBe('common.modelOutputTruncated')
  })

  it.each([
    [MODEL_IMAGE_INPUT_REJECTED_CODE, 'common.modelImageInputRejected'],
    [MODEL_REQUEST_TOO_LARGE_CODE, 'common.modelRequestTooLarge'],
    [MODEL_CONTEXT_WINDOW_EXCEEDED_CODE, 'common.modelContextWindowExceeded'],
    [MODEL_OUTPUT_FILTERED_CODE, 'common.modelOutputFiltered'],
    [MODEL_OUTPUT_INCOMPLETE_CODE, 'common.modelOutputIncomplete'],
  ])('localizes distinct incomplete reason %s', (code, key) => {
    expect(localizeAgentRuntimeError({ code, message: 'internal' }, 'fallback', t)).toContain(key)
    expect(localizeAgentRuntimeError({ code, message: 'internal' }, 'fallback', t)).toContain(code)
    expect(localizeAgentRuntimeReason(code, 'fallback', t)).toBe(key)
  })

  it('preserves ordinary runtime diagnostics', () => {
    expect(localizeAgentRuntimeError({ message: 'provider failed' }, 'fallback', t)).toBe('provider failed')
  })
})
