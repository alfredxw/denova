import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { InteractiveLayout } from './InteractiveLayout'
import { useInteractiveStore } from '../stores/interactive-store'
import { createInteractiveStory, getInteractiveBranches, getInteractiveSnapshot, getInteractiveStories, getInteractiveTellers, getStoryDirectors } from '../api'
import type { StorySummary } from '../types'

vi.mock('@/hooks/useIsMobile', () => ({
  useIsMobile: () => false,
}))

vi.mock('@/lib/api', () => ({
  readFile: vi.fn().mockRejectedValue(new Error('not found')),
}))

vi.mock('../api', () => ({
  createInteractiveBranch: vi.fn(),
  createInteractiveStory: vi.fn(),
  deleteInteractiveBranch: vi.fn(),
  deleteInteractiveStory: vi.fn(),
  getInteractiveBranches: vi.fn(),
  getInteractiveSnapshot: vi.fn(),
  getInteractiveStories: vi.fn(),
  getInteractiveTellers: vi.fn(),
  getStoryDirectors: vi.fn(),
  switchInteractiveBranch: vi.fn(),
  updateInteractiveStory: vi.fn(),
}))

vi.mock('./BranchTimeline', () => ({
  BranchTimeline: () => <div data-testid="branch-timeline" />,
}))

vi.mock('./MemoryPanel', () => ({
  MemoryPanel: () => <div data-testid="memory-panel" />,
}))

vi.mock('./SettingPanel', () => ({
  SettingPanel: () => <div data-testid="setting-panel" />,
}))

vi.mock('./StoryMemoryView', () => ({
  StoryMemoryView: () => <div data-testid="story-memory-view" />,
}))

vi.mock('./StoryPicker', () => ({
  StoryPicker: () => <div data-testid="story-picker" />,
}))

vi.mock('./StoryStage', () => ({
  StoryStage: (props: {
    stories: StorySummary[]
    storyId: string
    onStoryCreate: (input: { title: string; origin?: string; story_teller_id: string; story_director_id?: string; reply_target_chars?: number }) => Promise<void>
  }) => (
    <div data-testid="story-stage-probe" data-story-id={props.storyId}>
      <button
        type="button"
        onClick={() => void props.onStoryCreate({
          title: '新故事线',
          origin: '',
          story_teller_id: 'classic',
          story_director_id: 'default',
          reply_target_chars: 2000,
        })}
      >
        mock create story
      </button>
      <div data-testid="story-list">{props.stories.map((item) => item.title).join('|')}</div>
    </div>
  ),
}))

beforeEach(() => {
  window.localStorage.clear()
  useInteractiveStore.setState({
    stories: [],
    tellers: [],
    storyDirectors: [],
    branches: [],
    snapshot: null,
    storyStageRuns: {},
    currentStoryId: '',
    currentBranchId: 'main',
    submode: 'story',
  })
  vi.mocked(createInteractiveStory).mockReset()
  vi.mocked(getInteractiveStories).mockReset()
  vi.mocked(getInteractiveTellers).mockReset()
  vi.mocked(getStoryDirectors).mockReset()
  vi.mocked(getInteractiveSnapshot).mockReset()
  vi.mocked(getInteractiveBranches).mockReset()
  vi.mocked(getInteractiveTellers).mockResolvedValue([])
  vi.mocked(getStoryDirectors).mockResolvedValue([])
  vi.mocked(getInteractiveSnapshot).mockResolvedValue({ story_id: 'st_new', branch_id: 'main', turns: [], state: {} })
  vi.mocked(getInteractiveBranches).mockResolvedValue([{ id: 'main', head: '', title: '主线', created_at: '2026-07-04T00:00:00Z', current: true }])
})

describe('InteractiveLayout story creation', () => {
  it('selects and lists a newly created story even when stale story indexes resolve later', async () => {
    const initialIndex = deferred<{ current_story_id: string; stories: StorySummary[] }>()
    const afterCreateIndex = deferred<{ current_story_id: string; stories: StorySummary[] }>()
    vi.mocked(getInteractiveStories)
      .mockReturnValueOnce(initialIndex.promise)
      .mockReturnValueOnce(afterCreateIndex.promise)
    vi.mocked(createInteractiveStory).mockResolvedValue(story('st_new', '新故事线'))

    render(<InteractiveLayout workspace="/workspace" />)

    fireEvent.click(screen.getByRole('button', { name: 'mock create story' }))

    await waitFor(() => {
      expect(screen.getByTestId('story-stage-probe')).toHaveAttribute('data-story-id', 'st_new')
      expect(screen.getByTestId('story-list')).toHaveTextContent('新故事线')
    })

    await act(async () => {
      afterCreateIndex.resolve({ current_story_id: 'st_old', stories: [story('st_old', '旧故事线')] })
      await afterCreateIndex.promise
    })

    await waitFor(() => {
      expect(screen.getByTestId('story-stage-probe')).toHaveAttribute('data-story-id', 'st_new')
      expect(screen.getByTestId('story-list')).toHaveTextContent('新故事线|旧故事线')
    })

    await act(async () => {
      initialIndex.resolve({ current_story_id: 'st_old', stories: [story('st_old', '旧故事线')] })
      await initialIndex.promise
    })

    expect(screen.getByTestId('story-stage-probe')).toHaveAttribute('data-story-id', 'st_new')
    expect(screen.getByTestId('story-list')).toHaveTextContent('新故事线|旧故事线')
  })
})

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error?: unknown) => void
  const promise = new Promise<T>((innerResolve, innerReject) => {
    resolve = innerResolve
    reject = innerReject
  })
  return { promise, resolve, reject }
}

function story(id: string, title: string): StorySummary {
  return {
    id,
    title,
    origin: '',
    story_teller_id: 'classic',
    story_director_id: 'default',
    reply_target_chars: 2000,
    opening: { mode: 'ai' },
    created_at: '2026-07-04T00:00:00Z',
    updated_at: '2026-07-04T00:00:00Z',
    branches: 1,
    events: 0,
  }
}
