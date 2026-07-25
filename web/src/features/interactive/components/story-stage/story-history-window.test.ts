import { describe, expect, it } from 'vitest'
import type { Snapshot, TurnEvent } from '../../types'
import {
  STORY_HISTORY_CACHE_MAX_TURNS,
  boundStoryTurns,
  createStoryHistoryWindow,
  prependStoryHistoryPage,
  reconcileStoryHistoryWindow,
} from './story-history-window'

function turn(index: number, narrative = `剧情-${index}`): TurnEvent {
  return {
    id: `turn-${index}`,
    parent_id: index > 0 ? `turn-${index - 1}` : null,
    branch_id: 'main',
    ts: '2026-07-25T00:00:00Z',
    user: `行动-${index}`,
    narrative,
  }
}

function snapshot(turns: TurnEvent[]): Snapshot {
  return { story_id: 'story-1', branch_id: 'main', turns, state: {} }
}

describe('story history display window', () => {
  it('keeps continuous canonical updates bounded to the newest 10,000 turns', () => {
    const initial = createStoryHistoryWindow('stage', snapshot(Array.from({ length: 100 }, (_, index) => turn(index))))
    const next = reconcileStoryHistoryWindow(
      initial,
      'stage',
      snapshot(Array.from({ length: STORY_HISTORY_CACHE_MAX_TURNS + 25 }, (_, index) => turn(index))),
    )

    expect(next.turns).toHaveLength(STORY_HISTORY_CACHE_MAX_TURNS)
    expect(next.turns[0].id).toBe('turn-25')
    expect(next.turns.at(-1)?.id).toBe(`turn-${STORY_HISTORY_CACHE_MAX_TURNS + 24}`)
    expect(next.followLatest).toBe(true)
    expect(next.hasMore).toBe(true)
  })

  it('switches to an older contiguous window when prepending exceeds the cache', () => {
    const current = createStoryHistoryWindow(
      'stage',
      snapshot(Array.from({ length: STORY_HISTORY_CACHE_MAX_TURNS }, (_, index) => turn(index + 100))),
    )
    const next = prependStoryHistoryPage(current, 'stage', {
      story_id: 'story-1',
      branch_id: 'main',
      turns: Array.from({ length: 100 }, (_, index) => turn(index)),
      before_cursor: 'before-older',
      has_more: true,
    })

    expect(next.followLatest).toBe(false)
    expect(next.turns).toHaveLength(STORY_HISTORY_CACHE_MAX_TURNS)
    expect(next.turns[0].id).toBe('turn-0')
    expect(next.turns.at(-1)?.id).toBe(`turn-${STORY_HISTORY_CACHE_MAX_TURNS - 1}`)
  })

  it('also bounds unusually large tool and display payloads by approximate bytes', () => {
    const bounded = boundStoryTurns([turn(1, '界'.repeat(40)), turn(2, '界'.repeat(40))], 'latest', {
      maxTurns: 10,
      maxBytes: 400,
    })

    expect(bounded.turns).toHaveLength(1)
    expect(bounded.turns[0].id).toBe('turn-2')
    expect(bounded.approximateBytes).toBeLessThanOrEqual(400)
  })
})
