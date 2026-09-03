import { describe, expect, it } from 'vitest'

import { localizeAgentRuntimeError, localizeAgentRuntimeReason, MODEL_OUTPUT_TRUNCATED_CODE } from './agent-runtime-error'

const t = (key: string) => key

describe('agent runtime error localization', () => {
  it('localizes truncated model output from both live and recovered terminals', () => {
    expect(localizeAgentRuntimeError({ code: MODEL_OUTPUT_TRUNCATED_CODE, message: 'internal' }, 'fallback', t))
      .toBe('common.modelOutputTruncated')
    expect(localizeAgentRuntimeReason(MODEL_OUTPUT_TRUNCATED_CODE, 'fallback', t))
      .toBe('common.modelOutputTruncated')
  })

  it('preserves ordinary runtime diagnostics', () => {
    expect(localizeAgentRuntimeError({ message: 'provider failed' }, 'fallback', t)).toBe('provider failed')
  })
})
