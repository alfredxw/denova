import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ReviewThread, WorkspaceChangeGroup } from '../types'
import { ChangeReviewWorkspace, deriveFeedbackComments } from './ChangeReviewWorkspace'

const apiMocks = vi.hoisted(() => ({
  createProjectChangeComment: vi.fn(),
  deleteProjectChangeComment: vi.fn(),
  redoProjectChangeGroup: vi.fn(),
  reviewProjectChangeGroup: vi.fn(),
  undoProjectChangeGroup: vi.fn(),
  updateProjectChangeComment: vi.fn(),
}))

const queryMocks = vi.hoisted(() => ({
  invalidateProjectChangeQueries: vi.fn(),
  useProjectChangeGroup: vi.fn(),
  useProjectChangeReviewThread: vi.fn(),
}))

vi.mock('../api', () => apiMocks)
vi.mock('../use-change-review', () => queryMocks)
vi.mock('@/features/diff/CodeDiffSurface', () => ({
  CodeDiffSurface: ({ files, layout, annotationsByPath, renderAnnotation, renderHeaderMeta, renderHeaderAction, onLineSelectionEnd }: {
    files: Array<{ path: string; revision: string; after_exists?: boolean }>
    layout: string
    annotationsByPath?: Map<string, Array<{ side: string; lineNumber: number; metadata: unknown }>>
    renderAnnotation?: (annotation: { side: string; lineNumber: number; metadata: unknown }, file: { path: string; revision: string }) => React.ReactNode
    renderHeaderMeta?: (file: { path: string; revision: string; after_exists?: boolean }) => React.ReactNode
    renderHeaderAction?: (file: { path: string; revision: string; after_exists?: boolean }) => React.ReactNode
    onLineSelectionEnd?: (file: { path: string; revision: string; after_exists?: boolean }, range: { start: number; end: number; side: 'additions' }) => void
  }) => (
    <main aria-label="完整 Diff">
      {files.map((file) => (
        <section key={file.path} data-testid="review-diff-editor" data-layout={layout} data-revision={file.revision}>
          {file.path}
          {file.after_exists === false ? <span>已删除</span> : null}
          {renderHeaderMeta?.(file)}
          {renderHeaderAction?.(file)}
          {(annotationsByPath?.get(file.path) ?? []).map((annotation, index) => (
            <div key={index}>{renderAnnotation?.(annotation, file)}</div>
          ))}
          <button type="button" onClick={() => onLineSelectionEnd?.(file, { start: 1, end: 1, side: 'additions' })}>开始评论草稿</button>
        </section>
      ))}
    </main>
  ),
}))

describe('ChangeReviewWorkspace', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.localStorage.clear()
    queryMocks.invalidateProjectChangeQueries.mockResolvedValue(undefined)
    queryMocks.useProjectChangeGroup.mockReturnValue(emptyGroupQuery())
    queryMocks.useProjectChangeReviewThread.mockReturnValue({
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
    apiMocks.reviewProjectChangeGroup.mockResolvedValue({ project_id: 'project-demo', workspace: '/books/demo', affected_paths: ['chapters/ch01.md'] })
    const onWorkspaceChanged = vi.fn()
    renderWorkspace({ onWorkspaceChanged })
    await screen.findByTestId('review-diff-editor')

    fireEvent.click(screen.getByRole('button', { name: /驳回本轮|Reject run/i }))
    await waitFor(() => expect(apiMocks.reviewProjectChangeGroup).toHaveBeenCalledWith('project-demo', 'group-2', { decision: 'reject' }))
    await waitFor(() => expect(onWorkspaceChanged).toHaveBeenCalledWith(['chapters/ch01.md']))
    expect(queryMocks.invalidateProjectChangeQueries).toHaveBeenCalledWith(expect.anything(), 'project-demo')
  })

  it('exposes unresolved comments with derived path/line metadata and renders continuity conflicts explicitly', async () => {
    const thread = reviewThread()
    thread.files[0].continuity = 'conflicted'
    queryMocks.useProjectChangeReviewThread.mockReturnValue({
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

  it('does not duplicate the workbench Review tab when loading fails', () => {
    queryMocks.useProjectChangeReviewThread.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      isFetching: false,
      error: new Error('offline'),
      refetch: vi.fn(),
    })
    renderWorkspace()

    expect(screen.queryByText(/^(审阅|Review)$/i)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /关闭|Close/i })).not.toBeInTheDocument()
  })

  it('keeps standalone review closing in the existing toolbar', async () => {
    const onClose = vi.fn()
    renderWorkspace({ onClose })
    await screen.findByTestId('review-diff-editor')

    expect(screen.queryByText(/^(审阅|Review)$/i)).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /关闭|Close/i }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('disables review mutations while the Agent is still appending to the thread', async () => {
    renderWorkspace({ disabled: true })
    await screen.findByTestId('review-diff-editor')

    expect(screen.getByRole('button', { name: /接受本轮|Accept run/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /驳回本轮|Reject run/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /撤销整组|Undo group/i })).toBeDisabled()
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
    const onOpenFile = vi.fn()
    renderWorkspace({ onOpenFile })
    await screen.findByTestId('review-diff-editor')

    fireEvent.click(screen.getByRole('button', { name: '开始评论草稿' }))

    expect(scopeButton()).toBeDisabled()
    expect(screen.getByRole('button', { name: /\u5237\u65b0|Refresh/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /\u6253\u5f00\u6587\u4ef6|Open file/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /折叠全部 Diff|Collapse all diffs/i })).toBeEnabled()
    expect(screen.getByRole('button', { name: /\u64a4\u9500\u6574\u7ec4|Undo group/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /\u63a5\u53d7\u672c\u8f6e|Accept run/i })).toBeDisabled()

    fireEvent.click(screen.getByRole('button', { name: /取消|Cancel/i }))
    expect(scopeButton()).toBeEnabled()
  })

  it('marks deleted files and does not offer an open action for an absent target', async () => {
    const thread = reviewThread()
    thread.files[0] = {
      ...thread.files[0],
      after_content: '',
      revision: 'missing',
      before_exists: true,
      after_exists: false,
    }
    queryMocks.useProjectChangeReviewThread.mockReturnValue({
      data: thread,
      isLoading: false,
      isError: false,
      isFetching: false,
      refetch: vi.fn(),
    })
    renderWorkspace({ onOpenFile: vi.fn() })

    expect(await screen.findByText(/已删除|Deleted/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /打开文件|Open file/i })).not.toBeInTheDocument()
    expect(await screen.findByTestId('review-diff-editor')).toHaveTextContent('chapters/ch01.md')
  })

  it('switches the review surface to a historical Agent run from the scope menu', async () => {
    const historical = reviewGroup()
    queryMocks.useProjectChangeGroup.mockImplementation((_projectId: string, groupID: string) => (
      groupID === historical.id
        ? { ...emptyGroupQuery(), data: historical }
        : emptyGroupQuery()
    ))
    renderWorkspace()
    await screen.findByTestId('review-diff-editor')

    fireEvent.pointerDown(scopeButton(), { button: 0, ctrlKey: false })
    fireEvent.click(await screen.findByRole('menuitemcheckbox', { name: /第 1 轮修改|Agent edit 1/i }))

    await waitFor(() => expect(queryMocks.useProjectChangeGroup).toHaveBeenCalledWith('project-demo', 'group-1'))
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
    queryMocks.useProjectChangeGroup.mockImplementation((_projectId: string, groupID: string) => (
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
          projectId="project-demo"
          threadID="thread-1"
          scopeRequest={{ id: 1, threadID: 'thread-1', groupID: 'group-1' }}
        />
      </QueryClientProvider>,
    )

    await waitFor(() => expect(queryMocks.useProjectChangeGroup).toHaveBeenCalledWith('project-demo', 'group-1'))
    expect(await screen.findByTestId('review-diff-editor')).toHaveTextContent('history/round-one.md')
    expect(scopeButton()).toHaveTextContent(/第 1 轮修改|Agent edit 1/i)

    view.rerender(
      <QueryClientProvider client={queryClient}>
        <ChangeReviewWorkspace
          projectId="project-demo"
          threadID="thread-1"
          scopeRequest={{ id: 2, threadID: 'thread-1', groupID: 'group-2' }}
        />
      </QueryClientProvider>,
    )
    await waitFor(() => expect(queryMocks.useProjectChangeGroup).toHaveBeenCalledWith('project-demo', 'group-2'))
    expect(await screen.findByTestId('review-diff-editor')).toHaveTextContent('history/round-two.md')
    expect(scopeButton()).toHaveTextContent(/第 2 轮修改|Agent edit 2/i)
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
        projectId="project-demo"
        threadID="thread-1"
        {...overrides}
      />
    </QueryClientProvider>,
  )
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
