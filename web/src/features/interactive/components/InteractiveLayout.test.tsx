import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { Profiler } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { InteractiveLayout } from './InteractiveLayout'
import { useInteractiveStore } from '../stores/interactive-store'
import { createInteractiveBranch, createInteractiveStory, deleteInteractiveStory, getInteractiveBranches, getInteractiveDirectorStatus, getInteractiveSnapshot, getInteractiveStories, getInteractiveTellers, getStoryDirectors, selectInteractiveStory, updateInteractiveStory } from '../api'
import type { Snapshot, StoryDirector, StorySummary, Teller } from '../types'

vi.mock('@/hooks/useIsMobile', () => ({
  useIsMobile: () => false,
}))

vi.mock('@/lib/api', () => ({
  readOptionalProjectFile: vi.fn().mockResolvedValue(null),
}))

vi.mock('../api', () => ({
  createInteractiveBranch: vi.fn(),
  createInteractiveStory: vi.fn(),
  deleteInteractiveBranch: vi.fn(),
  deleteInteractiveStory: vi.fn(),
  getInteractiveBranches: vi.fn(),
  getInteractiveDirectorStatus: vi.fn(),
  getInteractiveSnapshot: vi.fn(),
  getInteractiveStories: vi.fn(),
  getInteractiveTellers: vi.fn(),
  getStoryDirectors: vi.fn(),
  selectInteractiveStory: vi.fn(),
  switchInteractiveBranch: vi.fn(),
  updateInteractiveStory: vi.fn(),
}))

vi.mock('./BranchTimeline', () => ({
  BranchTimeline: () => <div data-testid="branch-timeline" />,
}))

vi.mock('./DirectorPanel', () => ({
  DirectorPanel: () => <div data-testid="director-panel" />,
}))

vi.mock('./SettingPanel', () => ({
  SettingPanel: () => <div data-testid="setting-panel" />,
}))

vi.mock('./StoryPicker', () => ({
  StoryPicker: () => <div data-testid="story-picker" />,
}))

vi.mock('./StoryStage', () => ({
  StoryStage: (props: {
    stories: StorySummary[]
    storyId: string
    onStoryCreate: (input: { title: string; origin?: string; story_teller_id: string; story_director_id?: string; choice_count: number; reply_target_chars?: number }) => Promise<void>
    onStoryDelete: (storyIds: string[]) => Promise<void>
    onStorySelect: (storyId: string) => void
    onDirectorChange: (directorId: string) => Promise<void>
    onRequestCreateBranch: (source: { turnId: string; title: string; summary?: string }) => void
  }) => (
    <div data-testid="story-stage-probe" data-story-id={props.storyId}>
      <button
        type="button"
        onClick={() => void props.onStoryCreate({
          title: '新故事线',
          origin: '',
          story_teller_id: 'classic',
          story_director_id: 'default',
          choice_count: 5,
          reply_target_chars: 2000,
        })}
      >
        mock create story
      </button>
      <button
        type="button"
        onClick={() => void props.onStoryDelete(['st_1', 'st_2'])}
      >
        mock delete stories
      </button>
      <button
        type="button"
        onClick={() => void props.onDirectorChange('director-alt')}
      >
        mock switch director
      </button>
      <button type="button" onClick={() => props.onStorySelect('st_2')}>
        mock select story
      </button>
      <button type="button" onClick={() => props.onRequestCreateBranch({ turnId: 'turn-source', title: '调查钟楼', summary: '钟楼里传来齿轮声。' })}>
        mock create branch
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
  vi.mocked(createInteractiveBranch).mockReset()
  vi.mocked(deleteInteractiveStory).mockReset()
  vi.mocked(getInteractiveStories).mockReset()
  vi.mocked(getInteractiveTellers).mockReset()
  vi.mocked(getStoryDirectors).mockReset()
  vi.mocked(selectInteractiveStory).mockReset()
  vi.mocked(getInteractiveSnapshot).mockReset()
  vi.mocked(getInteractiveBranches).mockReset()
  vi.mocked(getInteractiveDirectorStatus).mockReset()
  vi.mocked(updateInteractiveStory).mockReset()
  vi.mocked(deleteInteractiveStory).mockResolvedValue(undefined)
  vi.mocked(getInteractiveTellers).mockResolvedValue([])
  vi.mocked(getStoryDirectors).mockResolvedValue([])
  vi.mocked(selectInteractiveStory).mockResolvedValue(undefined)
  vi.mocked(getInteractiveSnapshot).mockResolvedValue({ story_id: 'st_new', branch_id: 'main', turns: [], state: {} })
  vi.mocked(getInteractiveBranches).mockResolvedValue([{ id: 'main', head: '', title: '主线', created_at: '2026-07-04T00:00:00Z', current: true }])
  vi.mocked(getInteractiveDirectorStatus).mockResolvedValue(directorStatus('ready'))
})

afterEach(() => {
  vi.useRealTimers()
})

describe('InteractiveLayout polling lifecycle', () => {
  it('does not poll a pending snapshot while the retained route is inactive', async () => {
    vi.useFakeTimers()
    const pendingSnapshot = pendingInteractiveSnapshot()
    useInteractiveStore.setState({
      stories: [story('st_new', '故事')],
      currentStoryId: 'st_new',
      currentBranchId: 'main',
      snapshot: pendingSnapshot,
    })
    vi.mocked(getInteractiveStories).mockResolvedValue({ current_story_id: 'st_new', stories: useInteractiveStore.getState().stories })
    vi.mocked(getInteractiveSnapshot).mockResolvedValue(pendingSnapshot)

    render(<InteractiveLayout workspace="/workspace" active={false} />)
    await act(async () => {
      await Promise.resolve()
      await Promise.resolve()
    })
    const callsAfterInitialLoad = vi.mocked(getInteractiveSnapshot).mock.calls.length

    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000)
    })

    expect(vi.mocked(getInteractiveSnapshot)).toHaveBeenCalledTimes(callsAfterInitialLoad)
  })

  it('waits for the previous pending snapshot poll before scheduling another', async () => {
    vi.useFakeTimers()
    const pendingSnapshot = pendingInteractiveSnapshot()
    useInteractiveStore.setState({
      stories: [story('st_new', '故事')],
      currentStoryId: 'st_new',
      currentBranchId: 'main',
      snapshot: pendingSnapshot,
    })
    vi.mocked(getInteractiveStories).mockResolvedValue({ current_story_id: 'st_new', stories: useInteractiveStore.getState().stories })
    vi.mocked(getInteractiveSnapshot).mockResolvedValue(pendingSnapshot)

    render(<InteractiveLayout workspace="/workspace" active />)
    await act(async () => {
      await Promise.resolve()
      await Promise.resolve()
    })

    const poll = deferred<typeof pendingSnapshot>()
    vi.mocked(getInteractiveSnapshot).mockClear()
    vi.mocked(getInteractiveSnapshot).mockReturnValue(poll.promise)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
      await vi.advanceTimersByTimeAsync(5000)
    })
    expect(vi.mocked(getInteractiveSnapshot)).toHaveBeenCalledTimes(1)

    await act(async () => {
      poll.resolve(pendingSnapshot)
      await poll.promise
      await Promise.resolve()
      await vi.advanceTimersByTimeAsync(1000)
    })
    expect(vi.mocked(getInteractiveSnapshot)).toHaveBeenCalledTimes(2)
  })

  it('polls Director progress through the lightweight status projection', async () => {
    vi.useFakeTimers()
    const runningSnapshot: Snapshot = {
      story_id: 'st_new',
      branch_id: 'main',
      turns: [{ id: 'turn-1', parent_id: null, branch_id: 'main', ts: '2026-07-04T00:00:00Z', user: '继续', narrative: '结果' }],
      state: {},
      director_plan_status: directorStatus('running'),
    }
    useInteractiveStore.setState({
      stories: [story('st_new', '故事')],
      currentStoryId: 'st_new',
      currentBranchId: 'main',
      snapshot: runningSnapshot,
    })
    vi.mocked(getInteractiveStories).mockResolvedValue({ current_story_id: 'st_new', stories: useInteractiveStore.getState().stories })
    vi.mocked(getInteractiveSnapshot).mockResolvedValue(runningSnapshot)
    vi.mocked(getInteractiveDirectorStatus).mockResolvedValue(directorStatus('ready'))

    render(<InteractiveLayout workspace="/workspace" active />)
    await act(async () => {
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
    vi.mocked(getInteractiveSnapshot).mockClear()
    vi.mocked(getInteractiveBranches).mockClear()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
    })

    expect(vi.mocked(getInteractiveDirectorStatus)).toHaveBeenCalledTimes(1)
    expect(vi.mocked(getInteractiveDirectorStatus)).toHaveBeenCalledWith('st_new', 'main')
    expect(vi.mocked(getInteractiveSnapshot)).not.toHaveBeenCalled()
    expect(vi.mocked(getInteractiveBranches)).not.toHaveBeenCalled()
    expect(useInteractiveStore.getState().snapshot?.director_plan_status?.status).toBe('ready')

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000)
    })
    expect(vi.mocked(getInteractiveDirectorStatus)).toHaveBeenCalledTimes(1)
  })
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

  it('updates the story director and follows the director narrative style', async () => {
    vi.mocked(getInteractiveStories).mockResolvedValue({
      current_story_id: 'st_1',
      stories: [story('st_1', '故事线')],
    })
    vi.mocked(getInteractiveTellers).mockResolvedValue([teller('classic', '经典叙事'), teller('alt-style', '强风格')])
    vi.mocked(getStoryDirectors).mockResolvedValue([storyDirector('director-alt', '强导演', 'alt-style')])
    vi.mocked(updateInteractiveStory).mockResolvedValue({ ...story('st_1', '故事线'), story_director_id: 'director-alt', story_teller_id: 'alt-style' })

    render(<InteractiveLayout workspace="/workspace" />)

    await waitFor(() => expect(screen.getByTestId('story-stage-probe')).toHaveAttribute('data-story-id', 'st_1'))

    fireEvent.click(screen.getByRole('button', { name: 'mock switch director' }))

    await waitFor(() => {
      expect(updateInteractiveStory).toHaveBeenCalledWith('st_1', {
        story_director_id: 'director-alt',
        story_teller_id: 'alt-style',
      })
    })
  })

  it('deletes multiple stories and reloads the story index only once', async () => {
    vi.mocked(getInteractiveStories)
      .mockResolvedValueOnce({
        current_story_id: 'st_1',
        stories: [story('st_1', '主线'), story('st_2', '黑暗线'), story('st_3', '光明线')],
      })
      .mockResolvedValueOnce({
        current_story_id: 'st_3',
        stories: [story('st_3', '光明线')],
      })

    render(<InteractiveLayout workspace="/workspace" />)

    await waitFor(() => expect(screen.getByTestId('story-stage-probe')).toHaveAttribute('data-story-id', 'st_1'))
    fireEvent.click(screen.getByRole('button', { name: 'mock delete stories' }))

    await waitFor(() => {
      expect(deleteInteractiveStory).toHaveBeenCalledTimes(2)
      expect(deleteInteractiveStory).toHaveBeenNthCalledWith(1, 'st_1')
      expect(deleteInteractiveStory).toHaveBeenNthCalledWith(2, 'st_2')
      expect(getInteractiveStories).toHaveBeenCalledTimes(2)
      expect(screen.getByTestId('story-list')).toHaveTextContent('光明线')
    })
  })
})

describe('InteractiveLayout story selection', () => {
  it('persists the selected story for other browsers', async () => {
    vi.mocked(getInteractiveStories).mockResolvedValue({
      current_story_id: 'st_1',
      stories: [story('st_1', '故事线 1'), story('st_2', '故事线 2')],
    })

    render(<InteractiveLayout workspace="/workspace" />)
    await waitFor(() => expect(screen.getByTestId('story-stage-probe')).toHaveAttribute('data-story-id', 'st_1'))

    fireEvent.click(screen.getByRole('button', { name: 'mock select story' }))

    await waitFor(() => {
      expect(screen.getByTestId('story-stage-probe')).toHaveAttribute('data-story-id', 'st_2')
      expect(selectInteractiveStory).toHaveBeenCalledWith('st_2')
    })
  })
})

describe('InteractiveLayout branch creation', () => {
  it('uses the shared dialog to create and switch from a story reply', async () => {
    vi.mocked(getInteractiveStories).mockResolvedValue({
      current_story_id: 'st_1',
      stories: [story('st_1', '故事线')],
    })
    vi.mocked(getInteractiveSnapshot).mockImplementation(async (storyId, branchId) => ({
      story_id: storyId,
      branch_id: branchId || 'main',
      turns: [],
      state: {},
    }))
    vi.mocked(createInteractiveBranch).mockResolvedValue({
      id: 'br-1',
      head: 'turn-source',
      from: 'main',
      from_event: 'turn-source',
      title: '基于「调查钟楼」的新剧情线',
      created_at: '2026-07-04T00:01:00Z',
      current: true,
    })

    render(<InteractiveLayout workspace="/workspace" />)
    await waitFor(() => expect(screen.getByTestId('story-stage-probe')).toHaveAttribute('data-story-id', 'st_1'))

    fireEvent.click(screen.getByRole('button', { name: 'mock create branch' }))
    expect(screen.getByRole('dialog')).toHaveTextContent('钟楼里传来齿轮声。')
    fireEvent.click(screen.getByRole('button', { name: '创建并切换' }))

    await waitFor(() => {
      expect(createInteractiveBranch).toHaveBeenCalledWith('st_1', {
        parent_event_id: 'turn-source',
        title: '基于「调查钟楼」的新剧情线',
      })
      expect(useInteractiveStore.getState().currentBranchId).toBe('br-1')
      expect(getInteractiveSnapshot).toHaveBeenCalledWith('st_1', 'br-1')
    })
  })
})

describe('InteractiveLayout store subscriptions', () => {
  it('does not rerender for live messages owned by StoryStage', async () => {
    vi.mocked(getInteractiveStories).mockResolvedValue({ current_story_id: '', stories: [] })
    let commits = 0
    render(
      <Profiler id="interactive-layout" onRender={() => { commits += 1 }}>
        <InteractiveLayout workspace="/workspace" />
      </Profiler>,
    )
    await waitFor(() => expect(getInteractiveStories).toHaveBeenCalled())
    await act(async () => undefined)
    commits = 0

    act(() => {
      useInteractiveStore.getState().setStoryStageRun('other-stage', {
        streaming: true,
        activityContent: '',
        liveMessages: [],
      })
    })

    expect(commits).toBe(0)
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

function pendingInteractiveSnapshot(): Snapshot {
  const turn = {
    id: 'turn-1',
    parent_id: null,
    branch_id: 'main',
    ts: '2026-07-04T00:00:00Z',
    user: '继续',
    narrative: '待处理',
    state_status: 'pending' as const,
  }
  return {
    story_id: 'st_new',
    branch_id: 'main',
    turns: [turn],
    state: {},
    current_turn: turn,
  }
}

function directorStatus(status: 'running' | 'ready') {
  return {
    story_id: 'st_new',
    branch_id: 'main',
    status,
    summary: status === 'running' ? '规划中' : '规划完成',
    updated_at: status === 'running' ? '2026-07-04T00:00:00Z' : '2026-07-04T00:00:01Z',
    planned_docs: 3,
    completed_docs: status === 'ready' ? 3 : 0,
    doc_bytes: 1024,
    visible_bytes: 256,
    start_ready: status === 'ready',
    blocking: false,
  }
}

function story(id: string, title: string): StorySummary {
  return {
    id,
    title,
    origin: '',
    story_teller_id: 'classic',
    story_director_id: 'default',
    choice_count: 5,
    reply_target_chars: 2000,
    opening: { mode: 'ai' },
    created_at: '2026-07-04T00:00:00Z',
    updated_at: '2026-07-04T00:00:00Z',
    branches: 1,
    events: 0,
    turn_count: 0,
  }
}

function teller(id: string, name: string): Teller {
  return {
    version: 1,
    id,
    name,
    description: '',
    context_policy: {
      creator: 'summary',
      lore: 'summary',
      runtime_state: 'full',
    },
    slots: [],
    custom: false,
  }
}

function storyDirector(id: string, name: string, narrativeStyleId: string): StoryDirector {
  return {
    version: 1,
    id,
    name,
    description: '',
    module_refs: { narrative_style_id: narrativeStyleId },
		strategy: { enabled: true },
		trpg_system: {},
		custom: false,
  }
}
