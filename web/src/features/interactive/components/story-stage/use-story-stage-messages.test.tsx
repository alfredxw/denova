import { act, renderHook } from '@testing-library/react'
import { useState } from 'react'
import { describe, expect, it } from 'vitest'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { agentViewToRenderMessage, buildAgentMessageViews } from '@/lib/agent-message-view'
import { useLiveMessageAccumulator } from './use-live-message-accumulator'
import { useStoryStageMessages } from './use-story-stage-messages'

describe('story message timestamps', () => {
  it('projects the persisted turn time into both conversation messages', () => {
    const timestamp = '2026-09-05T11:52:00Z'
    const { result } = renderHook(() => useStoryStageMessages({
      snapshot: {
        story_id: 'story-1', branch_id: 'main', state: {},
        turns: [{ id: 'turn-1', parent_id: null, branch_id: 'main', ts: timestamp, user: 'Open the door', narrative: 'A light shines outside.' }],
      },
      liveMessages: [], streaming: false, stageKey: 'story-1:main',
      liveTurnNavigationAnchorId: 'live', publicRuleRollVisible: false,
      optimisticInteractiveImages: {}, belongsToStage: () => true, renderKeyFor: () => undefined,
    }))

    expect(buildAgentMessageViews(result.current.agentMessages).map((view) => agentViewToRenderMessage(view))).toMatchObject([
      { role: 'user', created_at: timestamp },
      { role: 'assistant', created_at: timestamp },
    ])
  })

  it('timestamps live user and assistant messages once as text accumulates', () => {
    const { result } = renderHook(() => {
      const [messages, setMessages] = useState<AgentUIMessage[]>([])
      const accumulator = useLiveMessageAccumulator({ setMessages, publicRuleRollVisible: false })
      return { messages, accumulator }
    })
    const startedAt = Date.now()
    act(() => {
      result.current.accumulator.prepareTurn('Open the door', 'live', 'replace')
      result.current.accumulator.appendAssistant('A light', 'live')
      result.current.accumulator.flush()
    })
    const timestamps = result.current.messages.map((message) => message.metadata?.created_at)
    for (const timestamp of timestamps) {
      expect(Date.parse(timestamp || '')).toBeGreaterThanOrEqual(startedAt)
      expect(Date.parse(timestamp || '')).toBeLessThanOrEqual(Date.now())
    }
    act(() => {
      result.current.accumulator.appendAssistant(' shines outside.', 'live', { created_at: '2099-01-01T00:00:00Z' })
      result.current.accumulator.flush()
    })
    expect(result.current.messages.map((message) => message.metadata?.created_at)).toEqual(timestamps)
  })
})
