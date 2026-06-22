import { beforeEach, describe, expect, it } from 'vitest'
import { useInteractiveStore } from './interactive-store'
import type { StorySummary } from '../types'

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
      {
        id: 'st_1',
        title: '故事线 1',
        origin: '',
        story_teller_id: 'classic',
        reply_target_chars: 2000,
        opening: { mode: 'ai' },
        created_at: '',
        updated_at: '',
        branches: 1,
        events: 0,
      },
      {
        id: 'st_2',
        title: '故事线 2',
        origin: '',
        story_teller_id: 'classic',
        reply_target_chars: 2000,
        opening: { mode: 'ai' },
        created_at: '',
        updated_at: '',
        branches: 1,
        events: 0,
      },
    ]
    useInteractiveStore.getState().setStories(stories, 'st_1')
    useInteractiveStore.getState().setCurrentStoryId('st_2')

    useInteractiveStore.setState({
      stories: [],
      branches: [],
      snapshot: null,
      currentStoryId: '',
      currentBranchId: 'main',
    })
    useInteractiveStore.getState().setStories(stories, 'st_1')

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
})
