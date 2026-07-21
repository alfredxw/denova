import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import type { UpdateCheckResult } from '@/features/settings/types'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { useWorkbenchNotice } from './use-workbench-notice'

describe('useWorkbenchNotice', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('shows the Star prompt only after a newly completed Agent turn', () => {
    const { result, rerender } = renderHook(
      ({ messages, isStreaming }) => useWorkbenchNotice({ messages, isStreaming }),
      { initialProps: { messages: [] as AgentUIMessage[], isStreaming: false } },
    )

    expect(result.current.notice).toBeNull()

    act(() => rerender({ messages: [], isStreaming: true }))
    act(() => rerender({ messages: [assistantMessage('assistant-1')], isStreaming: false }))

    expect(result.current.notice).toEqual({ kind: 'star' })
  })

  it('keeps updates higher priority and does not immediately replace a dismissed update', () => {
    const { result, rerender } = renderHook(
      ({ messages, isStreaming }) => useWorkbenchNotice({ messages, isStreaming }),
      { initialProps: { messages: [] as AgentUIMessage[], isStreaming: true } },
    )

    act(() => result.current.applyUpdateCheckResult(updateResult('v0.4.0')))
    act(() => rerender({ messages: [assistantMessage('assistant-1')], isStreaming: false }))
    expect(result.current.notice).toEqual({ kind: 'update', latestVersion: 'v0.4.0' })

    act(() => result.current.dismissNotice())
    expect(result.current.notice).toBeNull()
    expect(window.localStorage.getItem('nova.update.dismissedLatestVersion')).toBe('v0.4.0')

    act(() => rerender({ messages: [assistantMessage('assistant-1')], isStreaming: true }))
    act(() => rerender({
      messages: [assistantMessage('assistant-1'), assistantMessage('assistant-2')],
      isStreaming: false,
    }))
    expect(result.current.notice).toEqual({ kind: 'star' })
  })

  it('permanently quiets the Star prompt after it is dismissed', () => {
    const { result, rerender, unmount } = renderHook(
      ({ messages, isStreaming }) => useWorkbenchNotice({ messages, isStreaming }),
      { initialProps: { messages: [] as AgentUIMessage[], isStreaming: true } },
    )

    act(() => rerender({ messages: [assistantMessage('assistant-1')], isStreaming: false }))
    act(() => result.current.dismissNotice())

    expect(window.localStorage.getItem('nova.starNotice.dismissed')).toBe('true')
    expect(result.current.notice).toBeNull()
    unmount()

    const next = renderHook(
      ({ messages, isStreaming }) => useWorkbenchNotice({ messages, isStreaming }),
      { initialProps: { messages: [] as AgentUIMessage[], isStreaming: true } },
    )
    act(() => next.rerender({ messages: [assistantMessage('assistant-2')], isStreaming: false }))
    expect(next.result.current.notice).toBeNull()
  })
})

function assistantMessage(id: string): AgentUIMessage {
  return {
    id,
    role: 'assistant',
    parts: [{ type: 'text', text: `result from ${id}`, state: 'done' }],
  }
}

function updateResult(latestVersion: string): UpdateCheckResult {
  return {
    current_version: 'v0.3.0',
    latest_version: latestVersion,
    update_available: true,
    can_install: true,
    platform: 'darwin-arm64',
    release_url: 'https://github.com/alfredxw/denova/releases/latest',
    published_at: '2026-07-22T00:00:00Z',
  }
}
