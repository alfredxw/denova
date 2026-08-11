import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { Profiler } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { InteractiveLayout } from './InteractiveLayout'
import { useInteractiveStore } from '../stores/interactive-store'
import { createInteractiveStory, deleteInteractiveStory, getInteractiveBranches, getInteractiveSnapshot, getInteractiveStories, getInteractiveTellers, getStoryDirectors, renameInteractiveBranch, selectInteractiveStory, switchInteractiveBranch, updateInteractiveStory } from '../api'
import type { Snapshot, StoryDirector, StorySummary, Teller } from '../types'
import { consumeInteractiveStoryRecovery, requestInteractiveStoryRecovery } from '@/features/mobile-workbench/task-recovery-navigation'
import { registerExecutableDraft, unregisterExecutableDraft, useExecutableDraftGuard } from '@/features/config-guard/executable-draft-guard'

const responsiveState = vi.hoisted(() => ({ mobile: false }))

vi.mock('@/hooks/useIsMobile', () => ({
  useIsMobile: () => responsiveState.mobile,
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
  renameInteractiveBranch: vi.fn(),
  getStoryDirectors: vi.fn(),
  selectInteractiveStory: vi.fn(),
  switchInteractiveBranch: vi.fn(),
  updateInteractiveStory: vi.fn(),
}))

vi.mock('./BranchTimeline', () => ({
  BranchTimeline: () => <div data-testid="branch-timeline" />,
}))

vi.mock('./StorylinesView', () => ({
  StorylinesView: (props: { currentBranchId: string; branches: unknown[]; onRenameBranch: (branchId: string, title: string) => void }) => (
    <div data-testid="storylines-view" data-branch-id={props.currentBranchId} data-branch-count={props.branches.length}>
      <button type="button" onClick={() => props.onRenameBranch('br_1', '密林小径')}>
        mock rename branch
      </button>
    </div>
  ),
}))

vi.mock('./DirectorPanel', () => ({
  DirectorPanel: () => <div data-testid="director-panel" />,
}))

vi.mock('./director-backstage/DirectorBackstage', () => ({
  DirectorBackstage: () => <div data-testid="director-workspace" />,
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
    branchId: string
    onStoryCreate: (input: { title: string; origin?: string; story_teller_id: string; story_director_id?: string; choice_count: number; reply_target_chars?: number }) => Promise<void>
    onStoryDelete: (storyIds: string[]) => Promise<void>
    onStorySelect: (storyId: string) => void
    onDirectorChange: (directorId: string) => Promise<void>
    onToggleDirectorPanel: () => void
  }) => (
    <div data-testid="story-stage-probe" data-story-id={props.storyId} data-branch-id={props.branchId}>
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
      <button type="button" onClick={props.onToggleDirectorPanel}>
        mock open director workspace
      </button>
      <div data-testid="story-list">{props.stories.map((item) => item.title).join('|')}</div>
    </div>
  ),
}))

beforeEach(() => {
  useExecutableDraftGuard.setState({ entries: {} })
  responsiveState.mobile = false
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
  vi.mocked(deleteInteractiveStory).mockReset()
  vi.mocked(getInteractiveStories).mockReset()
  vi.mocked(getInteractiveTellers).mockReset()
  vi.mocked(renameInteractiveBranch).mockReset()
  vi.mocked(getStoryDirectors).mockReset()
  vi.mocked(selectInteractiveStory).mockReset()
  vi.mocked(switchInteractiveBranch).mockReset()
  vi.mocked(getInteractiveSnapshot).mockReset()
  vi.mocked(getInteractiveBranches).mockReset()
  vi.mocked(updateInteractiveStory).mockReset()
  vi.mocked(deleteInteractiveStory).mockResolvedValue(undefined)
  vi.mocked(getInteractiveTellers).mockResolvedValue([])
  vi.mocked(renameInteractiveBranch).mockResolvedValue({ id: 'br_1', head: '', title: '密林小径', created_at: '2026-08-01T00:00:00Z', current: false })
  vi.mocked(getStoryDirectors).mockResolvedValue([])
  vi.mocked(selectInteractiveStory).mockResolvedValue(undefined)
  vi.mocked(switchInteractiveBranch).mockResolvedValue(undefined)
  vi.mocked(getInteractiveSnapshot).mockResolvedValue({ story_id: 'st_new', branch_id: 'main', turns: [], state: {} })
  vi.mocked(getInteractiveBranches).mockResolvedValue([{ id: 'main', head: '', title: '主线', created_at: '2026-07-04T00:00:00Z', current: true }])
  consumeInteractiveStoryRecovery()
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

describe('InteractiveLayout mobile workspaces', () => {
  it('renders the list-first storylines surface on mobile', async () => {
    responsiveState.mobile = true
    useInteractiveStore.setState({
      stories: [story('st_1', '故事线')],
      currentStoryId: 'st_1',
      currentBranchId: 'main',
      submode: 'timeline',
      branches: [{ id: 'main', head: '', title: '主线', created_at: '2026-08-01T00:00:00Z', current: true }],
      snapshot: { story_id: 'st_1', branch_id: 'main', turns: [], state: {} },
    })
    vi.mocked(getInteractiveStories).mockResolvedValue({
      current_story_id: 'st_1',
      stories: [story('st_1', '故事线')],
    })

    render(<InteractiveLayout workspace="/workspace" />)

    await waitFor(() => expect(screen.getByTestId('storylines-view')).toBeInTheDocument())
    expect(screen.getByTestId('storylines-view')).toHaveAttribute('data-branch-id', 'main')
    expect(screen.getByTestId('storylines-view')).toHaveAttribute('data-branch-count', '1')
    expect(screen.queryByTestId('branch-timeline')).not.toBeInTheDocument()
  })

  it('keeps the graph as the timeline surface on desktop', async () => {
    responsiveState.mobile = false
    useInteractiveStore.setState({
      stories: [story('st_1', '故事线')],
      currentStoryId: 'st_1',
      currentBranchId: 'main',
      submode: 'timeline',
      branches: [{ id: 'main', head: '', title: '主线', created_at: '2026-08-01T00:00:00Z', current: true }],
      snapshot: { story_id: 'st_1', branch_id: 'main', turns: [], state: {} },
    })
    vi.mocked(getInteractiveStories).mockResolvedValue({
      current_story_id: 'st_1',
      stories: [story('st_1', '故事线')],
    })

    render(<InteractiveLayout workspace="/workspace" />)

    await waitFor(() => expect(screen.getByTestId('branch-timeline')).toBeInTheDocument())
    expect(screen.queryByTestId('storylines-view')).not.toBeInTheDocument()
  })

  it('renames a branch from the mobile storylines surface and refreshes the branch list', async () => {
    responsiveState.mobile = true
    useInteractiveStore.setState({
      stories: [story('st_1', '故事线')],
      currentStoryId: 'st_1',
      currentBranchId: 'main',
      submode: 'timeline',
      branches: [
        { id: 'main', head: '', title: '主线', created_at: '2026-08-01T00:00:00Z', current: true },
        { id: 'br_1', head: '', title: '折返路线', created_at: '2026-08-01T00:00:00Z', current: false },
      ],
      snapshot: { story_id: 'st_1', branch_id: 'main', turns: [], state: {} },
    })
    vi.mocked(getInteractiveStories).mockResolvedValue({
      current_story_id: 'st_1',
      stories: [story('st_1', '故事线')],
    })

    render(<InteractiveLayout workspace="/workspace" />)

    await waitFor(() => expect(screen.getByTestId('storylines-view')).toBeInTheDocument())
    const callsBeforeRename = vi.mocked(getInteractiveBranches).mock.calls.length

    fireEvent.click(screen.getByRole('button', { name: 'mock rename branch' }))

    await waitFor(() => {
      expect(renameInteractiveBranch).toHaveBeenCalledWith('st_1', 'br_1', '密林小径')
      expect(vi.mocked(getInteractiveBranches).mock.calls.length).toBeGreaterThan(callsBeforeRename)
    })
  })

  it('opens the director as a full workspace without an edge-swipe drawer host', async () => {
    responsiveState.mobile = true
    vi.mocked(getInteractiveStories).mockResolvedValue({
      current_story_id: 'st_1',
      stories: [story('st_1', '故事线')],
    })

    render(<InteractiveLayout workspace="/workspace" />)
    await waitFor(() => expect(screen.getByTestId('story-stage-probe')).toHaveAttribute('data-story-id', 'st_1'))

    expect(document.querySelector('[data-nova-mobile-pane-host="true"]')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'mock open director workspace' }))

    expect(screen.getByTestId('director-workspace')).toBeInTheDocument()
    expect(screen.queryByTestId('story-stage-probe')).not.toBeInTheDocument()
  })

  it('restores the source story and branch from a queued background task', async () => {
    vi.mocked(getInteractiveStories).mockResolvedValue({
      current_story_id: 'st_1',
      stories: [story('st_1', '故事线 1'), story('st_2', '故事线 2')],
    })
    vi.mocked(getInteractiveSnapshot).mockImplementation(async (storyId, branchId) => ({
      story_id: storyId,
      branch_id: branchId || 'main',
      turns: [],
      state: {},
    }))
    requestInteractiveStoryRecovery({ storyId: 'st_2', branchId: 'night', taskId: 'task-story' })

    render(<InteractiveLayout workspace="/workspace" />)

    await waitFor(() => {
      expect(screen.getByTestId('story-stage-probe')).toHaveAttribute('data-story-id', 'st_2')
      expect(screen.getByTestId('story-stage-probe')).toHaveAttribute('data-branch-id', 'night')
    })
    expect(selectInteractiveStory).toHaveBeenCalledWith('st_2')
    expect(switchInteractiveBranch).toHaveBeenCalledWith('st_2', 'night')
  })

  it('keeps the preset surface when a queued task would leave it with a pending draft', async () => {
    const discard = vi.fn()
    registerExecutableDraft('setting-panel', { hasPending: true, discard })
    useInteractiveStore.setState({
      stories: [story('st_1', '故事线 1')],
      currentStoryId: 'st_1',
      currentBranchId: 'main',
      submode: 'teller',
    })
    vi.mocked(getInteractiveStories).mockResolvedValue({
      current_story_id: 'st_1',
      stories: [story('st_1', '故事线 1')],
    })
    vi.mocked(getInteractiveSnapshot).mockImplementation(async (storyId, branchId) => ({
      story_id: storyId,
      branch_id: branchId || 'main',
      turns: [],
      state: {},
    }))
    requestInteractiveStoryRecovery({ storyId: 'st_1', branchId: 'night', taskId: 'task-story' })

    render(<InteractiveLayout workspace="/workspace" />)

    expect(await screen.findByRole('alertdialog', { name: '放弃未保存的配置？' })).toBeInTheDocument()
    expect(screen.getByTestId('setting-panel')).toBeInTheDocument()
    expect(useInteractiveStore.getState().submode).toBe('teller')

    fireEvent.click(screen.getByRole('button', { name: '继续编辑' }))
    expect(useInteractiveStore.getState().submode).toBe('teller')

    act(() => {
      requestInteractiveStoryRecovery({ storyId: 'st_1', branchId: 'night', taskId: 'task-story' })
    })
    expect(await screen.findByRole('alertdialog', { name: '放弃未保存的配置？' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '放弃修改' }))
    await waitFor(() => expect(useInteractiveStore.getState().submode).toBe('story'))
    expect(discard).toHaveBeenCalled()
    unregisterExecutableDraft('setting-panel')
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
