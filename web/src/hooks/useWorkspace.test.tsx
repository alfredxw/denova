import { useEffect } from 'react'
import { act, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useWorkspace } from './useWorkspace'

const apiMock = vi.hoisted(() => ({
  copyWorkspaceItem: vi.fn(),
  createWorkspaceItem: vi.fn(),
  deleteWorkspaceItem: vi.fn(),
  getBooks: vi.fn(),
  getCurrentWorkspace: vi.fn(),
  getWorkspaceSummary: vi.fn(),
  getWorkspaceTree: vi.fn(),
  moveWorkspaceItem: vi.fn(),
  readFile: vi.fn(),
  renameWorkspaceItem: vi.fn(),
  saveFile: vi.fn(),
}))

vi.mock('@/lib/api', () => apiMock)

describe('useWorkspace', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.getCurrentWorkspace.mockResolvedValue({ workspace: '/books/demo', has_state: true })
    apiMock.getBooks.mockResolvedValue([])
    apiMock.getWorkspaceTree.mockResolvedValue([])
    apiMock.getWorkspaceSummary.mockResolvedValue({ title: '', author: '', chapter_count: 0, total_words: 0, chapters: [] })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('关闭自动刷新时不注册目录和统计的后台轮询', async () => {
    const setIntervalSpy = vi.spyOn(window, 'setInterval')

    render(<WorkspaceHarness autoRefreshEnabled={false} onChange={() => {}} />)

    await waitFor(() => expect(apiMock.getWorkspaceTree).toHaveBeenCalledTimes(1))
    expect(apiMock.getWorkspaceSummary).toHaveBeenCalledTimes(1)
    expect(setIntervalSpy.mock.calls.some(([, timeout]) => timeout === TREE_AUTO_REFRESH_INTERVAL_MS_FOR_TEST)).toBe(false)
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
    expect(apiMock.saveFile).toHaveBeenLastCalledWith('chapters/ch01.md', '第一次保存', 'rev-1', '/books/demo')

    await act(async () => {
      await workspace?.saveFileContent('chapters/ch01.md', '第二次保存')
    })
    expect(apiMock.saveFile).toHaveBeenLastCalledWith('chapters/ch01.md', '第二次保存', 'rev-2', '/books/demo')
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

    expect(apiMock.saveFile).toHaveBeenLastCalledWith('setting/progress.md', '进度修改后', 'progress-rev-1', '/books/demo')
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
    await waitFor(() => expect(apiMock.getCurrentWorkspace).toHaveBeenCalled())
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

    expect(apiMock.saveFile).toHaveBeenLastCalledWith('chapters/ch01.md', '基于 Agent 版本继续保存', 'rev-3', '/books/demo')
  })

  it('工作区切换后目录和统计的旧响应不会落入新工作区', async () => {
    const oldTree = deferred<Array<{ name: string; type: 'file' }>>()
    const newTree = deferred<Array<{ name: string; type: 'file' }>>()
    const oldSummary = deferred<{ title: string; author: string; chapter_count: number; total_words: number; chapters: [] }>()
    const newSummary = deferred<{ title: string; author: string; chapter_count: number; total_words: number; chapters: [] }>()
    apiMock.getCurrentWorkspace.mockResolvedValue({ workspace: '/books/old', has_state: true })
    apiMock.getWorkspaceTree.mockImplementationOnce(() => oldTree.promise).mockImplementationOnce(() => newTree.promise)
    apiMock.getWorkspaceSummary.mockImplementationOnce(() => oldSummary.promise).mockImplementationOnce(() => newSummary.promise)

    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness autoRefreshEnabled={false} onChange={(value) => { workspace = value }} />)
    await waitFor(() => expect(apiMock.getWorkspaceTree).toHaveBeenCalledTimes(1))

    act(() => {
      workspace?.setWorkspace('/books/new')
    })
    await waitFor(() => expect(apiMock.getWorkspaceTree).toHaveBeenCalledTimes(2))

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
    const oldWorkspace = deferred<{ workspace: string; has_state: boolean }>()
    const newWorkspace = deferred<{ workspace: string; has_state: boolean }>()
    apiMock.getCurrentWorkspace.mockImplementationOnce(() => oldWorkspace.promise).mockImplementationOnce(() => newWorkspace.promise)

    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness autoRefreshEnabled={false} onChange={(value) => { workspace = value }} />)
    await waitFor(() => expect(apiMock.getCurrentWorkspace).toHaveBeenCalledTimes(1))
    act(() => {
      void workspace?.refreshAll()
    })
    await waitFor(() => expect(apiMock.getCurrentWorkspace).toHaveBeenCalledTimes(2))

    await act(async () => {
      newWorkspace.resolve({ workspace: '/books/new', has_state: true })
      await newWorkspace.promise
    })
    await waitFor(() => expect(screen.getByTestId('workspace-meta')).toHaveTextContent('/books/new'))

    await act(async () => {
      oldWorkspace.resolve({ workspace: '/books/old', has_state: true })
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
      <div data-testid="workspace-state">{workspace.selectedFile}|{workspace.fileContent}</div>
      <div data-testid="workspace-meta">{workspace.workspace}|{workspace.tree.map((node) => node.name).join(',')}|{workspace.summary?.title ?? ''}</div>
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

const TREE_AUTO_REFRESH_INTERVAL_MS_FOR_TEST = 3000
