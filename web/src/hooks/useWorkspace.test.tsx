import { StrictMode, useEffect } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { WorkspaceChangeEvent } from '@/features/changes/types'
import { WorkspaceFileRevisionConflictError } from '@/lib/autosave/workspace-file-revision-conflict'
import { useWorkspace } from './useWorkspace'

const workspaceEventsMock = vi.hoisted(() => ({
  subscribeProjectFileEvents: vi.fn(),
}))

const projectFilesApiMock = vi.hoisted(() => ({
  applyProjectFileOperations: vi.fn(),
}))

const apiMock = vi.hoisted(() => {
  class MockAPIError extends Error {
    readonly status: number
    readonly code?: string

    constructor(message: string, options: { status: number; code?: string }) {
      super(message)
      this.status = options.status
      this.code = options.code
    }
  }
  return {
    APIError: MockAPIError,
    getBookshelf: vi.fn(),
    getCurrentWorkspace: vi.fn(),
    getProjectBookSummary: vi.fn(),
    getProjectBookTree: vi.fn(),
    readFile: vi.fn(),
    saveFile: vi.fn(),
  }
})

vi.mock('@/lib/api', () => ({
  APIError: apiMock.APIError,
  getBookshelf: apiMock.getBookshelf,
  getCurrentWorkspace: apiMock.getCurrentWorkspace,
  getProjectBookSummary: apiMock.getProjectBookSummary,
  getProjectBookTree: apiMock.getProjectBookTree,
  readProjectFile: async (projectId: string, path: string) => {
    const document = await apiMock.readFile(path)
    const workspaceName = String(document.workspace || '').split('/').filter(Boolean).at(-1)
    const returnedProjectId = document.project_id
      || (document.workspace === '/books/demo' ? 'project-demo' : `project-${workspaceName}`)
    const content = String(document.content ?? '')
    return {
      kind: 'text',
      mime_type: 'text/plain',
      size: new TextEncoder().encode(content).byteLength,
      editable: true,
      ...document,
      content,
      project_id: returnedProjectId || projectId,
      path: document.path || path,
    }
  },
  saveProjectFile: (projectId: string, path: string, content: string, baseRevision: string) => (
    apiMock.saveFile(path, content, baseRevision, projectId)
  ),
}))
vi.mock('@/lib/api-client/project-files', () => projectFilesApiMock)
vi.mock('@/features/workspace-events/client', () => workspaceEventsMock)

describe('useWorkspace', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.getCurrentWorkspace.mockResolvedValue({ workspace: '/books/demo', project_id: 'project-demo', has_state: true })
    apiMock.getBookshelf.mockResolvedValue({ books: [], sort_mode: 'recent' })
    apiMock.getProjectBookTree.mockResolvedValue([])
    apiMock.getProjectBookSummary.mockResolvedValue({ title: '', author: '', chapter_count: 0, total_words: 0, chapters: [] })
    projectFilesApiMock.applyProjectFileOperations.mockImplementation(async (_projectId, operations) => (
      operations.map((operation: { kind: string; path: string; to?: string; new_name?: string }) => ({
        kind: operation.kind,
        ok: true,
        path: operation.kind === 'rename'
          ? [...operation.path.split('/').slice(0, -1), operation.new_name].filter(Boolean).join('/')
          : operation.to || operation.path,
      }))
    ))
    workspaceEventsMock.subscribeProjectFileEvents.mockReturnValue(vi.fn())
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('starts the canonical workspace and bookshelf reads once under StrictMode', async () => {
    render(
      <StrictMode>
        <WorkspaceHarness autoRefreshEnabled={false} onChange={() => {}} />
      </StrictMode>,
    )

    await waitFor(() => expect(screen.getByTestId('workspace-meta')).toHaveTextContent('/books/demo'))
    expect(apiMock.getCurrentWorkspace).toHaveBeenCalledTimes(1)
    expect(apiMock.getBookshelf).toHaveBeenCalledTimes(1)
    expect(apiMock.getProjectBookTree).toHaveBeenCalledTimes(1)
    expect(apiMock.getProjectBookSummary).toHaveBeenCalledTimes(1)
  })

  it('exposes the stable project identity for project-scoped workspace modules', async () => {
    render(<WorkspaceHarness autoRefreshEnabled={false} onChange={() => {}} />)

    await waitFor(() => expect(screen.getByTestId('workspace-meta')).toHaveTextContent('|project-demo'))
  })

  it('publishes readiness only after the bookshelf and initial workspace snapshot settle', async () => {
    const bookshelf = deferred<{ books: never[]; sort_mode: 'recent' }>()
    const tree = deferred<unknown[]>()
    const summary = deferred<{ title: string; author: string; chapter_count: number; total_words: number; chapters: never[] }>()
    apiMock.getBookshelf.mockReturnValue(bookshelf.promise)
    apiMock.getProjectBookTree.mockReturnValue(tree.promise)
    apiMock.getProjectBookSummary.mockReturnValue(summary.promise)

    render(<WorkspaceHarness autoRefreshEnabled={false} onChange={() => {}} />)

    await waitFor(() => expect(screen.getByTestId('workspace-readiness')).toHaveTextContent('true|false|false'))
    await act(async () => {
      bookshelf.resolve({ books: [], sort_mode: 'recent' })
      await bookshelf.promise
    })
    expect(screen.getByTestId('workspace-readiness')).toHaveTextContent('true|true|false')

    await act(async () => {
      tree.resolve([])
      summary.resolve({ title: '', author: '', chapter_count: 0, total_words: 0, chapters: [] })
      await Promise.all([tree.promise, summary.promise])
    })
    expect(screen.getByTestId('workspace-readiness')).toHaveTextContent('true|true|true')
  })

  it('routes every workspace tree mutation through the project-scoped operations API', async () => {
    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness autoRefreshEnabled={false} onChange={(value) => { workspace = value }} />)
    await waitFor(() => expect(screen.getByTestId('workspace-meta')).toHaveTextContent('|project-demo'))

    await act(async () => {
      await workspace?.createItem('notes/new.md', 'file')
      await workspace?.renameItem('notes/new.md', 'renamed.md')
      await workspace?.copyItem('notes/renamed.md', 'notes/copy.md')
      await workspace?.moveItem('notes/copy.md', 'archive/copy.md')
      await workspace?.deleteItem('archive/copy.md')
    })

    expect(projectFilesApiMock.applyProjectFileOperations.mock.calls).toEqual([
      ['project-demo', [{ kind: 'create', path: 'notes/new.md', type: 'file', content: '' }]],
      ['project-demo', [{ kind: 'rename', path: 'notes/new.md', new_name: 'renamed.md' }]],
      ['project-demo', [{ kind: 'copy', path: 'notes/renamed.md', to: 'notes/copy.md' }]],
      ['project-demo', [{ kind: 'move', path: 'notes/copy.md', to: 'archive/copy.md' }]],
      ['project-demo', [{ kind: 'delete', path: 'archive/copy.md' }]],
    ])
  })

  it('does not overlap the initial workspace read when the window gains focus', async () => {
    const tree = deferred<unknown[]>()
    const summary = deferred<{ title: string; author: string; chapter_count: number; total_words: number; chapters: unknown[] }>()
    apiMock.getProjectBookTree.mockReturnValue(tree.promise)
    apiMock.getProjectBookSummary.mockReturnValue(summary.promise)

    render(<WorkspaceHarness onChange={() => {}} />)
    await waitFor(() => expect(apiMock.getProjectBookTree).toHaveBeenCalledTimes(1))

    act(() => { fireEvent.focus(window) })
    expect(apiMock.getProjectBookTree).toHaveBeenCalledTimes(1)
    expect(apiMock.getProjectBookSummary).toHaveBeenCalledTimes(1)

    await act(async () => {
      tree.resolve([])
      summary.resolve({ title: '', author: '', chapter_count: 0, total_words: 0, chapters: [] })
      await Promise.all([tree.promise, summary.promise])
    })
  })

  it('关闭后台刷新时窗口唤醒也不扫描目录和章节统计', async () => {
    render(<WorkspaceHarness autoRefreshEnabled={false} onChange={() => {}} />)

    await waitFor(() => expect(apiMock.getProjectBookTree).toHaveBeenCalledTimes(1))
    expect(apiMock.getProjectBookSummary).toHaveBeenCalledTimes(1)
    apiMock.getProjectBookTree.mockClear()
    apiMock.getProjectBookSummary.mockClear()

    act(() => {
      fireEvent.focus(window)
    })

    expect(apiMock.getProjectBookTree).not.toHaveBeenCalled()
    expect(apiMock.getProjectBookSummary).not.toHaveBeenCalled()
  })

  it('启用后台刷新时也不按固定周期扫描目录和章节统计', async () => {
    vi.useFakeTimers()

    render(<WorkspaceHarness onChange={() => {}} />)
    await act(async () => {
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(apiMock.getProjectBookTree).toHaveBeenCalledTimes(1)
    expect(apiMock.getProjectBookSummary).toHaveBeenCalledTimes(1)

    apiMock.getProjectBookTree.mockClear()
    apiMock.getProjectBookSummary.mockClear()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000)
    })

    expect(apiMock.getProjectBookTree).not.toHaveBeenCalled()
    expect(apiMock.getProjectBookSummary).not.toHaveBeenCalled()
  })

  it('合并自动刷新期间的重复唤醒，避免目录和统计请求重叠', async () => {
    render(<WorkspaceHarness onChange={() => {}} />)
    await waitFor(() => expect(apiMock.getProjectBookTree).toHaveBeenCalledTimes(1))

    const treeRefresh = deferred<unknown[]>()
    const summaryRefresh = deferred<{ title: string; author: string; chapter_count: number; total_words: number; chapters: unknown[] }>()
    apiMock.getProjectBookTree.mockClear()
    apiMock.getProjectBookSummary.mockClear()
    apiMock.getProjectBookTree.mockReturnValue(treeRefresh.promise)
    apiMock.getProjectBookSummary.mockReturnValue(summaryRefresh.promise)

    act(() => {
      fireEvent.focus(window)
      fireEvent.focus(window)
    })

    expect(apiMock.getProjectBookTree).toHaveBeenCalledTimes(1)
    expect(apiMock.getProjectBookSummary).toHaveBeenCalledTimes(1)

    await act(async () => {
      treeRefresh.resolve([])
      summaryRefresh.resolve({ title: '', author: '', chapter_count: 0, total_words: 0, chapters: [] })
      await Promise.all([treeRefresh.promise, summaryRefresh.promise])
    })
  })

  it('暴露书架与快捷切换器共用的排序模式', async () => {
    apiMock.getBookshelf.mockResolvedValue({ books: [], sort_mode: 'manual' })

    render(<WorkspaceHarness autoRefreshEnabled={false} onChange={() => {}} />)

    await waitFor(() => expect(screen.getByTestId('workspace-meta')).toHaveTextContent('|manual'))
  })

  it('watcher 更新只重读命中的普通文件，不扫描目录树和章节统计', async () => {
    apiMock.readFile
      .mockResolvedValueOnce({ workspace: '/books/demo', path: 'notes/reference.md', content: '初始', revision: 'rev-1' })
      .mockResolvedValueOnce({ workspace: '/books/demo', path: 'notes/reference.md', content: '外部更新', revision: 'rev-2' })

    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness onChange={(value) => { workspace = value }} />)
    await waitFor(() => expect(workspaceEventsMock.subscribeProjectFileEvents).toHaveBeenCalledTimes(1))
    await act(async () => {
      await workspace?.selectFile('notes/reference.md')
    })
    apiMock.getProjectBookTree.mockClear()
    apiMock.getProjectBookSummary.mockClear()
    apiMock.readFile.mockClear()
    apiMock.readFile.mockResolvedValue({ workspace: '/books/demo', path: 'notes/reference.md', content: '外部更新', revision: 'rev-2' })

    await act(async () => {
      await emitWorkspaceChange([{ path: 'notes/reference.md', type: 'updated' }])
    })

    await waitFor(() => expect(screen.getByTestId('workspace-state')).toHaveTextContent('notes/reference.md|外部更新|rev-2'))
    expect(apiMock.readFile).toHaveBeenCalledWith('notes/reference.md')
    expect(apiMock.getProjectBookTree).not.toHaveBeenCalled()
    expect(apiMock.getProjectBookSummary).not.toHaveBeenCalled()
  })

  it('watcher 删除保留打开文件内容并把 missing revision 交给显式保存', async () => {
    apiMock.readFile.mockResolvedValue({ workspace: '/books/demo', path: 'chapters/ch01.md', content: '保留内容', revision: 'rev-1' })
    apiMock.saveFile.mockResolvedValue({ path: 'chapters/ch01.md', message: 'ok', revision: 'rev-recreated' })

    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness onChange={(value) => { workspace = value }} />)
    await waitFor(() => expect(workspaceEventsMock.subscribeProjectFileEvents).toHaveBeenCalledTimes(1))
    await act(async () => {
      await workspace?.selectFile('chapters/ch01.md')
    })
    apiMock.getProjectBookTree.mockClear()
    apiMock.getProjectBookSummary.mockClear()
    apiMock.readFile.mockClear()

    await act(async () => {
      await emitWorkspaceChange([{ path: 'chapters/ch01.md', type: 'deleted' }])
    })

    await waitFor(() => expect(screen.getByTestId('workspace-state')).toHaveTextContent('chapters/ch01.md|保留内容|missing'))
    expect(apiMock.readFile).not.toHaveBeenCalled()
    expect(apiMock.getProjectBookTree).toHaveBeenCalledTimes(1)
    expect(apiMock.getProjectBookSummary).toHaveBeenCalledTimes(1)

    await act(async () => {
      await workspace?.saveFileDraft('chapters/ch01.md', '重新创建', 'missing')
    })
    expect(apiMock.saveFile).toHaveBeenCalledWith('chapters/ch01.md', '重新创建', 'missing', 'project-demo')
  })

  it('watcher 重连 resync 会重新读取当前文件、目录树和统计', async () => {
    apiMock.readFile
      .mockResolvedValueOnce({ workspace: '/books/demo', path: 'chapters/ch01.md', content: '初始', revision: 'rev-1' })
      .mockResolvedValueOnce({ workspace: '/books/demo', path: 'chapters/ch01.md', content: '重连后', revision: 'rev-2' })

    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness onChange={(value) => { workspace = value }} />)
    await waitFor(() => expect(workspaceEventsMock.subscribeProjectFileEvents).toHaveBeenCalledTimes(1))
    await act(async () => {
      await workspace?.selectFile('chapters/ch01.md')
    })
    apiMock.getProjectBookTree.mockClear()
    apiMock.getProjectBookSummary.mockClear()

    await act(async () => {
      await emitWorkspaceChange([], true)
    })

    await waitFor(() => expect(screen.getByTestId('workspace-state')).toHaveTextContent('chapters/ch01.md|重连后|rev-2'))
    expect(apiMock.getProjectBookTree).toHaveBeenCalledTimes(1)
    expect(apiMock.getProjectBookSummary).toHaveBeenCalledTimes(1)
  })

  it('只应用最后一次选中文件的读取结果，避免旧请求晚返回覆盖当前内容', async () => {
    const oldRead = deferred<{ workspace: string; path: string; content: string }>()
    const newRead = deferred<{ workspace: string; path: string; content: string }>()
    apiMock.readFile.mockImplementation((path: string) => {
      if (path === 'chapters/old.md') return oldRead.promise
      if (path === 'chapters/new.md') return newRead.promise
      return Promise.reject(new Error(`unexpected path: ${path}`))
    })

    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness onChange={(value) => { workspace = value }} />)

    await waitFor(() => expect(apiMock.getCurrentWorkspace).toHaveBeenCalled())
    await act(async () => {
      void workspace?.selectFile('chapters/old.md')
      void workspace?.selectFile('chapters/new.md')
    })

    await act(async () => {
      newRead.resolve({ workspace: '/books/demo', path: 'chapters/new.md', content: '新内容' })
      await newRead.promise
    })

    await waitFor(() => expect(screen.getByTestId('workspace-state')).toHaveTextContent('chapters/new.md|新内容'))

    await act(async () => {
      oldRead.resolve({ workspace: '/books/demo', path: 'chapters/old.md', content: '旧内容' })
      await oldRead.promise
    })

    expect(screen.getByTestId('workspace-state')).toHaveTextContent('chapters/new.md|新内容')
  })

  it('重复选择当前文件时复用已加载文档，不发起重复读取', async () => {
    apiMock.readFile.mockResolvedValue({
      workspace: '/books/demo',
      path: 'chapters/ch01.md',
      content: '章节正文',
      revision: 'rev-1',
    })

    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness onChange={(value) => { workspace = value }} />)
    await waitFor(() => expect(apiMock.getCurrentWorkspace).toHaveBeenCalled())

    await act(async () => {
      await workspace?.selectFile('chapters/ch01.md')
    })
    expect(apiMock.readFile).toHaveBeenCalledTimes(1)
    apiMock.readFile.mockClear()

    await act(async () => {
      await workspace?.selectFile('chapters/ch01.md')
    })

    expect(apiMock.readFile).not.toHaveBeenCalled()
    expect(screen.getByTestId('workspace-state')).toHaveTextContent('chapters/ch01.md|章节正文|rev-1')
  })

  it('reports a missing restored file so persisted tabs can discard it', async () => {
    apiMock.readFile.mockRejectedValue(new apiMock.APIError('not found', { status: 404 }))

    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness onChange={(value) => { workspace = value }} />)
    await waitFor(() => expect(apiMock.getCurrentWorkspace).toHaveBeenCalled())

    let result: 'selected' | 'missing' | 'unavailable' | undefined
    await act(async () => {
      result = await workspace?.selectFile('tmp/deleted.md')
    })

    expect(result).toBe('missing')
    expect(screen.getByTestId('workspace-state')).toHaveTextContent('||')
  })

  it('选择图像文件时不按文本读取，避免把二进制内容塞进编辑器状态', async () => {
    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness onChange={(value) => { workspace = value }} />)

    await waitFor(() => expect(apiMock.getCurrentWorkspace).toHaveBeenCalled())
    await act(async () => {
      await workspace?.selectFile('covers/cover.jpeg')
    })

    expect(apiMock.readFile).not.toHaveBeenCalled()
    expect(screen.getByTestId('workspace-state')).toHaveTextContent('covers/cover.jpeg|')
  })

  it('保存当前文件时携带读取到的 revision，并在保存成功后更新 revision', async () => {
    apiMock.readFile.mockResolvedValue({ workspace: '/books/demo', path: 'chapters/ch01.md', content: '旧内容', revision: 'rev-1' })
    apiMock.saveFile.mockResolvedValueOnce({ path: 'chapters/ch01.md', message: 'ok', revision: 'rev-2' })
      .mockResolvedValueOnce({ path: 'chapters/ch01.md', message: 'ok', revision: 'rev-3' })

    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness onChange={(value) => { workspace = value }} />)

    await waitFor(() => expect(apiMock.getCurrentWorkspace).toHaveBeenCalled())
    await act(async () => {
      await workspace?.selectFile('chapters/ch01.md')
    })

    await act(async () => {
      await workspace?.saveFileContent('chapters/ch01.md', '第一次保存')
    })
    expect(apiMock.saveFile).toHaveBeenLastCalledWith('chapters/ch01.md', '第一次保存', 'rev-1', 'project-demo')

    await act(async () => {
      await workspace?.saveFileContent('chapters/ch01.md', '第二次保存')
    })
    expect(apiMock.saveFile).toHaveBeenLastCalledWith('chapters/ch01.md', '第二次保存', 'rev-2', 'project-demo')
  })

  it('文件落盘成功后立即确认保存，不等待章节统计刷新', async () => {
    apiMock.readFile.mockResolvedValue({ workspace: '/books/demo', path: 'chapters/ch01.md', content: '旧内容', revision: 'rev-1' })
    apiMock.saveFile.mockResolvedValue({ path: 'chapters/ch01.md', message: 'ok', revision: 'rev-2' })

    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness autoRefreshEnabled={false} onChange={(value) => { workspace = value }} />)

    await waitFor(() => expect(apiMock.getProjectBookSummary).toHaveBeenCalledTimes(1))
    await act(async () => {
      await workspace?.selectFile('chapters/ch01.md')
    })

    const summaryRefresh = deferred<{ title: string; author: string; chapter_count: number; total_words: number; chapters: [] }>()
    apiMock.getProjectBookSummary.mockClear()
    apiMock.getProjectBookSummary.mockReturnValue(summaryRefresh.promise)
    let saveSettled = false
    let saveRequest!: Promise<unknown>

    act(() => {
      saveRequest = workspace!.saveFileDraft('chapters/ch01.md', '新内容', 'rev-1')
      void saveRequest.then(() => {
        saveSettled = true
      })
    })

    await waitFor(() => expect(apiMock.getProjectBookSummary).toHaveBeenCalledTimes(1))
    await act(async () => {
      await Promise.resolve()
    })
    const settledBeforeSummary = saveSettled

    await act(async () => {
      summaryRefresh.resolve({ title: '', author: '', chapter_count: 0, total_words: 0, chapters: [] })
      await saveRequest
    })

    expect(settledBeforeSummary).toBe(true)
  })

  it('连续保存时合并后台章节统计刷新，避免整本作品并行扫描', async () => {
    apiMock.readFile.mockResolvedValue({ workspace: '/books/demo', path: 'chapters/ch01.md', content: '旧内容', revision: 'rev-1' })
    apiMock.saveFile
      .mockResolvedValueOnce({ path: 'chapters/ch01.md', message: 'ok', revision: 'rev-2' })
      .mockResolvedValueOnce({ path: 'chapters/ch01.md', message: 'ok', revision: 'rev-3' })

    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness autoRefreshEnabled={false} onChange={(value) => { workspace = value }} />)
    await waitFor(() => expect(apiMock.getProjectBookSummary).toHaveBeenCalledTimes(1))
    await act(async () => {
      await workspace?.selectFile('chapters/ch01.md')
    })

    const firstSummaryRefresh = deferred<{ title: string; author: string; chapter_count: number; total_words: number; chapters: [] }>()
    const trailingSummaryRefresh = deferred<{ title: string; author: string; chapter_count: number; total_words: number; chapters: [] }>()
    apiMock.getProjectBookSummary.mockClear()
    apiMock.getProjectBookSummary
      .mockReturnValueOnce(firstSummaryRefresh.promise)
      .mockReturnValueOnce(trailingSummaryRefresh.promise)

    await act(async () => {
      await workspace?.saveFileDraft('chapters/ch01.md', '第一次保存', 'rev-1')
      await workspace?.saveFileDraft('chapters/ch01.md', '第二次保存', 'rev-2')
    })
    const callsWhileFirstRefreshPending = apiMock.getProjectBookSummary.mock.calls.length

    await act(async () => {
      firstSummaryRefresh.resolve({ title: '', author: '', chapter_count: 0, total_words: 1, chapters: [] })
      await firstSummaryRefresh.promise
    })
    await waitFor(() => expect(apiMock.getProjectBookSummary).toHaveBeenCalledTimes(2))
    await act(async () => {
      trailingSummaryRefresh.resolve({ title: '', author: '', chapter_count: 0, total_words: 2, chapters: [] })
      await trailingSummaryRefresh.promise
    })

    expect(callsWhileFirstRefreshPending).toBe(1)
  })

  it('文件切换期间的迟到保存不会污染新文件的 revision', async () => {
    const firstSave = deferred<{ path: string; message: string; revision: string }>()
    apiMock.readFile.mockImplementation((path: string) => Promise.resolve(
      path === 'setting/outline.md'
        ? { workspace: '/books/demo', path, content: '大纲', revision: 'outline-rev-1' }
        : { workspace: '/books/demo', path, content: '进度', revision: 'progress-rev-1' },
    ))
    apiMock.saveFile.mockImplementation((path: string) => (
      path === 'setting/outline.md'
        ? firstSave.promise
        : Promise.resolve({ path, message: 'ok', revision: 'progress-rev-2' })
    ))

    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness onChange={(value) => { workspace = value }} />)

    await waitFor(() => expect(apiMock.getCurrentWorkspace).toHaveBeenCalled())
    await act(async () => {
      await workspace?.selectFile('setting/outline.md')
    })
    let outlineSave: Promise<boolean> | undefined
    act(() => {
      outlineSave = workspace?.saveFileContent('setting/outline.md', '大纲修改后')
    })
    await act(async () => {
      await workspace?.selectFile('setting/progress.md')
    })
    await act(async () => {
      firstSave.resolve({ path: 'setting/outline.md', message: 'ok', revision: 'outline-rev-2' })
      await outlineSave
    })
    await act(async () => {
      await workspace?.saveFileContent('setting/progress.md', '进度修改后')
    })

    expect(apiMock.saveFile).toHaveBeenLastCalledWith('setting/progress.md', '进度修改后', 'progress-rev-1', 'project-demo')
  })

  it('Agent 连续刷新同一文件时只应用最新一次读取', async () => {
    const olderRefresh = deferred<{ workspace: string; path: string; content: string; revision: string }>()
    const newerRefresh = deferred<{ workspace: string; path: string; content: string; revision: string }>()
    apiMock.readFile
      .mockResolvedValueOnce({ workspace: '/books/demo', path: 'chapters/ch01.md', content: '初始', revision: 'rev-1' })
      .mockImplementationOnce(() => olderRefresh.promise)
      .mockImplementationOnce(() => newerRefresh.promise)

    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness onChange={(value) => { workspace = value }} />)
    await waitFor(() => expect(screen.getByTestId('workspace-meta')).toHaveTextContent('/books/demo'))
    await act(async () => {
      await workspace?.selectFile('chapters/ch01.md')
    })

    let olderRequest: Promise<void> | undefined
    let newerRequest: Promise<void> | undefined
    act(() => {
      olderRequest = workspace?.refreshAfterAgentFileChange('chapters/ch01.md')
      newerRequest = workspace?.refreshAfterAgentFileChange('chapters/ch01.md')
    })
    await waitFor(() => expect(apiMock.readFile).toHaveBeenCalledTimes(3))

    await act(async () => {
      newerRefresh.resolve({ workspace: '/books/demo', path: 'chapters/ch01.md', content: '最新内容', revision: 'rev-3' })
      await newerRequest
    })
    expect(screen.getByTestId('workspace-state')).toHaveTextContent('chapters/ch01.md|最新内容')

    await act(async () => {
      olderRefresh.resolve({ workspace: '/books/demo', path: 'chapters/ch01.md', content: '迟到旧内容', revision: 'rev-2' })
      await olderRequest
    })
    expect(screen.getByTestId('workspace-state')).toHaveTextContent('chapters/ch01.md|最新内容')
  })

  it('skips the legacy directory tree for content-only Agent invalidations', async () => {
    apiMock.readFile.mockResolvedValue({
      workspace: '/books/demo',
      path: 'chapters/ch01.md',
      content: 'Agent content',
      revision: 'rev-2',
    })
    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness onChange={(value) => { workspace = value }} />)
    await waitFor(() => expect(screen.getByTestId('workspace-meta')).toHaveTextContent('/books/demo'))
    await act(async () => {
      await workspace?.selectFile('chapters/ch01.md')
    })
    const treeRequests = apiMock.getProjectBookTree.mock.calls.length
    const summaryRequests = apiMock.getProjectBookSummary.mock.calls.length

    await act(async () => {
      await workspace?.refreshAfterAgentFileChange('chapters/ch01.md', 'content')
    })

    expect(apiMock.getProjectBookTree).toHaveBeenCalledTimes(treeRequests)
    expect(apiMock.getProjectBookSummary).toHaveBeenCalledTimes(summaryRequests + 1)
    expect(screen.getByTestId('workspace-state')).toHaveTextContent('chapters/ch01.md|Agent content')
  })

  it('文件刷新先观察到新 revision 时忽略迟到的保存响应', async () => {
    const firstSave = deferred<{ path: string; message: string; revision: string }>()
    apiMock.readFile
      .mockResolvedValueOnce({ workspace: '/books/demo', path: 'chapters/ch01.md', content: '初始', revision: 'rev-1' })
      .mockResolvedValueOnce({ workspace: '/books/demo', path: 'chapters/ch01.md', content: 'Agent 新内容', revision: 'rev-3' })
    apiMock.saveFile
      .mockImplementationOnce(() => firstSave.promise)
      .mockResolvedValueOnce({ path: 'chapters/ch01.md', message: 'ok', revision: 'rev-4' })

    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness onChange={(value) => { workspace = value }} />)
    await waitFor(() => expect(apiMock.getCurrentWorkspace).toHaveBeenCalled())
    await act(async () => {
      await workspace?.selectFile('chapters/ch01.md')
    })

    let saveRequest: Promise<boolean> | undefined
    act(() => {
      saveRequest = workspace?.saveFileContent('chapters/ch01.md', '本地保存')
    })
    await act(async () => {
      await workspace?.refreshAfterAgentFileChange('chapters/ch01.md')
    })
    expect(screen.getByTestId('workspace-state')).toHaveTextContent('Agent 新内容')

    await act(async () => {
      firstSave.resolve({ path: 'chapters/ch01.md', message: 'ok', revision: 'rev-2' })
      await saveRequest
    })
    await act(async () => {
      await workspace?.saveFileContent('chapters/ch01.md', '基于 Agent 版本继续保存')
    })

    expect(apiMock.saveFile).toHaveBeenLastCalledWith('chapters/ch01.md', '基于 Agent 版本继续保存', 'rev-3', 'project-demo')
  })

  it('编辑器草稿保存使用草稿自己的 baseline revision，不被 Agent reload 偷换', async () => {
    apiMock.readFile
      .mockResolvedValueOnce({ workspace: '/books/demo', path: 'chapters/ch01.md', content: '初始', revision: 'rev-1' })
      .mockResolvedValueOnce({ workspace: '/books/demo', path: 'chapters/ch01.md', content: 'Agent 新内容', revision: 'rev-2' })
    apiMock.saveFile.mockResolvedValue({ path: 'chapters/ch01.md', message: 'ok', revision: 'rev-3' })

    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness onChange={(value) => { workspace = value }} />)
    await waitFor(() => expect(apiMock.getCurrentWorkspace).toHaveBeenCalled())
    await act(async () => {
      await workspace?.selectFile('chapters/ch01.md')
      await workspace?.refreshAfterAgentFileChange('chapters/ch01.md')
    })

    await act(async () => {
      await workspace?.saveFileDraft('chapters/ch01.md', '基于旧草稿的本地内容', 'rev-1')
    })

    expect(apiMock.saveFile).toHaveBeenLastCalledWith(
      'chapters/ch01.md',
      '基于旧草稿的本地内容',
      'rev-1',
      'project-demo',
    )
  })

  it('revision 冲突时把重新读取的完整快照交给编辑器适配层', async () => {
    apiMock.readFile.mockReset()
    apiMock.saveFile.mockReset()
    apiMock.readFile
      .mockResolvedValueOnce({ workspace: '/books/demo', path: 'chapters/ch01.md', content: '基线', revision: 'rev-1' })
      .mockResolvedValueOnce({ workspace: '/books/demo', path: 'chapters/ch01.md', content: 'Agent 新内容', revision: 'rev-2' })
    apiMock.saveFile.mockRejectedValue(new apiMock.APIError('revision conflict', {
      status: 409,
      code: 'revision_conflict',
    }))

    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness onChange={(value) => { workspace = value }} />)
    await waitFor(() => expect(screen.getByTestId('workspace-meta')).toHaveTextContent('/books/demo'))
    await act(async () => {
      await workspace?.selectFile('chapters/ch01.md')
    })

    let caught: unknown
    await act(async () => {
      try {
        await workspace?.saveFileDraft('chapters/ch01.md', '本地内容', 'rev-1')
      } catch (error) {
        caught = error
      }
    })

    expect(caught).toBeInstanceOf(WorkspaceFileRevisionConflictError)
    expect((caught as WorkspaceFileRevisionConflictError).latest).toEqual({
      workspace: 'project-demo',
      content: 'Agent 新内容',
      revision: 'rev-2',
    })
    expect(screen.getByTestId('workspace-state')).toHaveTextContent('Agent 新内容')
  })

  it('工作区切换后目录和统计的旧响应不会落入新工作区', async () => {
    const oldTree = deferred<Array<{ name: string; type: 'file' }>>()
    const newTree = deferred<Array<{ name: string; type: 'file' }>>()
    const oldSummary = deferred<{ title: string; author: string; chapter_count: number; total_words: number; chapters: [] }>()
    const newSummary = deferred<{ title: string; author: string; chapter_count: number; total_words: number; chapters: [] }>()
    apiMock.getCurrentWorkspace.mockResolvedValue({ workspace: '/books/old', project_id: 'project-old', has_state: true })
    apiMock.getProjectBookTree.mockImplementationOnce(() => oldTree.promise).mockImplementationOnce(() => newTree.promise)
    apiMock.getProjectBookSummary.mockImplementationOnce(() => oldSummary.promise).mockImplementationOnce(() => newSummary.promise)

    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness autoRefreshEnabled={false} onChange={(value) => { workspace = value }} />)
    await waitFor(() => expect(apiMock.getProjectBookTree).toHaveBeenCalledTimes(1))

    apiMock.getCurrentWorkspace.mockResolvedValue({ workspace: '/books/new', project_id: 'project-new', has_state: true })
    await act(async () => {
      await workspace?.refreshAll()
    })
    await waitFor(() => expect(apiMock.getProjectBookTree).toHaveBeenCalledTimes(2))

    await act(async () => {
      newTree.resolve([{ name: 'new.md', type: 'file' }])
      newSummary.resolve({ title: '新作品', author: '', chapter_count: 0, total_words: 0, chapters: [] })
      await Promise.all([newTree.promise, newSummary.promise])
    })
    expect(screen.getByTestId('workspace-meta')).toHaveTextContent('/books/new|new.md|新作品')

    await act(async () => {
      oldTree.resolve([{ name: 'old.md', type: 'file' }])
      oldSummary.resolve({ title: '旧作品', author: '', chapter_count: 0, total_words: 0, chapters: [] })
      await Promise.all([oldTree.promise, oldSummary.promise])
    })
    expect(screen.getByTestId('workspace-meta')).toHaveTextContent('/books/new|new.md|新作品')
  })

  it('只应用最后一次 current workspace 请求', async () => {
    const oldWorkspace = deferred<{ workspace: string; project_id: string; has_state: boolean }>()
    const newWorkspace = deferred<{ workspace: string; project_id: string; has_state: boolean }>()
    apiMock.getCurrentWorkspace.mockImplementationOnce(() => oldWorkspace.promise).mockImplementationOnce(() => newWorkspace.promise)

    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness autoRefreshEnabled={false} onChange={(value) => { workspace = value }} />)
    await waitFor(() => expect(apiMock.getCurrentWorkspace).toHaveBeenCalledTimes(1))
    act(() => {
      void workspace?.refreshAll()
    })
    await waitFor(() => expect(apiMock.getCurrentWorkspace).toHaveBeenCalledTimes(2))

    await act(async () => {
      newWorkspace.resolve({ workspace: '/books/new', project_id: 'project-new', has_state: true })
      await newWorkspace.promise
    })
    await waitFor(() => expect(screen.getByTestId('workspace-meta')).toHaveTextContent('/books/new'))

    await act(async () => {
      oldWorkspace.resolve({ workspace: '/books/old', project_id: 'project-old', has_state: true })
      await oldWorkspace.promise
    })
    expect(screen.getByTestId('workspace-meta')).toHaveTextContent('/books/new')
  })

  it('忽略 canonical workspace 与当前工作区不匹配的文件读取', async () => {
    apiMock.readFile.mockResolvedValue({ workspace: '/books/old', path: 'chapters/ch01.md', content: '旧工作区内容', revision: 'rev-old' })
    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness onChange={(value) => { workspace = value }} />)

    await waitFor(() => expect(apiMock.getCurrentWorkspace).toHaveBeenCalled())
    await act(async () => {
      await workspace?.selectFile('chapters/ch01.md')
    })

    expect(screen.getByTestId('workspace-state')).toHaveTextContent('|')
    expect(screen.getByTestId('workspace-state')).not.toHaveTextContent('旧工作区内容')
  })
})

function WorkspaceHarness({
  autoRefreshEnabled,
  onChange,
}: {
  autoRefreshEnabled?: boolean
  onChange: (workspace: ReturnType<typeof useWorkspace>) => void
}) {
  const workspace = useWorkspace({ autoRefreshEnabled })
  useEffect(() => onChange(workspace), [onChange, workspace])
  return (
    <>
      <div data-testid="workspace-state">{workspace.selectedFile}|{workspace.fileContent}|{workspace.fileRevision}</div>
      <div data-testid="workspace-meta">{workspace.workspace}|{workspace.tree.map((node) => node.name).join(',')}|{workspace.summary?.title ?? ''}|{workspace.bookSortMode}|{workspace.projectId}</div>
      <div data-testid="workspace-readiness">{String(workspace.workspaceLoaded)}|{String(workspace.booksLoaded)}|{String(workspace.workspaceSnapshotLoaded)}</div>
    </>
  )
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

async function emitWorkspaceChange(
  changes: Array<{ path: string; type: 'added' | 'updated' | 'deleted' }>,
  resync = false,
) {
  const subscriber = workspaceEventsMock.subscribeProjectFileEvents.mock.calls.at(-1)?.[1] as
    | ((event: WorkspaceChangeEvent) => void | Promise<void>)
    | undefined
  if (!subscriber) throw new Error('workspace event subscriber was not registered')
  await subscriber({
    project_id: 'project-demo',
    workspace: '/books/demo',
    source: 'watcher',
    changes,
    paths: changes.map(change => change.path),
    resync,
  })
}
