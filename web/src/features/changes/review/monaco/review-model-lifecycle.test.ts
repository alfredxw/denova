import { describe, expect, it, vi } from 'vitest'
import type { Monaco } from '@monaco-editor/react'
import { scheduleDetachedReviewModelDisposal } from './review-model-lifecycle'

describe('scheduleDetachedReviewModelDisposal', () => {
  it('disposes detached models after the editor cleanup and preserves attached models', async () => {
    const detached = model(false)
    const attached = model(true)
    const models = new Map([
      ['review://before', detached],
      ['review://after', attached],
    ])
    const monaco = {
      Uri: { parse: (path: string) => path },
      editor: { getModel: (path: string) => models.get(path) },
    } as unknown as Monaco

    scheduleDetachedReviewModelDisposal(monaco, ['review://before', 'review://after'])
    expect(detached.dispose).not.toHaveBeenCalled()

    await Promise.resolve()

    expect(detached.dispose).toHaveBeenCalledOnce()
    expect(attached.dispose).not.toHaveBeenCalled()
  })
})

function model(attached: boolean) {
  return {
    dispose: vi.fn(),
    isDisposed: () => false,
    isAttachedToEditor: () => attached,
  }
}
