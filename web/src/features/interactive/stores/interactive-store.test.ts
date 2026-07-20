import { beforeEach, describe, expect, it } from 'vitest'
import { mergeInteractiveTurnPersistedSnapshot, useInteractiveStore } from './interactive-store'
import type { InteractiveTurnPersistedEvent, Snapshot, StorySummary, TurnEvent } from '../types'

describe('interactive-store', () => {
  beforeEach(() => {
    window.localStorage.clear()
    useInteractiveStore.setState({
      stories: [],
      tellers: [],
      branches: [],
      snapshot: null,
      currentStoryId: '',
      currentBranchId: 'main',
      submode: 'story',
      storyStageRuns: {},
    })
  })

  it('selects current story and resets branch state when nothing was remembered', () => {
    useInteractiveStore.getState().setStories(
      [
        {
          id: 'st_1',
          title: '开端',
          origin: '',
          story_teller_id: 'classic',
          story_director_id: 'default',
          choice_count: 5,
          reply_target_chars: 2000,
          opening: { mode: 'ai' },
          created_at: '',
          updated_at: '',
          branches: 1,
          events: 0,
        },
      ],
      'st_1',
    )
    useInteractiveStore.getState().setCurrentBranchId('br_1')
    useInteractiveStore.getState().setCurrentStoryId('st_2')

    expect(useInteractiveStore.getState().currentStoryId).toBe('st_2')
    expect(useInteractiveStore.getState().currentBranchId).toBe('main')
    expect(useInteractiveStore.getState().snapshot).toBeNull()
  })

  it('remembers the selected branch for each story across refreshes', () => {
    useInteractiveStore.getState().setStories(
      [
        {
          id: 'st_1',
          title: '开端',
          origin: '',
          story_teller_id: 'classic',
          story_director_id: 'default',
          choice_count: 5,
          reply_target_chars: 2000,
          opening: { mode: 'ai' },
          created_at: '',
          updated_at: '',
          branches: 2,
          events: 0,
        },
      ],
      'st_1',
    )
    useInteractiveStore.getState().setCurrentBranchId('br_1')

    useInteractiveStore.setState({
      stories: [],
      branches: [],
      snapshot: null,
      currentStoryId: '',
      currentBranchId: 'main',
    })

    useInteractiveStore.getState().setStories(
      [
        {
          id: 'st_1',
          title: '开端',
          origin: '',
          story_teller_id: 'classic',
          story_director_id: 'default',
          choice_count: 5,
          reply_target_chars: 2000,
          opening: { mode: 'ai' },
          created_at: '',
          updated_at: '',
          branches: 2,
          events: 0,
        },
      ],
      'st_1',
    )

    expect(useInteractiveStore.getState().currentBranchId).toBe('br_1')
  })

  it('remembers the selected story across refreshes', () => {
    const stories: StorySummary[] = [
      storySummary('st_1', '故事线 1'),
      storySummary('st_2', '故事线 2'),
    ]
    useInteractiveStore.getState().setStories(stories)
    useInteractiveStore.getState().setCurrentStoryId('st_2')

    useInteractiveStore.setState({
      stories: [],
      branches: [],
      snapshot: null,
      currentStoryId: '',
      currentBranchId: 'main',
    })
    useInteractiveStore.getState().setStories(stories)

    expect(useInteractiveStore.getState().currentStoryId).toBe('st_2')
  })

  it('prefers the workspace current story over another browser local selection', () => {
    const stories: StorySummary[] = [
      storySummary('st_1', '本浏览器旧选择'),
      storySummary('st_2', '工作区当前选择'),
    ]
    useInteractiveStore.getState().setCurrentStoryId('st_1')

    useInteractiveStore.setState({
      stories: [],
      branches: [],
      snapshot: null,
      currentStoryId: '',
      currentBranchId: 'main',
    })
    useInteractiveStore.getState().setStories(stories, 'st_2')

    expect(useInteractiveStore.getState().currentStoryId).toBe('st_2')
  })

  it('syncs the backend current branch into local branch memory', () => {
    useInteractiveStore.getState().setStories(
      [
        {
          id: 'st_1',
          title: '开端',
          origin: '',
          story_teller_id: 'classic',
          story_director_id: 'default',
          choice_count: 5,
          reply_target_chars: 2000,
          opening: { mode: 'ai' },
          created_at: '',
          updated_at: '',
          branches: 2,
          events: 0,
        },
      ],
      'st_1',
    )
    useInteractiveStore.getState().setBranches([
      { id: 'main', head: '', title: '主线', created_at: '', current: false },
      { id: 'br_2', head: '', title: '支线', created_at: '', current: true },
    ])

    useInteractiveStore.setState({
      stories: [],
      branches: [],
      snapshot: null,
      currentStoryId: '',
      currentBranchId: 'main',
    })
    useInteractiveStore.getState().setStories(
      [
        {
          id: 'st_1',
          title: '开端',
          origin: '',
          story_teller_id: 'classic',
          story_director_id: 'default',
          choice_count: 5,
          reply_target_chars: 2000,
          opening: { mode: 'ai' },
          created_at: '',
          updated_at: '',
          branches: 2,
          events: 0,
        },
      ],
      'st_1',
    )

    expect(useInteractiveStore.getState().currentBranchId).toBe('br_2')
  })

  it('remembers the selected top-level interactive page', () => {
    useInteractiveStore.getState().setSubmode('timeline')

    expect(useInteractiveStore.getState().submode).toBe('timeline')
    expect(window.localStorage.getItem('nova.interactive.submode.v1')).toBe('timeline')
  })

  it('merges a persisted turn by appending it to the active branch snapshot', () => {
    const current = snapshot([turn('turn-1', null, '醒来', '雾气很重。')])
    const event = persistedEvent(turn('turn-2', 'turn-1', '推门', '门外有灯。'))

    const next = mergeInteractiveTurnPersistedSnapshot(current, event)

    expect(next.turns.map((item) => item.id)).toEqual(['turn-1', 'turn-2'])
    expect(next.current_turn?.id).toBe('turn-2')
    expect(next.state).toEqual({ scene: { location: '门外' } })
    expect(next.director_plan).toBeUndefined()
    expect(next.director_plan_status?.status).toBe('running')
    expect(next.director_plan_status?.completed_docs).toBe(1)
    expect(next.graph?.branches[0].head).toBe('turn-2')
  })

  it('merges a persisted turn by replacing the same turn without duplicating it', () => {
    const current = snapshot([
      turn('turn-1', null, '醒来', '雾气很重。'),
      turn('turn-2', 'turn-1', '推门', '旧正文。'),
    ])
    const event = persistedEvent(turn('turn-2', 'turn-1', '推门', '新正文。'))

    const next = mergeInteractiveTurnPersistedSnapshot(current, event)

    expect(next.turns.map((item) => item.id)).toEqual(['turn-1', 'turn-2'])
    expect(next.turns[1].narrative).toBe('新正文。')
  })

  it('merges a regenerated turn by truncating turns after its parent', () => {
    const current = snapshot([
      turn('turn-1', null, '醒来', '雾气很重。'),
      turn('turn-old', 'turn-1', '推门', '旧分支。'),
      turn('turn-tail', 'turn-old', '继续', '旧后续。'),
    ])
    const event = persistedEvent(turn('turn-new', 'turn-1', '重新推门', '新分支。'))

    const next = mergeInteractiveTurnPersistedSnapshot(current, event)

    expect(next.turns.map((item) => item.id)).toEqual(['turn-1', 'turn-new'])
    expect(next.current_turn?.id).toBe('turn-new')
  })

  it('applies a persisted turn through the store only for the active story and branch', () => {
    useInteractiveStore.setState({
      currentStoryId: 'story-1',
      currentBranchId: 'main',
      snapshot: snapshot([turn('turn-1', null, '醒来', '雾气很重。')]),
      branches: [{ id: 'main', head: 'turn-1', created_at: '', current: true }],
    })

    const applied = useInteractiveStore.getState().applyTurnPersisted(persistedEvent(turn('turn-2', 'turn-1', '推门', '门外有灯。')))

    expect(applied?.turns.map((item) => item.id)).toEqual(['turn-1', 'turn-2'])
    expect(useInteractiveStore.getState().snapshot?.current_turn?.id).toBe('turn-2')
    expect(useInteractiveStore.getState().branches[0].head).toBe('turn-2')

    const ignored = useInteractiveStore.getState().applyTurnPersisted({
      ...persistedEvent(turn('other-turn', null, '别处', '不应合并。')),
      branch_id: 'other-branch',
    })
    expect(ignored).toBeNull()
    expect(useInteractiveStore.getState().snapshot?.current_turn?.id).toBe('turn-2')
  })
})

function storySummary(id: string, title: string): StorySummary {
  return {
    id,
    title,
    origin: '',
    story_teller_id: 'classic',
    story_director_id: 'default',
    choice_count: 5,
    reply_target_chars: 2000,
    opening: { mode: 'ai' },
    created_at: '',
    updated_at: '',
    branches: 1,
    events: 0,
  }
}

function snapshot(turns: TurnEvent[]): Snapshot {
  const last = turns[turns.length - 1]
  return {
    story_id: 'story-1',
    branch_id: 'main',
    turns,
    current_turn: last,
    state: { scene: { location: '室内' } },
    graph: {
      nodes: turns.map((item) => ({
        id: item.id,
        parent_id: typeof item.parent_id === 'string' ? item.parent_id : undefined,
        branch_id: item.branch_id,
        title: item.user,
        summary: item.narrative,
        ts: item.ts,
        current: true,
        head: item.id === last?.id,
      })),
      branches: [{ id: 'main', head: last?.id || '', created_at: '', current: true }],
    },
  }
}

function persistedEvent(turnEvent: TurnEvent): InteractiveTurnPersistedEvent {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    turn: turnEvent,
    director_plan_status: directorPlanStatus(),
    state: { scene: { location: '门外' } },
    graph: {
      nodes: [{
        id: turnEvent.id,
        parent_id: typeof turnEvent.parent_id === 'string' ? turnEvent.parent_id : undefined,
        branch_id: turnEvent.branch_id,
        title: turnEvent.user,
        summary: turnEvent.narrative,
        ts: turnEvent.ts,
        current: true,
        head: true,
      }],
      branches: [{ id: 'main', head: turnEvent.id, created_at: '', current: true }],
    },
    branches: [{ id: 'main', head: turnEvent.id, created_at: '', current: true }],
  }
}

function directorPlanStatus() {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    status: 'running',
    summary: '后台导演正在规划开局。',
    source_turn_id: 'turn-2',
    updated_at: '2026-06-28T00:00:00Z',
    planned_docs: 1,
    completed_docs: 1,
    doc_bytes: 1200,
    visible_bytes: 320,
    start_ready: false,
    blocking: true,
  }
}

function turn(id: string, parentID: string | null, user: string, narrative: string): TurnEvent {
  return {
    id,
    parent_id: parentID,
    branch_id: 'main',
    ts: `2026-06-28T00:00:0${id.length % 10}Z`,
    user,
    narrative,
  }
}
