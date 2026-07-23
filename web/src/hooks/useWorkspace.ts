import { useState, useEffect, useCallback, useRef } from 'react'
import {
  copyWorkspaceItem,
  createWorkspaceItem,
  deleteWorkspaceItem,
  getBookshelf,
  getCurrentWorkspace,
  getWorkspaceSummary,
  getWorkspaceTree,
  moveWorkspaceItem,
  readFile as readWorkspaceFile,
  renameWorkspaceItem,
  saveFile,
  APIError,
} from '@/lib/api'
import type { BookRecord, BookSortMode } from '@/lib/api'
import type { WorkspaceSummary } from '@/lib/api'
import { WorkspaceFileRevisionConflictError } from '@/lib/autosave/workspace-file-revision-conflict'
import { workspaceFileKind } from '@/lib/workspace-file-kind'

export interface FileNode {
  name: string
  type: 'file' | 'dir'
  children?: FileNode[]
}

interface WorkspaceFileDocumentState {
  content: string
  revision: string
}

interface WorkspaceRefreshOptions {
  showLoading?: boolean
  clearOnError?: boolean
}

interface UseWorkspaceOptions {
  autoRefreshEnabled?: boolean
}

/** 工作区目录树 hook，负责获取目录结构、文件内容和保存 */
export function useWorkspace(options: UseWorkspaceOptions = {}) {
  const autoRefreshEnabled = options.autoRefreshEnabled ?? true
  const [tree, setTree] = useState<FileNode[]>([])
  const [loading, setLoading] = useState(true)
  const [selectedFile, setSelectedFile] = useState<string | null>(null)
  const [fileDocument, setFileDocumentState] = useState<WorkspaceFileDocumentState>({ content: '', revision: '' })
  const fileContent = fileDocument.content
  const fileRevision = fileDocument.revision
  const [workspace, setWorkspaceState] = useState<string>('')
  const [workspaceLoaded, setWorkspaceLoaded] = useState(false)
  const [summary, setSummary] = useState<WorkspaceSummary | null>(null)
  const [books, setBooks] = useState<BookRecord[]>([])
  const [bookSortMode, setBookSortMode] = useState<BookSortMode>('recent')

  // 用 ref 追踪最新 selectedFile，避免异步回调闭包捕获旧值
  const selectedFileRef = useRef<string | null>(null)
  const fileDocumentRef = useRef(fileDocument)
  const workspaceRef = useRef(workspace)
  const workspaceEpochRef = useRef(0)
  const workspaceRequestRef = useRef(0)
  const treeRequestRef = useRef(0)
  const summaryRequestRef = useRef(0)
  const booksRequestRef = useRef(0)
  const backgroundSummaryRefreshRef = useRef<Promise<void> | null>(null)
  const backgroundSummaryRefreshQueuedRef = useRef(false)
  const fileVersionsRef = useRef<Map<string, { revision: string; workspace: string; generation: number }>>(new Map())
  const fileReadGenerationsRef = useRef<Map<string, number>>(new Map())
  const selectFileRequestRef = useRef(0)
  selectedFileRef.current = selectedFile
  fileDocumentRef.current = fileDocument
  workspaceRef.current = workspace

  const setFileDocument = useCallback((next: WorkspaceFileDocumentState) => {
    fileDocumentRef.current = next
    setFileDocumentState(next)
  }, [])

  const setWorkspace = useCallback((nextWorkspace: string) => {
    if (workspaceRef.current === nextWorkspace) return
    workspaceRef.current = nextWorkspace
    workspaceEpochRef.current += 1
    treeRequestRef.current += 1
    summaryRequestRef.current += 1
    backgroundSummaryRefreshRef.current = null
    backgroundSummaryRefreshQueuedRef.current = false
    selectFileRequestRef.current += 1
    fileVersionsRef.current.clear()
    fileReadGenerationsRef.current.clear()
    setTree([])
    setSelectedFile(null)
    setFileDocument({ content: '', revision: '' })
    setSummary(null)
    setLoading(Boolean(nextWorkspace))
    setWorkspaceState(nextWorkspace)
  }, [setFileDocument])

  const recordFileVersion = useCallback((targetWorkspace: string, path: string, revision: string) => {
    const previous = fileVersionsRef.current.get(path)
    const generation = previous?.workspace === targetWorkspace ? previous.generation + 1 : 1
    const next = { revision, workspace: targetWorkspace, generation }
    fileVersionsRef.current.set(path, next)
    return next
  }, [])

  const beginFileRead = useCallback((targetWorkspace: string, path: string) => {
    const key = `${targetWorkspace}\u0000${path}`
    const generation = (fileReadGenerationsRef.current.get(key) ?? 0) + 1
    fileReadGenerationsRef.current.set(key, generation)
    return { key, generation }
  }, [])

  const isLatestFileRead = useCallback((key: string, generation: number) => (
    fileReadGenerationsRef.current.get(key) === generation
  ), [])

  const resetWorkspaceState = useCallback(() => {
    treeRequestRef.current += 1
    summaryRequestRef.current += 1
    backgroundSummaryRefreshRef.current = null
    backgroundSummaryRefreshQueuedRef.current = false
    setTree([])
    setLoading(false)
    setSelectedFile(null)
    setFileDocument({ content: '', revision: '' })
    selectFileRequestRef.current += 1
    fileVersionsRef.current.clear()
    fileReadGenerationsRef.current.clear()
    setSummary(null)
  }, [setFileDocument])

  /** 获取当前 workspace 路径 */
  const fetchWorkspace = useCallback(async () => {
    const requestID = workspaceRequestRef.current + 1
    workspaceRequestRef.current = requestID
    const requestEpoch = workspaceEpochRef.current
    try {
      const data = await getCurrentWorkspace()
      if (requestID !== workspaceRequestRef.current || requestEpoch !== workspaceEpochRef.current) return
      setWorkspace(data.workspace || '')
      setWorkspaceLoaded(true)
    } catch (e) {
      if (requestID !== workspaceRequestRef.current || requestEpoch !== workspaceEpochRef.current) return
      console.error('获取 workspace 失败', e)
      setWorkspace('')
      setWorkspaceLoaded(true)
    }
  }, [setWorkspace])

  const fetchTree = useCallback(async (options: WorkspaceRefreshOptions = {}) => {
    const showLoading = options.showLoading ?? true
    const clearOnError = options.clearOnError ?? true
    const targetWorkspace = workspace
    const requestEpoch = workspaceEpochRef.current
    const requestID = treeRequestRef.current + 1
    treeRequestRef.current = requestID
    if (!targetWorkspace) {
      setTree([])
      setLoading(false)
      return
    }
    if (showLoading) setLoading(true)
    try {
      const nextTree = (await getWorkspaceTree()) as FileNode[]
      if (requestID !== treeRequestRef.current || requestEpoch !== workspaceEpochRef.current || workspaceRef.current !== targetWorkspace) return
      setTree(nextTree)
    } catch (e) {
      if (requestID !== treeRequestRef.current || requestEpoch !== workspaceEpochRef.current || workspaceRef.current !== targetWorkspace) return
      console.error('获取目录树失败', e)
      if (clearOnError) setTree([])
    } finally {
      if (showLoading && requestID === treeRequestRef.current && requestEpoch === workspaceEpochRef.current && workspaceRef.current === targetWorkspace) {
        setLoading(false)
      }
    }
  }, [workspace])

  /** 获取当前作品章节统计 */
  const fetchSummary = useCallback(async (options: WorkspaceRefreshOptions = {}) => {
    const clearOnError = options.clearOnError ?? true
    const targetWorkspace = workspace
    const requestEpoch = workspaceEpochRef.current
    const requestID = summaryRequestRef.current + 1
    summaryRequestRef.current = requestID
    if (!targetWorkspace) {
      setSummary(null)
      return
    }
    try {
      const nextSummary = await getWorkspaceSummary()
      if (requestID !== summaryRequestRef.current || requestEpoch !== workspaceEpochRef.current || workspaceRef.current !== targetWorkspace) return
      setSummary(nextSummary)
    } catch (e) {
      if (requestID !== summaryRequestRef.current || requestEpoch !== workspaceEpochRef.current || workspaceRef.current !== targetWorkspace) return
      console.error('获取作品统计失败', e)
      if (clearOnError) setSummary(null)
    }
  }, [workspace])

  /** 合并保存触发的统计刷新，保证大作品最多只有一次全量扫描在途。 */
  const queueSummaryRefreshAfterSave = useCallback(() => {
    if (backgroundSummaryRefreshRef.current) {
      backgroundSummaryRefreshQueuedRef.current = true
      return
    }

    const run = () => {
      backgroundSummaryRefreshQueuedRef.current = false
      const request = fetchSummary({ clearOnError: false })
      backgroundSummaryRefreshRef.current = request
      void request.finally(() => {
        if (backgroundSummaryRefreshRef.current !== request) return
        backgroundSummaryRefreshRef.current = null
        if (backgroundSummaryRefreshQueuedRef.current) run()
      })
    }

    run()
  }, [fetchSummary])

  /** 获取当前 Nova 数据目录下实际存在的书籍列表 */
  const fetchBooks = useCallback(async () => {
    const requestID = booksRequestRef.current + 1
    booksRequestRef.current = requestID
    try {
      const bookshelf = await getBookshelf()
      if (requestID !== booksRequestRef.current) return
      setBooks(bookshelf.books)
      setBookSortMode(bookshelf.sort_mode)
    } catch (e) {
      if (requestID !== booksRequestRef.current) return
      console.error('获取书籍列表失败', e)
      setBooks([])
      setBookSortMode('recent')
    }
  }, [])

  useEffect(() => {
    void Promise.all([fetchWorkspace(), fetchBooks()])
  }, [fetchWorkspace, fetchBooks])

  useEffect(() => {
    if (!workspaceLoaded) return
    if (!workspace) {
      resetWorkspaceState()
      return
    }
    void Promise.all([fetchTree(), fetchSummary()])
  }, [fetchSummary, fetchTree, resetWorkspaceState, workspace, workspaceLoaded])

  // 窗口重新激活时刷新派生状态；Agent 的文件事件另有即时刷新，避免固定周期扫描整本作品。
  useEffect(() => {
    if (!autoRefreshEnabled || !workspaceLoaded || !workspace) return
    let cancelled = false
    let inFlight: Promise<void> | null = null
    const backgroundOptions = { showLoading: false, clearOnError: false }
    const refreshIfVisible = () => {
      if (cancelled || document.visibilityState !== 'visible') return Promise.resolve()
      if (inFlight) return inFlight
      inFlight = Promise.all([
        fetchTree(backgroundOptions),
        fetchSummary(backgroundOptions),
      ]).then(() => undefined).finally(() => {
        inFlight = null
      })
      return inFlight
    }
    const refreshOnWakeup = () => {
      void refreshIfVisible()
    }
    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') refreshOnWakeup()
    }

    window.addEventListener('focus', refreshOnWakeup)
    document.addEventListener('visibilitychange', handleVisibilityChange)

    return () => {
      cancelled = true
      window.removeEventListener('focus', refreshOnWakeup)
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  }, [autoRefreshEnabled, fetchTree, fetchSummary, workspace, workspaceLoaded])

  /** 选中文件并加载内容 */
  const selectFile = useCallback(async (path: string) => {
    const targetWorkspace = workspaceRef.current
    const requestID = selectFileRequestRef.current + 1
    selectFileRequestRef.current = requestID
    if (workspaceFileKind(path) === 'image') {
      setSelectedFile(path)
      setFileDocument({ content: '', revision: '' })
      return
    }
    const { key, generation } = beginFileRead(targetWorkspace, path)
    try {
      const data = await readWorkspaceFile(path)
      if (requestID !== selectFileRequestRef.current) return
      if (!isLatestFileRead(key, generation)) return
      if (workspaceRef.current !== targetWorkspace || data.workspace !== targetWorkspace) return
      // React 18 自动批量：两个 setState 合并为一次渲染，确保 MarkdownEditor 拿到一致的 (fileName, content)
      setSelectedFile(path)
      setFileDocument({ content: data.content || '', revision: data.revision || '' })
      recordFileVersion(data.workspace, path, data.revision || '')
    } catch (e) {
      console.error('读取文件失败', e)
    }
  }, [beginFileRead, isLatestFileRead, recordFileVersion, setFileDocument])

  /** 清空当前选中文件，用于关闭最后一个 tab 等场景 */
  const clearSelectedFile = useCallback(() => {
    setSelectedFile(null)
    setFileDocument({ content: '', revision: '' })
  }, [setFileDocument])

  /** 读取指定文件内容 */
  const readFile = useCallback(async (path: string) => {
    const data = await readWorkspaceFile(path)
    return data.content || ''
  }, [])

  /** Agent 写入或创建文件后，刷新目录树并同步当前打开文件内容。 */
  const refreshAfterAgentFileChange = useCallback(async (changedPath?: string) => {
    const targetWorkspace = workspace
    if (!targetWorkspace) return
    const currentFile = selectedFileRef.current
    let readRequest: { key: string; generation: number } | null = null
    if (currentFile) {
      // changedPath 可能是绝对路径，selectedFile 是相对路径。
      const isMatch = !changedPath || changedPath === currentFile || changedPath.endsWith('/' + currentFile)
      if (isMatch) readRequest = beginFileRead(targetWorkspace, currentFile)
    }
    await Promise.all([fetchTree(), fetchSummary()])
    if (!currentFile || !readRequest) return
    if (workspaceRef.current !== targetWorkspace || selectedFileRef.current !== currentFile) return
    if (workspaceFileKind(currentFile) === 'image') {
      setFileDocument({ content: '', revision: '' })
      return
    }

    try {
      const data = await readWorkspaceFile(currentFile)
      // 只有同一 workspace + path 的最后一次读取可以更新界面，避免 SSE 连续刷新时旧响应回滚新内容。
      if (!isLatestFileRead(readRequest.key, readRequest.generation)) return
      if (workspaceRef.current !== targetWorkspace || data.workspace !== targetWorkspace || selectedFileRef.current !== currentFile) return
      setFileDocument({ content: data.content || '', revision: data.revision || '' })
      recordFileVersion(data.workspace, currentFile, data.revision || '')
    } catch (e) {
      console.error('刷新当前文件失败', e)
    }
  }, [beginFileRead, fetchTree, fetchSummary, isLatestFileRead, recordFileVersion, setFileDocument, workspace])

  /** Saves an editor draft against the revision captured with that draft. Typed API errors propagate to the editor adapter. */
  const saveFileDraft = useCallback(async (path: string, content: string, draftBaseRevision: string) => {
    if (!workspace || !path) throw new Error('workspace and path are required to save an editor draft')
    const version = fileVersionsRef.current.get(path)
    const targetWorkspace = version?.workspace || workspace
    let result: Awaited<ReturnType<typeof saveFile>>
    try {
      result = await saveFile(path, content, draftBaseRevision, targetWorkspace)
    } catch (error) {
      if (error instanceof APIError && error.code === 'revision_conflict') {
        try {
          const latest = await readWorkspaceFile(path)
          if (latest.workspace !== targetWorkspace) {
            console.warn('[useWorkspace.ts] ignored revision-conflict reload from a different workspace', {
              path,
              targetWorkspace,
              loadedWorkspace: latest.workspace,
            })
            throw error
          }
          if (workspaceRef.current === targetWorkspace && latest.workspace === targetWorkspace && selectedFileRef.current === path) {
            setFileDocument({ content: latest.content || '', revision: latest.revision || '' })
            recordFileVersion(targetWorkspace, path, latest.revision || '')
          }
          throw new WorkspaceFileRevisionConflictError(error, {
            workspace: latest.workspace,
            content: latest.content || '',
            revision: latest.revision || '',
          })
        } catch (reloadError) {
          if (reloadError instanceof WorkspaceFileRevisionConflictError) throw reloadError
          console.error('[useWorkspace.ts] failed to reload editor file after revision conflict', { path, targetWorkspace, reloadError })
        }
      }
      throw error
    }
    if (result.revision && workspaceRef.current === targetWorkspace) {
      const currentVersion = fileVersionsRef.current.get(path)
      if (currentVersion?.workspace === targetWorkspace && currentVersion.revision === draftBaseRevision) {
        recordFileVersion(targetWorkspace, path, result.revision)
      }
      if (selectedFileRef.current === path && fileDocumentRef.current.revision === draftBaseRevision) {
        setFileDocument({ content, revision: result.revision })
      }
    }
    // 文件写入成功即完成保存；章节统计是派生数据，不能延长编辑器的 saving 状态。
    queueSummaryRefreshAfterSave()
    return result
  }, [queueSummaryRefreshAfterSave, recordFileVersion, setFileDocument, workspace])

  /** 保存指定文件内容；路径和 revision 绑定，避免文件切换期间的迟到响应串写。 */
  const saveFileContent = useCallback(async (path: string, content: string): Promise<boolean> => {
    if (!workspace || !path) return false
    const version = fileVersionsRef.current.get(path)
    try {
      await saveFileDraft(path, content, version?.revision || '')
      return true
    } catch (e) {
      if (e instanceof APIError) {
        console.error('保存文件失败：服务端拒绝工作区写入', {
          path,
          status: e.status,
          code: e.code,
          details: e.details,
          error: e,
        })
      } else {
        console.error('保存文件失败', e)
      }
      return false
    }
  }, [saveFileDraft, workspace])

  /** 切换 workspace 后刷新所有状态 */
  const refreshAll = useCallback(async () => {
    treeRequestRef.current += 1
    summaryRequestRef.current += 1
    setSelectedFile(null)
    setFileDocument({ content: '', revision: '' })
    selectFileRequestRef.current += 1
    fileVersionsRef.current.clear()
    fileReadGenerationsRef.current.clear()
    await Promise.all([fetchWorkspace(), fetchBooks()])
  }, [fetchWorkspace, fetchBooks, setFileDocument])

  /** 新建文件或目录 */
  const createItem = useCallback(async (path: string, type: 'file' | 'dir') => {
    await createWorkspaceItem({ path, type, content: '' })
    await Promise.all([fetchTree(), fetchSummary()])
  }, [fetchTree, fetchSummary])

  /** 删除文件或目录 */
  const deleteItem = useCallback(async (path: string) => {
    await deleteWorkspaceItem(path)
    if (selectedFile === path || selectedFile?.startsWith(`${path}/`)) {
      setSelectedFile(null)
      setFileDocument({ content: '', revision: '' })
    }
    await Promise.all([fetchTree(), fetchSummary()])
  }, [fetchTree, fetchSummary, selectedFile, setFileDocument])

  /** 重命名文件或目录 */
  const renameItem = useCallback(async (path: string, newName: string) => {
    const result = await renameWorkspaceItem({ path, new_name: newName })
    if (selectedFile === path) {
      setSelectedFile(result.path)
      await selectFile(result.path)
    } else if (selectedFile?.startsWith(`${path}/`)) {
      const nextPath = `${result.path}/${selectedFile.slice(path.length + 1)}`
      setSelectedFile(nextPath)
      await selectFile(nextPath)
    }
    await Promise.all([fetchTree(), fetchSummary()])
  }, [fetchTree, fetchSummary, selectFile, selectedFile])

  /** 复制文件或目录 */
  const copyItem = useCallback(async (from: string, to: string) => {
    await copyWorkspaceItem({ from, to })
    await Promise.all([fetchTree(), fetchSummary()])
  }, [fetchTree, fetchSummary])

  /** 移动文件或目录 */
  const moveItem = useCallback(async (from: string, to: string) => {
    const result = await moveWorkspaceItem({ from, to })
    if (selectedFile === from) {
      setSelectedFile(result.path)
      await selectFile(result.path)
    } else if (selectedFile?.startsWith(`${from}/`)) {
      const nextPath = `${result.path}/${selectedFile.slice(from.length + 1)}`
      setSelectedFile(nextPath)
      await selectFile(nextPath)
    }
    await Promise.all([fetchTree(), fetchSummary()])
  }, [fetchTree, fetchSummary, selectFile, selectedFile])

  /** 刷新目录树和章节统计 */
  const refresh = useCallback(async () => {
    if (!workspace) {
      resetWorkspaceState()
      return
    }
    await Promise.all([fetchTree(), fetchSummary()])
  }, [fetchTree, fetchSummary, resetWorkspaceState, workspace])

  return {
    tree,
    loading,
    selectedFile,
    fileContent,
    fileRevision,
    workspace,
    workspaceLoaded,
    summary,
    books,
    bookSortMode,
    selectFile,
    clearSelectedFile,
    saveFileDraft,
    saveFileContent,
    readFile,
    createItem,
    deleteItem,
    renameItem,
    copyItem,
    moveItem,
    refresh,
    refreshSummary: fetchSummary,
    refreshAfterAgentFileChange,
    refreshAll,
    refreshBooks: fetchBooks,
    setWorkspace,
  }
}
