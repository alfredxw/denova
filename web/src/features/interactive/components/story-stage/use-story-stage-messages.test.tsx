import { renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { Snapshot, TurnEvent } from '../../types'
import { useStoryStageMessages } from './use-story-stage-messages'

describe('useStoryStageMessages', () => {
  it('projects persisted display event IDs as stable segment identities', () => {
    const turn = {
      id: 'turn-1',
      parent_id: null,
      branch_id: 'main',
      ts: '2026-07-31T00:00:00Z',
      user: '推开石门',
      narrative: '门后亮起一盏灯。',
      run_id: 'run-game',
      display_events: [
        { id: 'thinking-segment', role: 'thinking', content: '正在判断门后的动静。', run_id: 'run-game' },
        { id: 'narrative-anchor', role: 'narrative' },
        { id: 'tool-segment', role: 'tool_call', name: 'submit_choices', status: 'success', run_id: 'run-game' },
      ],
    } as TurnEvent
    const snapshot = {
      story_id: 'story-1',
      branch_id: 'main',
      turns: [turn],
      current_turn: turn,
      state: {},
    } as Snapshot

    const { result } = renderHook(() => useStoryStageMessages({
      snapshot,
      liveMessages: [],
      streaming: false,
      stageKey: '/books/demo:story-1:main',
      liveTurnNavigationAnchorId: 'live-turn',
      publicRuleRollVisible: false,
      optimisticInteractiveImages: {},
      belongsToStage: () => true,
      renderKeyFor: () => undefined,
    }))

    expect(result.current.agentMessages.find((message) => message.id === 'thinking-segment')).toMatchObject({
      metadata: {
        run_id: 'run-game',
        display_segment_id: 'thinking-segment',
      },
      parts: [{ type: 'reasoning', text: '正在判断门后的动静。' }],
    })
  })
})
