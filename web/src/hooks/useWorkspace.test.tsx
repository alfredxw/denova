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
  getStyles: vi.fn(),
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
    apiMock.getStyles.mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('关闭自动刷新时不注册目录、统计和风格的后台轮询', async () => {
    const setIntervalSpy = vi.spyOn(window, 'setInterval')

    render(<WorkspaceHarness autoRefreshEnabled={false} onChange={() => {}} />)

    await waitFor(() => expect(apiMock.getWorkspaceTree).toHaveBeenCalledTimes(1))
    expect(apiMock.getWorkspaceSummary).toHaveBeenCalledTimes(1)
    expect(apiMock.getStyles).toHaveBeenCalledTimes(1)
    expect(setIntervalSpy.mock.calls.some(([, timeout]) => timeout === TREE_AUTO_REFRESH_INTERVAL_MS_FOR_TEST)).toBe(false)
  })

  it('只应用最后一次选中文件的读取结果，避免旧请求晚返回覆盖当前内容', async () => {
    const oldRead = deferred<{ path: string; content: string }>()
    const newRead = deferred<{ path: string; content: string }>()
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
      newRead.resolve({ path: 'chapters/new.md', content: '新内容' })
      await newRead.promise
    })

    await waitFor(() => expect(screen.getByTestId('workspace-state')).toHaveTextContent('chapters/new.md|新内容'))

    await act(async () => {
      oldRead.resolve({ path: 'chapters/old.md', content: '旧内容' })
      await oldRead.promise
    })

    expect(screen.getByTestId('workspace-state')).toHaveTextContent('chapters/new.md|新内容')
  })

  it('保存当前文件时携带读取到的 revision，并在保存成功后更新 revision', async () => {
    apiMock.readFile.mockResolvedValue({ path: 'chapters/ch01.md', content: '旧内容', revision: 'rev-1' })
    apiMock.saveFile.mockResolvedValueOnce({ path: 'chapters/ch01.md', message: 'ok', revision: 'rev-2' })
      .mockResolvedValueOnce({ path: 'chapters/ch01.md', message: 'ok', revision: 'rev-3' })

    let workspace: ReturnType<typeof useWorkspace> | null = null
    render(<WorkspaceHarness onChange={(value) => { workspace = value }} />)

    await waitFor(() => expect(apiMock.getCurrentWorkspace).toHaveBeenCalled())
    await act(async () => {
      await workspace?.selectFile('chapters/ch01.md')
    })

    await act(async () => {
      await workspace?.saveCurrentFile('第一次保存')
    })
    expect(apiMock.saveFile).toHaveBeenLastCalledWith('chapters/ch01.md', '第一次保存', 'rev-1')

    await act(async () => {
      await workspace?.saveCurrentFile('第二次保存')
    })
    expect(apiMock.saveFile).toHaveBeenLastCalledWith('chapters/ch01.md', '第二次保存', 'rev-2')
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
  return <div data-testid="workspace-state">{workspace.selectedFile}|{workspace.fileContent}</div>
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
