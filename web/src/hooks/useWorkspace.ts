import { useState, useEffect, useCallback, useRef } from 'react'
import {
  getBookshelf,
  getCurrentWorkspace,
  getProjectBookSummary,
  getProjectBookTree,
  readProjectFile,
  saveProjectFile,
  APIError,
} from '@/lib/api'
import type { BookRecord, BookSortMode } from '@/lib/api'
import type { WorkspaceSummary } from '@/lib/api'
import type { WorkspaceChangeEvent, WorkspaceChangeImpact, WorkspaceFileChange } from '@/features/changes/types'
import { WorkspaceFileRevisionConflictError } from '@/lib/autosave/workspace-file-revision-conflict'
import { workspaceFileKind } from '@/lib/workspace-file-kind'
import { MISSING_WORKSPACE_REVISION } from '@/lib/api-client/workspace'
import {
  applyProjectFileOperations,
  type ProjectFileDocument,
  type ProjectFileOperation,
  type ProjectFileOperationResult,
} from '@/lib/api-client/project-files'
import { useProjectFileEvents } from './useProjectFileEvents'

export interface FileNode {
  name: string
  type: 'file' | 'dir'
  children?: FileNode[]
}

interface WorkspaceRefreshOptions {
  showLoading?: boolean
  clearOnError?: boolean
}

type SelectFileResult = 'selected' | 'missing' | 'unavailable'

interface UseWorkspaceOptions {
  autoRefreshEnabled?: boolean
}

/** 工作区目录树 hook，负责获取目录结构、文件内容和保存 */
export function useWorkspace(options: UseWorkspaceOptions = {}) {
  const autoRefreshEnabled = options.autoRefreshEnabled ?? true
  const [tree, setTree] = useState<FileNode[]>([])
  const [loading, setLoading] = useState(true)
  const [selectedFile, setSelectedFile] = useState<string | null>(null)
  const [fileDocument, setFileDocumentState] = useState<ProjectFileDocument | null>(null)
  const fileContent = fileDocument?.content ?? ''
  const fileRevision = fileDocument?.revision ?? ''
  const [{ workspace, projectId }, setWorkspaceIdentityState] = useState({ workspace: '', projectId: '' })
  const [workspaceLoaded, setWorkspaceLoaded] = useState(false)
  const [workspaceSnapshotLoaded, setWorkspaceSnapshotLoaded] = useState(false)
  const [summary, setSummary] = useState<WorkspaceSummary | null>(null)
  const [books, setBooks] = useState<BookRecord[]>([])
  const [booksLoaded, setBooksLoaded] = useState(false)
  const [bookSortMode, setBookSortMode] = useState<BookSortMode>('recent')

  // 用 ref 追踪最新 selectedFile，避免异步回调闭包捕获旧值
  const selectedFileRef = useRef<string | null>(null)
  const fileDocumentRef = useRef(fileDocument)
  const workspaceRef = useRef(workspace)
  const projectIdRef = useRef(projectId)
  const workspaceEpochRef = useRef(0)
  const workspaceRequestRef = useRef(0)
  const initialWorkspaceLoadStartedRef = useRef(false)
  const treeRequestRef = useRef(0)
  const summaryRequestRef = useRef(0)
  const booksRequestRef = useRef(0)
  const backgroundSummaryRefreshRef = useRef<Promise<void> | null>(null)
  const backgroundSummaryRefreshQueuedRef = useRef(false)
  const fileVersionsRef = useRef<Map<string, { revision: string; projectId: string; generation: number }>>(new Map())
  const fileReadGenerationsRef = useRef<Map<string, number>>(new Map())
  const selectFileRequestRef = useRef(0)
  const filePreviewVersionRef = useRef(0)
  selectedFileRef.current = selectedFile
  fileDocumentRef.current = fileDocument
  workspaceRef.current = workspace
  projectIdRef.current = projectId

  const setFileDocument = useCallback((next: ProjectFileDocument | null) => {
    fileDocumentRef.current = next
    setFileDocumentState(next)
  }, [])

  const setWorkspaceIdentity = useCallback((nextWorkspace: string, nextProjectId: string) => {
    const normalizedProjectId = nextWorkspace ? nextProjectId.trim() : ''
    if (workspaceRef.current === nextWorkspace && projectIdRef.current === normalizedProjectId) return
    const workspaceChanged = workspaceRef.current !== nextWorkspace
    workspaceRef.current = nextWorkspace
    projectIdRef.current = normalizedProjectId
    if (workspaceChanged) {
      workspaceEpochRef.current += 1
      treeRequestRef.current += 1
      summaryRequestRef.current += 1
      backgroundSummaryRefreshRef.current = null
      backgroundSummaryRefreshQueuedRef.current = false
      selectFileRequestRef.current += 1
      fileVersionsRef.current.clear()
      fileReadGenerationsRef.current.clear()
      filePreviewVersionRef.current = 0
      setTree([])
      setSelectedFile(null)
      setFileDocument(null)
      setSummary(null)
      setLoading(Boolean(nextWorkspace))
      setWorkspaceSnapshotLoaded(false)
    }
    setWorkspaceIdentityState({ workspace: nextWorkspace, projectId: normalizedProjectId })
  }, [setFileDocument])

  const recordFileVersion = useCallback((targetProjectId: string, path: string, revision: string) => {
    const previous = fileVersionsRef.current.get(path)
    const generation = previous?.projectId === targetProjectId ? previous.generation + 1 : 1
    const next = { revision, projectId: targetProjectId, generation }
    fileVersionsRef.current.set(path, next)
    return next
  }, [])

  const beginFileRead = useCallback((targetProjectId: string, path: string) => {
    const key = `${targetProjectId}\u0000${path}`
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
    setFileDocument(null)
    selectFileRequestRef.current += 1
    fileVersionsRef.current.clear()
    fileReadGenerationsRef.current.clear()
    filePreviewVersionRef.current = 0
    setSummary(null)
    setWorkspaceSnapshotLoaded(true)
  }, [setFileDocument])

  /** 获取当前 workspace 路径 */
  const fetchWorkspace = useCallback(async () => {
    const requestID = workspaceRequestRef.current + 1
    workspaceRequestRef.current = requestID
    const requestEpoch = workspaceEpochRef.current
    try {
      const data = await getCurrentWorkspace()
      if (requestID !== workspaceRequestRef.current || requestEpoch !== workspaceEpochRef.current) return
      setWorkspaceIdentity(data.workspace || '', data.project_id || '')
      setWorkspaceLoaded(true)
    } catch (e) {
      if (requestID !== workspaceRequestRef.current || requestEpoch !== workspaceEpochRef.current) return
      console.error('[hooks/useWorkspace.ts] failed to load the active workspace', e)
      setWorkspaceIdentity('', '')
      setWorkspaceLoaded(true)
    }
  }, [setWorkspaceIdentity])

  const fetchBookSnapshot = useCallback(async (
    options: WorkspaceRefreshOptions = {},
    projection: 'tree' | 'summary' | 'all' = 'all',
  ) => {
    const showLoading = options.showLoading ?? true
    const clearOnError = options.clearOnError ?? true
    const targetProjectId = projectId
    const requestEpoch = workspaceEpochRef.current
    const treeRequestID = projection === 'summary' ? treeRequestRef.current : treeRequestRef.current + 1
    const summaryRequestID = projection === 'tree' ? summaryRequestRef.current : summaryRequestRef.current + 1
    if (projection !== 'summary') treeRequestRef.current = treeRequestID
    if (projection !== 'tree') summaryRequestRef.current = summaryRequestID
    if (!targetProjectId) {
      if (projection !== 'summary') setTree([])
      if (projection !== 'tree') setSummary(null)
      if (projection !== 'summary') setLoading(false)
      if (projection !== 'summary') setWorkspaceSnapshotLoaded(true)
      return
    }
    if (showLoading && projection !== 'summary') setLoading(true)
    const current = () => (
      requestEpoch === workspaceEpochRef.current
      && projectIdRef.current === targetProjectId
      && (projection === 'summary' || treeRequestRef.current === treeRequestID)
      && (projection === 'tree' || summaryRequestRef.current === summaryRequestID)
    )
    try {
      const [nextTree, nextSummary] = await Promise.all([
        projection === 'summary'
          ? Promise.resolve<FileNode[] | null>(null)
          : getProjectBookTree(targetProjectId) as Promise<FileNode[]>,
        projection === 'tree'
          ? Promise.resolve<WorkspaceSummary | null>(null)
          : getProjectBookSummary(targetProjectId),
      ])
      if (!current()) return
      if (nextTree) setTree(nextTree)
      if (nextSummary) setSummary(nextSummary)
    } catch (e) {
      if (!current()) return
      console.error('[hooks/useWorkspace.ts] loading Project Book snapshot failed', {
        projectId: targetProjectId,
        projection,
        error: e,
      })
      if (clearOnError && projection !== 'summary') setTree([])
      if (clearOnError && projection !== 'tree') setSummary(null)
    } finally {
      if (showLoading && projection !== 'summary' && current()) {
        setLoading(false)
      }
      if (projection !== 'summary' && current()) setWorkspaceSnapshotLoaded(true)
    }
  }, [projectId])

  const fetchSummary = useCallback(
    (options: WorkspaceRefreshOptions = {}) => fetchBookSnapshot(options, 'summary'),
    [fetchBookSnapshot],
  )

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
      console.error('[hooks/useWorkspace.ts] failed to load the bookshelf', e)
      setBooks([])
      setBookSortMode('recent')
    } finally {
      if (requestID === booksRequestRef.current) setBooksLoaded(true)
    }
  }, [])

  useEffect(() => {
    // React StrictMode replays mount effects in development; the canonical startup
    // snapshot remains valid for both passes and should only hit the server once.
    if (initialWorkspaceLoadStartedRef.current) return
    initialWorkspaceLoadStartedRef.current = true
    void Promise.all([fetchWorkspace(), fetchBooks()])
  }, [fetchWorkspace, fetchBooks])

  useEffect(() => {
    if (!workspaceLoaded) return
    if (!workspace) {
      resetWorkspaceState()
      return
    }
    if (!projectId) return
    void fetchBookSnapshot()
  }, [fetchBookSnapshot, projectId, resetWorkspaceState, workspace, workspaceLoaded])

  // 窗口重新激活时刷新派生状态；Agent 的文件事件另有即时刷新，避免固定周期扫描整本作品。
  useEffect(() => {
    if (!autoRefreshEnabled || !workspaceLoaded || !workspace || loading) return
    let cancelled = false
    let inFlight: Promise<void> | null = null
    const backgroundOptions = { showLoading: false, clearOnError: false }
    const refreshIfVisible = () => {
      if (cancelled || document.visibilityState !== 'visible') return Promise.resolve()
      if (inFlight) return inFlight
      inFlight = fetchBookSnapshot(backgroundOptions).finally(() => {
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
  }, [autoRefreshEnabled, fetchBookSnapshot, loading, projectId, workspace, workspaceLoaded])

  /** 选中文件并加载内容 */
  const selectFile = useCallback(async (path: string): Promise<SelectFileResult> => {
    // Explicit refresh paths use refreshSelectedFile. Re-selecting the active file here
    // only replaces the same document object and wakes the entire writing workbench.
    if (selectedFileRef.current === path) return 'selected'
    const targetProjectId = projectIdRef.current
    const requestID = selectFileRequestRef.current + 1
    selectFileRequestRef.current = requestID
    if (workspaceFileKind(path) === 'image') {
      setSelectedFile(path)
      setFileDocument(imageProjectFileDocument(targetProjectId, path))
      return 'selected'
    }
    if (!targetProjectId) return 'unavailable'
    const { key, generation } = beginFileRead(targetProjectId, path)
    try {
      const data = await readProjectFile(targetProjectId, path)
      if (requestID !== selectFileRequestRef.current) return 'unavailable'
      if (!isLatestFileRead(key, generation)) return 'unavailable'
      if (projectIdRef.current !== targetProjectId || data.project_id !== targetProjectId) return 'unavailable'
      // React batches these updates so the active editor receives one consistent document snapshot.
      setSelectedFile(path)
      setFileDocument(data)
      recordFileVersion(data.project_id, path, data.revision || '')
      return 'selected'
    } catch (e) {
      if (e instanceof APIError && e.status === 404) {
        console.info('[hooks/useWorkspace.ts] selected project file no longer exists', {
          projectId: targetProjectId,
          path,
        })
        return 'missing'
      }
      console.error('[hooks/useWorkspace.ts] failed to read the selected project file', e)
      return 'unavailable'
    }
  }, [beginFileRead, isLatestFileRead, recordFileVersion, setFileDocument])

  /** 清空当前选中文件，用于关闭最后一个 tab 等场景 */
  const clearSelectedFile = useCallback(() => {
    setSelectedFile(null)
    setFileDocument(null)
  }, [setFileDocument])

  /** 读取指定文件内容 */
  const readFile = useCallback(async (path: string) => {
    const targetProjectId = projectIdRef.current
    if (!targetProjectId) throw new Error('A stable project identity is required to read a Book file')
    const data = await readProjectFile(targetProjectId, path)
    return data.content || ''
  }, [])

  /** Re-read one selected file, or retain its last snapshot as an orphan after deletion. */
  const refreshSelectedFile = useCallback(async (targetProjectId: string, currentFile: string, deleted = false) => {
    const readRequest = beginFileRead(targetProjectId, currentFile)
    const isCurrentTarget = () => (
      isLatestFileRead(readRequest.key, readRequest.generation) &&
      projectIdRef.current === targetProjectId &&
      selectedFileRef.current === currentFile
    )

    if (deleted) {
      if (!isCurrentTarget()) return
      const currentDocument = fileDocumentRef.current
      if (currentDocument) setFileDocument({ ...currentDocument, revision: MISSING_WORKSPACE_REVISION })
      recordFileVersion(targetProjectId, currentFile, MISSING_WORKSPACE_REVISION)
      console.info('[useWorkspace.ts] selected workspace file became orphaned', {
        projectId: targetProjectId,
        path: currentFile,
      })
      return
    }

    if (workspaceFileKind(currentFile) === 'image') {
      if (!isCurrentTarget()) return
      filePreviewVersionRef.current += 1
      const currentDocument = fileDocumentRef.current
      setFileDocument(currentDocument
        ? { ...currentDocument, revision: `watch:${filePreviewVersionRef.current}` }
        : imageProjectFileDocument(targetProjectId, currentFile, `watch:${filePreviewVersionRef.current}`))
      return
    }

    try {
      const data = await readProjectFile(targetProjectId, currentFile)
      if (!isCurrentTarget() || data.project_id !== targetProjectId) return
      setFileDocument(data)
      recordFileVersion(data.project_id, currentFile, data.revision || '')
    } catch (error) {
      if (error instanceof APIError && error.status === 404 && isCurrentTarget()) {
        const currentDocument = fileDocumentRef.current
        if (currentDocument) setFileDocument({ ...currentDocument, revision: MISSING_WORKSPACE_REVISION })
        recordFileVersion(targetProjectId, currentFile, MISSING_WORKSPACE_REVISION)
        console.info('[useWorkspace.ts] selected workspace file was missing during refresh', {
          projectId: targetProjectId,
          path: currentFile,
        })
        return
      }
      console.error('[useWorkspace.ts] failed to refresh selected workspace file', {
        projectId: targetProjectId,
        path: currentFile,
        error,
      })
    }
  }, [beginFileRead, isLatestFileRead, recordFileVersion, setFileDocument])

  /** Refreshes only the projections invalidated by an Agent or review mutation. */
  const refreshAfterAgentFileChange = useCallback(async (
    changedPath?: string,
    impact: WorkspaceChangeImpact = 'structure',
  ) => {
    const targetProjectId = projectId
    if (!targetProjectId) return
    const currentFile = selectedFileRef.current
    const selectedRefresh = currentFile && (
      !changedPath || changedPath === currentFile || changedPath.endsWith('/' + currentFile)
    ) ? refreshSelectedFile(targetProjectId, currentFile) : Promise.resolve()
    await Promise.all([
      fetchBookSnapshot({}, impact === 'structure' ? 'all' : 'summary'),
      selectedRefresh,
    ])
  }, [fetchBookSnapshot, projectId, refreshSelectedFile])

  const refreshAfterWorkspaceFileEvent = useCallback(async (event: WorkspaceChangeEvent) => {
    const targetWorkspace = workspaceRef.current
    const targetProjectId = projectIdRef.current
    if (!targetWorkspace || !targetProjectId || event.project_id !== targetProjectId) return
    const changes = event.changes ?? []
    const backgroundOptions = { showLoading: false, clearOnError: false }
    const refreshes: Promise<void>[] = []

    const structureChanged = event.resync || changes.some(change => change.type === 'added' || change.type === 'deleted')
    const summaryChanged = event.resync || changes.some(workspaceChangeAffectsSummary)
    if (structureChanged || summaryChanged) {
      const projection = structureChanged && summaryChanged ? 'all' : structureChanged ? 'tree' : 'summary'
      refreshes.push(fetchBookSnapshot(backgroundOptions, projection))
    }

    const currentFile = selectedFileRef.current
    if (currentFile) {
      const presentChange = changes.some(change => change.path === currentFile && change.type !== 'deleted')
      const deletedChange = changes.some(change => (
        change.type === 'deleted' && pathContainsFile(change.path, currentFile)
      ))
      if (event.resync || presentChange) {
        refreshes.push(refreshSelectedFile(targetProjectId, currentFile))
      } else if (deletedChange) {
        refreshes.push(refreshSelectedFile(targetProjectId, currentFile, true))
      }
    }

    await Promise.all(refreshes)
  }, [fetchBookSnapshot, refreshSelectedFile])

  useProjectFileEvents(projectId, refreshAfterWorkspaceFileEvent)

  /** Saves an editor draft against the revision captured with that draft. Typed API errors propagate to the editor adapter. */
  const saveFileDraft = useCallback(async (path: string, content: string, draftBaseRevision: string) => {
    if (!projectId || !path) throw new Error('project ID and path are required to save an editor draft')
    const version = fileVersionsRef.current.get(path)
    const targetProjectId = version?.projectId || projectId
    let result: Awaited<ReturnType<typeof saveProjectFile>>
    try {
      result = await saveProjectFile(targetProjectId, path, content, draftBaseRevision)
    } catch (error) {
      if (error instanceof APIError && error.code === 'revision_conflict') {
        try {
          const latest = await readProjectFile(targetProjectId, path)
          if (latest.project_id !== targetProjectId) {
            console.warn('[useWorkspace.ts] ignored revision-conflict reload from a different project', {
              path,
              targetProjectId,
              loadedProjectId: latest.project_id,
            })
            throw error
          }
          if (projectIdRef.current === targetProjectId && selectedFileRef.current === path) {
            setFileDocument(latest)
            recordFileVersion(targetProjectId, path, latest.revision || '')
          }
          throw new WorkspaceFileRevisionConflictError(error, {
            workspace: targetProjectId,
            content: latest.content || '',
            revision: latest.revision || '',
          })
        } catch (reloadError) {
          if (reloadError instanceof WorkspaceFileRevisionConflictError) throw reloadError
          console.error('[useWorkspace.ts] failed to reload editor file after revision conflict', {
            path,
            targetProjectId,
            reloadError,
          })
        }
      }
      throw error
    }
    if (result.revision && projectIdRef.current === targetProjectId) {
      const currentVersion = fileVersionsRef.current.get(path)
      if (currentVersion?.projectId === targetProjectId && currentVersion.revision === draftBaseRevision) {
        recordFileVersion(targetProjectId, path, result.revision)
      }
      if (selectedFileRef.current === path && fileDocumentRef.current?.revision === draftBaseRevision) {
        const currentDocument = fileDocumentRef.current
        if (currentDocument) setFileDocument({
          ...currentDocument,
          content,
          revision: result.revision,
          size: new TextEncoder().encode(content).byteLength,
        })
      }
    }
    // 文件写入成功即完成保存；章节统计是派生数据，不能延长编辑器的 saving 状态。
    queueSummaryRefreshAfterSave()
    return result
  }, [projectId, queueSummaryRefreshAfterSave, recordFileVersion, setFileDocument])

  /** 保存指定文件内容；路径和 revision 绑定，避免文件切换期间的迟到响应串写。 */
  const saveFileContent = useCallback(async (path: string, content: string): Promise<boolean> => {
    if (!projectId || !path) return false
    const version = fileVersionsRef.current.get(path)
    try {
      await saveFileDraft(path, content, version?.revision || '')
      return true
    } catch (e) {
      if (e instanceof APIError) {
        console.error('[hooks/useWorkspace.ts] project file save was rejected', {
          path,
          status: e.status,
          code: e.code,
          details: e.details,
          error: e,
        })
      } else {
        console.error('[hooks/useWorkspace.ts] failed to save the project file', e)
      }
      return false
    }
  }, [projectId, saveFileDraft])

  /** 切换 workspace 后刷新所有状态 */
  const refreshAll = useCallback(async () => {
    treeRequestRef.current += 1
    summaryRequestRef.current += 1
    setSelectedFile(null)
    setFileDocument(null)
    selectFileRequestRef.current += 1
    fileVersionsRef.current.clear()
    fileReadGenerationsRef.current.clear()
    await Promise.all([fetchWorkspace(), fetchBooks()])
  }, [fetchWorkspace, fetchBooks, setFileDocument])

  const applyWorkspaceFileOperation = useCallback(async (
    operation: ProjectFileOperation,
  ): Promise<ProjectFileOperationResult> => {
    if (!projectId) throw new Error('A stable project identity is required for workspace file operations')
    const [result] = await applyProjectFileOperations(projectId, [operation])
    if (!result?.ok) throw new Error(result?.error || 'Project file operation failed')
    return result
  }, [projectId])

  /** 新建文件或目录 */
  const createItem = useCallback(async (path: string, type: 'file' | 'dir') => {
    await applyWorkspaceFileOperation({ kind: 'create', path, type, content: '' })
    await fetchBookSnapshot()
  }, [applyWorkspaceFileOperation, fetchBookSnapshot])

  /** 删除文件或目录 */
  const deleteItem = useCallback(async (path: string) => {
    await applyWorkspaceFileOperation({ kind: 'delete', path })
    if (selectedFile === path || selectedFile?.startsWith(`${path}/`)) {
      setSelectedFile(null)
      setFileDocument(null)
    }
    await fetchBookSnapshot()
  }, [applyWorkspaceFileOperation, fetchBookSnapshot, selectedFile, setFileDocument])

  /** 重命名文件或目录 */
  const renameItem = useCallback(async (path: string, newName: string) => {
    const result = await applyWorkspaceFileOperation({ kind: 'rename', path, new_name: newName })
    const renamedPath = result.path || path
    if (selectedFile === path) {
      setSelectedFile(renamedPath)
      await selectFile(renamedPath)
    } else if (selectedFile?.startsWith(`${path}/`)) {
      const nextPath = `${renamedPath}/${selectedFile.slice(path.length + 1)}`
      setSelectedFile(nextPath)
      await selectFile(nextPath)
    }
    await fetchBookSnapshot()
  }, [applyWorkspaceFileOperation, fetchBookSnapshot, selectFile, selectedFile])

  /** 复制文件或目录 */
  const copyItem = useCallback(async (from: string, to: string) => {
    await applyWorkspaceFileOperation({ kind: 'copy', path: from, to })
    await fetchBookSnapshot()
  }, [applyWorkspaceFileOperation, fetchBookSnapshot])

  /** 移动文件或目录 */
  const moveItem = useCallback(async (from: string, to: string) => {
    const result = await applyWorkspaceFileOperation({ kind: 'move', path: from, to })
    const movedPath = result.path || to
    if (selectedFile === from) {
      setSelectedFile(movedPath)
      await selectFile(movedPath)
    } else if (selectedFile?.startsWith(`${from}/`)) {
      const nextPath = `${movedPath}/${selectedFile.slice(from.length + 1)}`
      setSelectedFile(nextPath)
      await selectFile(nextPath)
    }
    await fetchBookSnapshot()
  }, [applyWorkspaceFileOperation, fetchBookSnapshot, selectFile, selectedFile])

  /** 刷新目录树和章节统计 */
  const refresh = useCallback(async () => {
    if (!workspace) {
      resetWorkspaceState()
      return
    }
    await fetchBookSnapshot()
  }, [fetchBookSnapshot, resetWorkspaceState, workspace])

  return {
    tree,
    loading,
    selectedFile,
    fileDocument,
    fileContent,
    fileRevision,
    workspace,
    projectId,
    workspaceLoaded,
    workspaceSnapshotLoaded,
    summary,
    books,
    booksLoaded,
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
  }
}

export function workspaceChangeAffectsSummary(change: WorkspaceFileChange): boolean {
  const path = change.path.replace(/^\.\//, '').replace(/\/+$/, '')
  return path === 'book.json' ||
    path === 'ideas.md' ||
    path === 'chapters' ||
    path.startsWith('chapters/') ||
    path === 'setting' ||
    path === 'setting/outline.md' ||
    path === 'setting/chapter-groups' ||
    path.startsWith('setting/chapter-groups/')
}

function pathContainsFile(path: string, file: string): boolean {
  return path === file || file.startsWith(`${path}/`)
}

function imageProjectFileDocument(projectId: string, path: string, revision = ''): ProjectFileDocument {
  return {
    project_id: projectId,
    path,
    revision,
    kind: 'image',
    mime_type: 'application/octet-stream',
    size: 0,
    editable: false,
  }
}
