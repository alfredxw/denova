import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ReviewThread, WorkspaceChangeGroup } from '../types'
import { ChangeReviewWorkspace, deriveFeedbackComments } from './ChangeReviewWorkspace'

const apiMocks = vi.hoisted(() => ({
  createWorkspaceChangeComment: vi.fn(),
  deleteWorkspaceChangeComment: vi.fn(),
  redoWorkspaceChangeGroup: vi.fn(),
  resolveWorkspaceChangeComment: vi.fn(),
  reviewWorkspaceChangeGroup: vi.fn(),
  undoWorkspaceChangeGroup: vi.fn(),
  updateWorkspaceChangeComment: vi.fn(),
}))

const queryMocks = vi.hoisted(() => ({
  invalidateWorkspaceChangeQueries: vi.fn(),
  useWorkspaceChangeGroup: vi.fn(),
  useWorkspaceChangeReviewThread: vi.fn(),
}))

vi.mock('../api', () => apiMocks)
vi.mock('../use-change-review', () => queryMocks)
vi.mock('./ReviewDiffEditor', () => ({
  ReviewDiffEditor: ({ file, layout, onDraftChange, onHeightChange }: { file: { path: string; revision: string }; layout: string; onDraftChange?: (hasDraft: boolean) => void; onHeightChange?: (height: number) => void }) => (
    <div data-testid="review-diff-editor" data-layout={layout} data-revision={file.revision}>
      {file.path}
      <button type="button" onClick={() => onDraftChange?.(true)}>开始评论草稿</button>
      <button type="button" onClick={() => onDraftChange?.(false)}>取消评论草稿</button>
      <button type="button" onClick={() => onHeightChange?.(999)}>更新高度 {file.path}</button>
    </div>
  ),
}))

describe('ChangeReviewWorkspace', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.localStorage.clear()
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, value: vi.fn() })
    queryMocks.invalidateWorkspaceChangeQueries.mockResolvedValue(undefined)
    queryMocks.useWorkspaceChangeGroup.mockReturnValue(emptyGroupQuery())
    queryMocks.useWorkspaceChangeReviewThread.mockReturnValue({
      data: reviewThread(),
      isLoading: false,
      isError: false,
      isFetching: false,
      refetch: vi.fn().mockResolvedValue(undefined),
    })
  })

  it('defaults invalid preferences to unified, persists split, and restores it on the next mount', async () => {
    window.localStorage.setItem('nova:change-review-layout', 'invalid')
    const first = renderWorkspace()
    await screen.findByTestId('review-diff-editor')
    expect(layoutButton('unified')).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByTestId('review-diff-editor')).toHaveAttribute('data-layout', 'unified')

    fireEvent.click(layoutButton('split'))
    await waitFor(() => expect(window.localStorage.getItem('nova:change-review-layout')).toBe('split'))
    expect(screen.getByTestId('review-diff-editor')).toHaveAttribute('data-layout', 'split')
    first.unmount()

    renderWorkspace()
    await screen.findByTestId('review-diff-editor')
    expect(layoutButton('split')).toHaveAttribute('aria-pressed', 'true')
  })

  it('reviews exactly the selected group and refreshes receipt paths only for byte-changing decisions', async () => {
    apiMocks.reviewWorkspaceChangeGroup.mockResolvedValue({ workspace: '/books/demo', affected_paths: ['chapters/ch01.md'] })
    const onWorkspaceChanged = vi.fn()
    renderWorkspace({ onWorkspaceChanged })
    await screen.findByTestId('review-diff-editor')

    fireEvent.click(screen.getByRole('button', { name: /驳回本轮|Reject run/i }))
    await waitFor(() => expect(apiMocks.reviewWorkspaceChangeGroup).toHaveBeenCalledWith('/books/demo', 'group-2', { decision: 'reject' }))
    await waitFor(() => expect(onWorkspaceChanged).toHaveBeenCalledWith(['chapters/ch01.md']))
    expect(queryMocks.invalidateWorkspaceChangeQueries).toHaveBeenCalledWith(expect.anything(), '/books/demo')
  })

  it('exposes unresolved comments with derived path/line metadata and renders continuity conflicts explicitly', async () => {
    const thread = reviewThread()
    thread.files[0].continuity = 'conflicted'
    queryMocks.useWorkspaceChangeReviewThread.mockReturnValue({
      data: thread,
      isLoading: false,
      isError: false,
      isFetching: false,
      refetch: vi.fn(),
    })
    const onFeedbackCommentsChange = vi.fn()
    const view = renderWorkspace({ onFeedbackCommentsChange })

    expect(await screen.findByRole('status')).toHaveTextContent(/工作区内容已变化|Workspace content changed/i)
    await waitFor(() => expect(onFeedbackCommentsChange).toHaveBeenCalledWith('thread-1', [
      expect.objectContaining({ id: 'comment-1', review_path: 'chapters/ch01.md', review_line: 2 }),
    ]))
    view.unmount()
    expect(onFeedbackCommentsChange).not.toHaveBeenCalledWith('thread-1', [])
  })

  it('keeps the temporary Review tab closable when loading fails', () => {
    const onClose = vi.fn()
    queryMocks.useWorkspaceChangeReviewThread.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      isFetching: false,
      error: new Error('offline'),
      refetch: vi.fn(),
    })
    renderWorkspace({ onClose })

    fireEvent.click(screen.getByRole('button', { name: /关闭|Close/i }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('disables review mutations while the Agent is still appending to the thread', async () => {
    renderWorkspace({ disabled: true })
    await screen.findByTestId('review-diff-editor')

    expect(screen.getByRole('button', { name: /接受本轮|Accept run/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /驳回本轮|Reject run/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /撤销整组|Undo group/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /关闭|Close/i })).toBeEnabled()
  })

  it('renders every file in one review scroll while file navigation leaves the selected history scope unchanged', async () => {
    const thread = reviewThread()
    thread.groups[0].paths = ['chapters/a.md']
    thread.groups[1].paths = ['chapters/b.md']
    thread.files = [
      { ...thread.files[0], path: 'chapters/a.md', latest_group_id: 'group-1', latest_change_set_id: 'set-1', group_ids: ['group-1'], change_set_ids: ['set-1'] },
      { ...thread.files[0], path: 'chapters/b.md', base_group_id: 'group-2', base_change_set_id: 'set-2', latest_group_id: 'group-2', latest_change_set_id: 'set-2', group_ids: ['group-2'], change_set_ids: ['set-2'] },
    ]
    queryMocks.useWorkspaceChangeReviewThread.mockReturnValue({
      data: thread,
      isLoading: false,
      isError: false,
      isFetching: false,
      refetch: vi.fn(),
    })
    renderWorkspace()

    const scopeSelector = await screen.findByRole('button', { name: /全部审阅变更|All review changes/i })
    expect(screen.getByRole('main', { name: /完整 Diff|Full Diff/i })).toHaveClass('nova-review-scrollbar')
    expect(document.querySelector('[data-review-scroll-root] [data-slot="scroll-area-scrollbar"]')).toBeInTheDocument()
    const editors = await screen.findAllByTestId('review-diff-editor')
    expect(editors).toHaveLength(2)
    expect(editors[0]).toHaveTextContent('chapters/a.md')
    expect(editors[1]).toHaveTextContent('chapters/b.md')
    await flushAnimationFrames()
    expect(HTMLElement.prototype.scrollIntoView).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('option', { name: /chapters\/b\.md/ }))
    expect(scopeSelector).toHaveTextContent(/全部审阅变更|All review changes/i)
    await waitFor(() => expect(HTMLElement.prototype.scrollIntoView).toHaveBeenCalledTimes(1))

    fireEvent.click(screen.getByRole('button', { name: '更新高度 chapters/a.md' }))
    await flushAnimationFrames()
    expect(HTMLElement.prototype.scrollIntoView).toHaveBeenCalledTimes(1)

    const reviewScroll = screen.getByRole('main', { name: /完整 Diff|Full Diff/i })
    const scrollTo = vi.fn()
    Object.defineProperty(reviewScroll, 'scrollTo', { configurable: true, value: scrollTo })
    fireEvent.keyDown(reviewScroll, { key: 'End' })
    expect(scrollTo).not.toHaveBeenCalled()

    fireEvent.scroll(reviewScroll)
    await flushAnimationFrames()
    expect(HTMLElement.prototype.scrollIntoView).toHaveBeenCalledTimes(1)
  })

  it('collapses files together or individually and lets the right navigator be hidden and restored', async () => {
    const thread = reviewThread()
    thread.files.push({
      ...thread.files[0],
      path: 'setting/progress.md',
      latest_change_set_id: 'set-3',
      change_set_ids: ['set-3'],
      pending_edit_ids: ['edit-3'],
    })
    queryMocks.useWorkspaceChangeReviewThread.mockReturnValue({
      data: thread,
      isLoading: false,
      isError: false,
      isFetching: false,
      refetch: vi.fn(),
    })
    renderWorkspace()
    await screen.findAllByTestId('review-diff-editor')

    fireEvent.click(screen.getByRole('button', { name: /折叠全部 Diff|Collapse all diffs/i }))
    for (const content of document.querySelectorAll('[data-review-file-content]')) {
      expect(content).toHaveAttribute('hidden')
    }
    fireEvent.click(screen.getByRole('button', { name: /展开全部 Diff|Expand all diffs/i }))
    for (const content of document.querySelectorAll('[data-review-file-content]')) {
      expect(content).not.toHaveAttribute('hidden')
    }

    fireEvent.click(screen.getByRole('button', { name: /折叠 chapters\/ch01\.md|Collapse chapters\/ch01\.md/i }))
    expect(document.querySelector('[data-review-file-content="chapters/ch01.md"]')).toHaveAttribute('hidden')

    const navigator = document.querySelector<HTMLElement>('[data-review-file-navigator]')
    expect(navigator).not.toBeNull()
    fireEvent.click(within(navigator as HTMLElement).getByRole('button', { name: /收起文件导航|Hide file navigator/i }))
    expect(document.querySelector('[data-review-file-navigator]')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: /展开文件导航|Show file navigator/i }))
    expect(document.querySelector('[data-review-file-navigator]')).not.toBeNull()
  })

  it('keeps an Agent toggle in the Review toolbar after the Agent panel is closed', async () => {
    const onToggleAgent = vi.fn()
    renderWorkspace({ agentVisible: false, onToggleAgent })
    await screen.findByTestId('review-diff-editor')

    const toggle = screen.getByRole('button', { name: /显示创作 Agent|Show Writing Agent/i })
    expect(toggle).toHaveAttribute('aria-pressed', 'false')
    fireEvent.click(toggle)
    expect(onToggleAgent).toHaveBeenCalledTimes(1)
  })

  it('locks snapshot-changing actions while an inline comment draft is open', async () => {
    const onClose = vi.fn()
    const onOpenFile = vi.fn()
    renderWorkspace({ onClose, onOpenFile })
    await screen.findByTestId('review-diff-editor')

    fireEvent.click(screen.getByRole('button', { name: '开始评论草稿' }))

    expect(scopeButton()).toBeDisabled()
    expect(screen.getByRole('button', { name: /\u5237\u65b0|Refresh/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /\u5173\u95ed|Close/i })).toBeEnabled()
    expect(screen.getByRole('button', { name: /\u6253\u5f00\u6587\u4ef6|Open file/i })).toBeDisabled()
    expect(screen.getByRole('option', { name: /chapters\/ch01\.md/ })).toBeEnabled()
    expect(screen.getByRole('button', { name: /折叠全部 Diff|Collapse all diffs/i })).toBeEnabled()
    expect(screen.getByRole('button', { name: /\u64a4\u9500\u6574\u7ec4|Undo group/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /\u63a5\u53d7\u672c\u8f6e|Accept run/i })).toBeDisabled()

    fireEvent.click(screen.getByRole('button', { name: /折叠 chapters\/ch01\.md|Collapse chapters\/ch01\.md/i }))
    expect(document.querySelector('[data-review-file-content="chapters/ch01.md"]')).toHaveAttribute('hidden')
    expect(screen.getByTestId('review-diff-editor')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /展开 chapters\/ch01\.md|Expand chapters\/ch01\.md/i }))

    fireEvent.click(screen.getByRole('button', { name: '取消评论草稿' }))
    expect(scopeButton()).toBeEnabled()
    expect(screen.getByRole('button', { name: /\u5173\u95ed|Close/i })).toBeEnabled()
  })

  it('switches the review surface to a historical Agent run from the scope menu', async () => {
    const historical = reviewGroup()
    queryMocks.useWorkspaceChangeGroup.mockImplementation((_workspace: string, groupID: string) => (
      groupID === historical.id
        ? { ...emptyGroupQuery(), data: historical }
        : emptyGroupQuery()
    ))
    renderWorkspace()
    await screen.findByTestId('review-diff-editor')

    fireEvent.pointerDown(scopeButton(), { button: 0, ctrlKey: false })
    fireEvent.click(await screen.findByRole('menuitemcheckbox', { name: /第 1 轮修改|Agent edit 1/i }))

    await waitFor(() => expect(queryMocks.useWorkspaceChangeGroup).toHaveBeenCalledWith('/books/demo', 'group-1'))
    expect(await screen.findByTestId('review-diff-editor')).toHaveTextContent('history/round-one.md')
    expect(scopeButton()).toHaveTextContent(/第 1 轮修改|Agent edit 1/i)
  })

  it('opens directly on the Agent change group requested by its summary card', async () => {
    const historical = reviewGroup()
    const secondHistorical: WorkspaceChangeGroup = {
      ...reviewGroup(),
      id: 'group-2',
      review_status: 'pending',
      change_sets: reviewGroup().change_sets.map((change) => ({
        ...change,
        id: 'history-set-2',
        group_id: 'group-2',
        path: 'history/round-two.md',
      })),
    }
    queryMocks.useWorkspaceChangeGroup.mockImplementation((_workspace: string, groupID: string) => (
      groupID === historical.id
        ? { ...emptyGroupQuery(), data: historical }
        : groupID === secondHistorical.id
          ? { ...emptyGroupQuery(), data: secondHistorical }
        : emptyGroupQuery()
    ))
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    const view = render(
      <QueryClientProvider client={queryClient}>
        <ChangeReviewWorkspace
          workspace="/books/demo"
          threadID="thread-1"
          scopeRequest={{ id: 1, threadID: 'thread-1', groupID: 'group-1' }}
          onClose={vi.fn()}
        />
      </QueryClientProvider>,
    )

    await waitFor(() => expect(queryMocks.useWorkspaceChangeGroup).toHaveBeenCalledWith('/books/demo', 'group-1'))
    expect(await screen.findByTestId('review-diff-editor')).toHaveTextContent('history/round-one.md')
    expect(scopeButton()).toHaveTextContent(/第 1 轮修改|Agent edit 1/i)

    view.rerender(
      <QueryClientProvider client={queryClient}>
        <ChangeReviewWorkspace
          workspace="/books/demo"
          threadID="thread-1"
          scopeRequest={{ id: 2, threadID: 'thread-1', groupID: 'group-2' }}
          onClose={vi.fn()}
        />
      </QueryClientProvider>,
    )
    await waitFor(() => expect(queryMocks.useWorkspaceChangeGroup).toHaveBeenCalledWith('/books/demo', 'group-2'))
    expect(await screen.findByTestId('review-diff-editor')).toHaveTextContent('history/round-two.md')
    expect(scopeButton()).toHaveTextContent(/第 2 轮修改|Agent edit 2/i)
  })

  it('freezes the displayed snapshot while a comment draft is open and adopts refreshes after cancel', async () => {
    let currentThread = reviewThread()
    queryMocks.useWorkspaceChangeReviewThread.mockImplementation(() => ({
      data: currentThread,
      isLoading: false,
      isError: false,
      isFetching: false,
      refetch: vi.fn().mockResolvedValue(undefined),
    }))
    renderWorkspace()
    expect(await screen.findByTestId('review-diff-editor')).toHaveAttribute('data-revision', 'after-revision')

    fireEvent.click(screen.getByRole('button', { name: '开始评论草稿' }))
    currentThread = {
      ...currentThread,
      files: currentThread.files.map((file) => ({ ...file, revision: 'refreshed-revision', after_content: `${file.after_content}刷新内容\n` })),
    }
    fireEvent.click(layoutButton('split'))
    expect(screen.getByTestId('review-diff-editor')).toHaveAttribute('data-revision', 'after-revision')

    fireEvent.click(screen.getByRole('button', { name: '取消评论草稿' }))
    await waitFor(() => expect(screen.getByTestId('review-diff-editor')).toHaveAttribute('data-revision', 'refreshed-revision'))
  })
})

describe('deriveFeedbackComments', () => {
  it('reanchors one unique stale quote without mutating the ledger comment', () => {
    const thread = reviewThread()
    const source = thread.comments[0]
    source.anchor = { ...source.anchor, revision: 'stale', start: 0, quote: '调整😀' }

    const [feedback] = deriveFeedbackComments(thread)
    expect(feedback).toMatchObject({ review_path: 'chapters/ch01.md', review_line: 2 })
    expect(source.review_path).toBeUndefined()
    expect(source.review_line).toBeUndefined()
  })

  it('omits a stale line number when the quote is missing or ambiguous', () => {
    const missing = reviewThread()
    missing.comments[0].anchor = { ...missing.comments[0].anchor, revision: 'stale', quote: '找不到', start: 999 }
    expect(deriveFeedbackComments(missing)[0]).toMatchObject({ review_path: 'chapters/ch01.md', review_line: undefined })

    const ambiguous = reviewThread()
    ambiguous.files[0].after_content = '调整😀\n调整😀\n'
    ambiguous.comments[0].anchor = { ...ambiguous.comments[0].anchor, revision: 'stale', quote: '调整😀', start: 999 }
    expect(deriveFeedbackComments(ambiguous)[0]).toMatchObject({ review_path: 'chapters/ch01.md', review_line: undefined })
  })
})

function layoutButton(layout: 'unified' | 'split'): HTMLButtonElement {
  const button = document.querySelector<HTMLButtonElement>(`[data-review-layout="${layout}"]`)
  if (!button) throw new Error(`missing ${layout} layout button`)
  return button
}

function scopeButton(): HTMLButtonElement {
  const button = screen.getByRole('button', { name: /全部审阅变更|All review changes|第 \d+ 轮修改|Agent edit \d+/i })
  return button as HTMLButtonElement
}

function emptyGroupQuery() {
  return {
    data: undefined,
    isLoading: false,
    isError: false,
    isFetching: false,
    refetch: vi.fn().mockResolvedValue(undefined),
  }
}

function renderWorkspace(overrides: Partial<React.ComponentProps<typeof ChangeReviewWorkspace>> = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <ChangeReviewWorkspace
        workspace="/books/demo"
        threadID="thread-1"
        onClose={vi.fn()}
        {...overrides}
      />
    </QueryClientProvider>,
  )
}

async function flushAnimationFrames() {
  await new Promise<void>((resolve) => window.requestAnimationFrame(() => resolve()))
  await new Promise<void>((resolve) => window.requestAnimationFrame(() => resolve()))
}

function reviewThread(): ReviewThread {
  return {
    id: 'thread-1',
    latest_group_id: 'group-2',
    groups: [
      {
        id: 'group-1',
        review_thread_id: 'thread-1',
        created_at: '2026-07-16T00:00:00Z',
        review_status: 'accepted',
        apply_state: 'applied',
        pending_edit_count: 0,
        can_undo: true,
        can_redo: false,
        paths: ['chapters/ch01.md'],
      },
      {
        id: 'group-2',
        review_thread_id: 'thread-1',
        created_at: '2026-07-16T00:01:00Z',
        review_status: 'pending',
        apply_state: 'applied',
        pending_edit_count: 1,
        can_undo: true,
        can_redo: false,
        paths: ['chapters/ch01.md'],
      },
    ],
    comments: [{
      id: 'comment-1',
      group_id: 'group-2',
      change_set_id: 'set-2',
      body: '这里需要更具体',
      anchor: {
        kind: 'text-range',
        side: 'after',
        encoding: 'utf8-bytes-v1',
        revision: 'after-revision',
        start: 10,
        end: 20,
        quote: '调整😀',
      },
    }],
    files: [{
      path: 'chapters/ch01.md',
      before_content: '第一行\n旧句\n',
      after_content: '第一行\n调整😀\n',
      base_revision: 'before-revision',
      revision: 'after-revision',
      base_group_id: 'group-1',
      base_change_set_id: 'set-1',
      latest_group_id: 'group-2',
      latest_change_set_id: 'set-2',
      group_ids: ['group-1', 'group-2'],
      change_set_ids: ['set-1', 'set-2'],
      pending_edit_ids: ['edit-2'],
      review_status: 'pending',
      apply_state: 'applied',
      continuity: 'continuous',
      additions: 1,
      deletions: 1,
    }],
  }
}

function reviewGroup(): WorkspaceChangeGroup {
  return {
    id: 'group-1',
    review_thread_id: 'thread-1',
    created_at: '2026-07-16T00:00:00Z',
    review_status: 'accepted',
    apply_state: 'applied',
    change_sets: [{
      id: 'history-set-1',
      sequence: 1,
      group_id: 'group-1',
      path: 'history/round-one.md',
      base_revision: 'history-before',
      revision: 'history-after',
      before_content: '旧历史\n',
      after_content: '新历史\n',
      review_status: 'accepted',
      apply_state: 'applied',
      created_at: '2026-07-16T00:00:00Z',
    }],
    comments: [],
  }
}
