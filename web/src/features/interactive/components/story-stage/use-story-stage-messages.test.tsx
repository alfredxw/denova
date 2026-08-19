import { renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { Snapshot, TurnEvent } from '../../types'
import { useStoryStageMessages } from './use-story-stage-messages'

describe('useStoryStageMessages', () => {
  it('keeps autonomous model-only turn input out of the player timeline', () => {
    const turn = {
      id: 'turn-goal-next', parent_id: null, branch_id: 'main', ts: '2026-08-20T00:00:00Z',
      user: '完成剩余目标并验证', user_context_only: true, narrative: '剧情继续推进。',
    } as TurnEvent
    const snapshot = {
      story_id: 'story-1', branch_id: 'main', turns: [turn], current_turn: turn, state: {},
    } as Snapshot

    const { result } = renderHook(() => useStoryStageMessages({
      snapshot, liveMessages: [], streaming: false, stageKey: '/books/demo:story-1:main',
      liveTurnNavigationAnchorId: 'live-turn', publicRuleRollVisible: false,
      optimisticInteractiveImages: {}, belongsToStage: () => true, renderKeyFor: () => undefined,
    }))

    expect(result.current.agentMessages.some((message) => message.role === 'user')).toBe(false)
    expect(result.current.agentMessages.find((message) => message.id === 'turn-goal-next-assistant')).toBeTruthy()
  })

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

  it('replays a persisted result-only interactive-media contract as turn media', () => {
    const turn = {
      id: 'turn-media', parent_id: null, branch_id: 'main', ts: '2026-08-14T00:00:00Z',
      user: '继续', narrative: '雾中出现一道身影。',
      display_events: [{
        id: 'media-tool', role: 'tool_call', name: 'renamed_media_generator', status: 'success',
        result: JSON.stringify({
          schema: 'interactive_image.v1', story_id: 'story-1', branch_id: 'main', turn_id: 'turn-media',
          image_path: 'assets/interactive/images/scene.png', meta_path: 'assets/interactive/images/scene.json',
        }),
        tool_presentation: { call: 'image', result: 'interactive_media' },
      }],
    } as TurnEvent
    const snapshot = {
      story_id: 'story-1', branch_id: 'main', turns: [turn], current_turn: turn, state: {},
    } as Snapshot

    const { result } = renderHook(() => useStoryStageMessages({
      snapshot, liveMessages: [], streaming: false, stageKey: '/books/demo:story-1:main',
      liveTurnNavigationAnchorId: 'live-turn', publicRuleRollVisible: false,
      optimisticInteractiveImages: {}, belongsToStage: () => true, renderKeyFor: () => undefined,
    }))

    expect(result.current.agentMessages.some((message) => message.id === 'media-tool')).toBe(false)
    const assistant = result.current.agentMessages.find((message) => message.id === 'turn-media-assistant')
    expect(assistant?.parts).toEqual(expect.arrayContaining([
      expect.objectContaining({
        type: 'data-agent-interactive-image',
        data: expect.objectContaining({
          interactive_image: expect.objectContaining({ image_path: 'assets/interactive/images/scene.png' }),
        }),
      }),
    ]))
  })
})
