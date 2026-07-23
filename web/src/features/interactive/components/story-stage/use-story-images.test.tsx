import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAgentCommandID } from '@/lib/api'
import type { ChatMessage } from '@/lib/api'
import { generateInteractiveImage } from '../../api'
import type { Snapshot } from '../../types'
import { automaticInteractiveImageCommandID, useStoryImages } from './use-story-images'

vi.mock('@/lib/api', () => ({ createAgentCommandID: vi.fn() }))
vi.mock('../../api', () => ({ generateInteractiveImage: vi.fn() }))

const snapshot: Snapshot = {
  story_id: 'story-1',
  branch_id: 'main',
  state: {},
  turns: [{
    id: 'turn-1', parent_id: null, branch_id: 'main', ts: '2026-07-22T00:00:00Z',
    user: 'continue', narrative: 'the story continues',
  }],
}

function renderStoryImages() {
  return renderHook(() => useStoryImages({
    stageKey: 'story-1:main',
    storyId: 'story-1',
    branchId: 'main',
    snapshot,
    t: ((key: string) => key) as never,
    onDone: vi.fn().mockResolvedValue(snapshot),
    setActivity: vi.fn(),
    setMessages: vi.fn(),
  }))
}

describe('useStoryImages command identity', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(createAgentCommandID).mockReturnValueOnce('manual-image-1').mockReturnValueOnce('manual-image-2')
  })

  it('retains a manual command after an uncertain failure and releases it after 2xx', async () => {
    vi.mocked(generateInteractiveImage)
      .mockRejectedValueOnce(new TypeError('Failed to fetch'))
      .mockResolvedValue({ enabled: true, skipped: true })
    const hook = renderStoryImages()
    const message = { role: 'assistant', content: 'scene', turn_id: 'turn-1' } as ChatMessage

    await act(async () => { await hook.result.current.generateForMessage(message) })
    await act(async () => { await hook.result.current.generateForMessage(message) })
    await act(async () => { await hook.result.current.generateForMessage(message) })

    expect(vi.mocked(generateInteractiveImage).mock.calls.map(([, request]) => request.command_id)).toEqual([
      'manual-image-1',
      'manual-image-1',
      'manual-image-2',
    ])
  })

  it('retains a manual command after 5xx but releases it after an explicit 4xx', async () => {
    vi.mocked(generateInteractiveImage)
      .mockRejectedValueOnce(Object.assign(new Error('server unavailable'), { status: 503 }))
      .mockRejectedValueOnce(Object.assign(new Error('rejected'), { status: 409 }))
      .mockResolvedValueOnce({ enabled: true, skipped: true })
    const hook = renderStoryImages()
    const message = { role: 'assistant', content: 'scene', turn_id: 'turn-1' } as ChatMessage

    await act(async () => { await hook.result.current.generateForMessage(message) })
    await act(async () => { await hook.result.current.generateForMessage(message) })
    await act(async () => { await hook.result.current.generateForMessage(message) })

    expect(vi.mocked(generateInteractiveImage).mock.calls.map(([, request]) => request.command_id)).toEqual([
      'manual-image-1',
      'manual-image-1',
      'manual-image-2',
    ])
  })

  it('uses the same bounded command for repeated automatic generation of one turn', async () => {
    vi.mocked(generateInteractiveImage).mockResolvedValue({ enabled: true, skipped: true })
    const hook = renderStoryImages()

    await act(async () => { await hook.result.current.maybeGenerateAutomatically(snapshot) })
    await act(async () => { await hook.result.current.maybeGenerateAutomatically(snapshot) })

    const expected = automaticInteractiveImageCommandID('story-1', 'main', 'turn-1')
    expect(expected.length).toBeLessThan(256)
    expect(vi.mocked(generateInteractiveImage).mock.calls.map(([, request]) => request.command_id)).toEqual([expected, expected])
  })
})
