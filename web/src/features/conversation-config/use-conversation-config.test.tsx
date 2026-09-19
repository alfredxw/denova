import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { APIError } from '@/lib/api-client'
import { useConversationConfig } from './use-conversation-config'
import type { ConversationConfigSnapshot } from './types'

const api = vi.hoisted(() => ({ fetchConversationConfig: vi.fn(), patchConversationConfig: vi.fn() }))
vi.mock('./api', () => api)
vi.mock('@/features/settings/query', () => ({
  GLOBAL_SETTINGS_TARGET: 'global', settingsQueryKeys: { all: ['settings'] },
  subscribeSettingsTarget: () => () => {},
}))

const native: ConversationConfigSnapshot = {
  agent_kind: 'ide', profile_id: 'default', thinking_level: 'medium', approval_mode: 'write', revision: 1,
}
const external: ConversationConfigSnapshot = {
  ...native, revision: 2, runtime: { kind: 'codex', codex: { model: 'external-model' } },
}

describe('conversation runtime application', () => {
  beforeEach(() => {
    api.fetchConversationConfig.mockReset().mockResolvedValue(native)
    api.patchConversationConfig.mockReset().mockResolvedValue(external)
  })

  it('refreshes the same Writing session when Agents applies its runtime through AgentChat', async () => {
    const agents = renderHook(() => useConversationConfig({ mode: 'agent_chat', project_id: 'book', session_id: 'scene' }))
    const writing = renderHook(() => useConversationConfig({ mode: 'writing', project_id: 'book', session_id: 'scene' }))
    const other = renderHook(() => useConversationConfig({ mode: 'writing', project_id: 'other-book', session_id: 'scene' }))
    await waitFor(() => expect(writing.result.current.snapshot).toEqual(native))
    await waitFor(() => expect(agents.result.current.snapshot).toEqual(native))
    await waitFor(() => expect(other.result.current.snapshot).toEqual(native))
    await act(async () => { expect(await agents.result.current.patch({ runtime: external.runtime })).toBe(true) })
    expect(writing.result.current.snapshot).toEqual(external)
    expect(other.result.current.snapshot).toEqual(native)
    expect(api.patchConversationConfig).toHaveBeenCalledTimes(1)
  })

  it('shows a conflicting runtime application without silently retrying against the new revision', async () => {
    const view = renderHook(() => useConversationConfig({ mode: 'agent_chat', project_id: 'book', session_id: 'scene' }))
    await waitFor(() => expect(view.result.current.snapshot).toEqual(native))
    api.patchConversationConfig.mockRejectedValue(new APIError('Configuration changed', { status: 409 }))
    api.fetchConversationConfig.mockResolvedValue({ ...native, revision: 3 })
    await act(async () => { expect(await view.result.current.patch({ runtime: external.runtime })).toBe(false) })
    expect(api.patchConversationConfig).toHaveBeenCalledTimes(1)
    expect(view.result.current.snapshot?.revision).toBe(3)
    expect(view.result.current.error).toBe('Configuration changed')
  })
})
