import { describe, expect, it } from 'vitest'
import { useInteractiveStore } from './interactive-store'

describe('interactive-store', () => {
  it('selects current story and resets branch state', () => {
    useInteractiveStore.setState({
      stories: [],
      tellers: [],
      branches: [],
      snapshot: null,
      currentStoryId: '',
      currentBranchId: 'main',
      submode: 'story',
    })

    useInteractiveStore.getState().setStories(
      [
        {
          id: 'st_1',
          title: '开端',
          origin: '',
          story_teller_id: 'classic',
          reply_target_chars: 1200,
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
})
