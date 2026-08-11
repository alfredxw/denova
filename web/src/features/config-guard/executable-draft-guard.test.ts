import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  discardExecutableDraft,
  hasPendingExecutableDraft,
  registerExecutableDraft,
  unregisterExecutableDraft,
  useExecutableDraftGuard,
} from './executable-draft-guard'

describe('executable draft guard registry', () => {
  beforeEach(() => {
    useExecutableDraftGuard.setState({ entries: {} })
  })

  it('reports pending state and delegates discard to the registered surface', () => {
    const discard = vi.fn()
    registerExecutableDraft('automations', { hasPending: true, discard })

    expect(hasPendingExecutableDraft('automations')).toBe(true)
    discardExecutableDraft('automations')
    expect(discard).toHaveBeenCalled()
  })

  it('clears entries after unregister', () => {
    registerExecutableDraft('skills', { hasPending: true, discard: vi.fn() })
    unregisterExecutableDraft('skills')
    expect(hasPendingExecutableDraft('skills')).toBe(false)
  })
})
