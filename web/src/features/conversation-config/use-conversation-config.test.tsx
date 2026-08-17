import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { APIError } from '@/lib/api-client'
import type { ConversationConfigBinding, ConversationConfigSnapshot } from './types'
import { useConversationConfig } from './use-conversation-config'

const apiMocks = vi.hoisted(() => ({
  fetchConversationConfig: vi.fn(),
  patchConversationConfig: vi.fn(),
}))

vi.mock('./api', () => apiMocks)

const binding: ConversationConfigBinding = {
  mode: 'agent_chat',
  project_id: 'project-1',
  session_id: 'session-1',
}

describe('useConversationConfig', () => {
  beforeEach(() => vi.clearAllMocks())

  it('loads one durable snapshot and patches only the selected field', async () => {
    const initial = snapshot({ revision: 3 })
    const saved = snapshot({ thinking_level: 'high', revision: 4 })
    apiMocks.fetchConversationConfig.mockResolvedValue(initial)
    apiMocks.patchConversationConfig.mockResolvedValue(saved)

    const { result } = renderHook(() => useConversationConfig(binding))
    await waitFor(() => expect(result.current.initialized).toBe(true))

    await act(async () => {
      expect(await result.current.patch({ thinking_level: 'high' })).toBe(true)
    })

    expect(apiMocks.patchConversationConfig).toHaveBeenCalledWith(
      binding,
      { thinking_level: 'high' },
      3,
    )
    expect(result.current.snapshot).toEqual(saved)
  })

  it('rebases a stale revision once without expanding the field patch', async () => {
    const initial = snapshot({ revision: 1 })
    const latest = snapshot({ approval_mode: 'ask', revision: 5 })
    const saved = snapshot({ approval_mode: 'ask', thinking_level: 'max', revision: 6 })
    apiMocks.fetchConversationConfig
      .mockResolvedValueOnce(initial)
      .mockResolvedValueOnce(latest)
    apiMocks.patchConversationConfig
      .mockRejectedValueOnce(new APIError('revision conflict', { status: 409 }))
      .mockResolvedValueOnce(saved)

    const { result } = renderHook(() => useConversationConfig(binding))
    await waitFor(() => expect(result.current.snapshot).toEqual(initial))

    await act(async () => {
      expect(await result.current.patch({ thinking_level: 'max' })).toBe(true)
    })

    expect(apiMocks.patchConversationConfig).toHaveBeenNthCalledWith(
      1,
      binding,
      { thinking_level: 'max' },
      1,
    )
    expect(apiMocks.patchConversationConfig).toHaveBeenNthCalledWith(
      2,
      binding,
      { thinking_level: 'max' },
      5,
    )
    expect(result.current.snapshot).toEqual(saved)
  })

  it('does not initialize or issue requests without a complete surface binding', async () => {
    const { result } = renderHook(() => useConversationConfig())
    await act(async () => undefined)

    expect(result.current.initialized).toBe(false)
    expect(result.current.loading).toBe(false)
    expect(apiMocks.fetchConversationConfig).not.toHaveBeenCalled()
  })

  it('makes the previous snapshot unavailable immediately when the binding changes', async () => {
    const first = snapshot({ profile_id: 'first', revision: 2 })
    const second = snapshot({ profile_id: 'second', revision: 1 })
    let resolveSecond: ((value: ConversationConfigSnapshot) => void) | undefined
    apiMocks.fetchConversationConfig
      .mockResolvedValueOnce(first)
      .mockImplementationOnce(() => new Promise((resolve) => { resolveSecond = resolve }))

    const secondBinding = { ...binding, session_id: 'session-2' }
    const { result, rerender } = renderHook(
      ({ currentBinding }) => useConversationConfig(currentBinding),
      { initialProps: { currentBinding: binding } },
    )
    await waitFor(() => expect(result.current.snapshot).toEqual(first))

    rerender({ currentBinding: secondBinding })
    expect(result.current.snapshot).toBeNull()
    expect(result.current.initialized).toBe(false)
    expect(result.current.loading).toBe(true)

    await act(async () => { resolveSecond?.(second) })
    await waitFor(() => expect(result.current.snapshot).toEqual(second))
  })
})

function snapshot(patch: Partial<ConversationConfigSnapshot> = {}): ConversationConfigSnapshot {
  return {
    agent_kind: 'general',
    profile_id: 'default',
    thinking_level: 'medium',
    approval_mode: 'write',
    revision: 1,
    ...patch,
  }
}
