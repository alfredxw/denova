import { useState, useEffect, useCallback, useRef } from 'react'
import {
  copyWorkspaceItem,
  createWorkspaceItem,
  deleteWorkspaceItem,
  getBooks,
  getCurrentWorkspace,
  getStyles,
  getWorkspaceSummary,
  getWorkspaceTree,
  moveWorkspaceItem,
  readFile as readWorkspaceFile,
  renameWorkspaceItem,
  saveFile,
} from '@/lib/api'
import type { BookRecord } from '@/lib/api'
import type { WorkspaceSummary } from '@/lib/api'

export interface FileNode {
  name: string
  type: 'file' | 'dir'
  children?: FileNode[]
}

const TREE_AUTO_REFRESH_INTERVAL_MS = 3000

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
  const [fileContent, setFileContent] = useState<string>('')
  const [workspace, setWorkspace] = useState<string>('')
  const [workspaceLoaded, setWorkspaceLoaded] = useState(false)
  const [summary, setSummary] = useState<WorkspaceSummary | null>(null)
  const [styles, setStyles] = useState<string[]>([])
  const [books, setBooks] = useState<BookRecord[]>([])

  // 用 ref 追踪最新 selectedFile，避免异步回调闭包捕获旧值
  const selectedFileRef = useRef<string | null>(null)
  const selectedFileRevisionRef = useRef<string>('')
  const selectFileRequestRef = useRef(0)
  selectedFileRef.current = selectedFile

  const resetWorkspaceState = useCallback(() => {
    setTree([])
    setLoading(false)
    setSelectedFile(null)
    setFileContent('')
    selectedFileRevisionRef.current = ''
    setSummary(null)
    setStyles([])
  }, [])

  /** 获取当前 workspace 路径 */
  const fetchWorkspace = useCallback(async () => {
    try {
      const data = await getCurrentWorkspace()
      setWorkspace(data.workspace || '')
    } catch (e) {
      console.error('获取 workspace 失败', e)
      setWorkspace('')
    } finally {
      setWorkspaceLoaded(true)
    }
  }, [])

  const fetchTree = useCallback(async (options: WorkspaceRefreshOptions = {}) => {
    const showLoading = options.showLoading ?? true
    const clearOnError = options.clearOnError ?? true
    if (!workspace) {
      setTree([])
      setLoading(false)
      return
    }
    if (showLoading) setLoading(true)
    try {
      setTree((await getWorkspaceTree()) as FileNode[])
    } catch (e) {
      console.error('获取目录树失败', e)
      if (clearOnError) setTree([])
    } finally {
      if (showLoading) setLoading(false)
    }
  }, [workspace])

  /** 获取当前作品章节统计 */
  const fetchSummary = useCallback(async (options: WorkspaceRefreshOptions = {}) => {
    const clearOnError = options.clearOnError ?? true
    if (!workspace) {
      setSummary(null)
      return
    }
    try {
      setSummary(await getWorkspaceSummary())
    } catch (e) {
      console.error('获取作品统计失败', e)
      if (clearOnError) setSummary(null)
    }
  }, [workspace])

  /** 获取用户级 styles 下的风格参考文件 */
  const fetchStyles = useCallback(async (options: WorkspaceRefreshOptions = {}) => {
    const clearOnError = options.clearOnError ?? true
    if (!workspace) {
      setStyles([])
      return
    }
    try {
      setStyles(await getStyles())
    } catch (e) {
      console.error('获取风格参考失败', e)
      if (clearOnError) setStyles([])
    }
  }, [workspace])

  /** 获取当前 Nova 数据目录下实际存在的书籍列表 */
  const fetchBooks = useCallback(async () => {
    try {
      setBooks(await getBooks())
    } catch (e) {
      console.error('获取书籍列表失败', e)
      setBooks([])
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
    void Promise.all([fetchTree(), fetchStyles(), fetchSummary()])
  }, [fetchSummary, fetchStyles, fetchTree, resetWorkspaceState, workspace, workspaceLoaded])

  // 自动刷新目录树，覆盖 AI Agent 直接写入文件后的结构变化。
  useEffect(() => {
    if (!autoRefreshEnabled || !workspaceLoaded || !workspace) return
    const refreshIfVisible = () => {
      if (document.visibilityState === 'visible') {
        const backgroundOptions = { showLoading: false, clearOnError: false }
        void Promise.all([
          fetchTree(backgroundOptions),
          fetchStyles(backgroundOptions),
          fetchSummary(backgroundOptions),
        ])
      }
    }

    const timer = window.setInterval(refreshIfVisible, TREE_AUTO_REFRESH_INTERVAL_MS)
    window.addEventListener('focus', refreshIfVisible)
    document.addEventListener('visibilitychange', refreshIfVisible)

    return () => {
      window.clearInterval(timer)
      window.removeEventListener('focus', refreshIfVisible)
      document.removeEventListener('visibilitychange', refreshIfVisible)
    }
  }, [autoRefreshEnabled, fetchTree, fetchStyles, fetchSummary, workspace, workspaceLoaded])

  /** 选中文件并加载内容 */
  const selectFile = useCallback(async (path: string) => {
    const requestID = selectFileRequestRef.current + 1
    selectFileRequestRef.current = requestID
    try {
      const data = await readWorkspaceFile(path)
      if (requestID !== selectFileRequestRef.current) return
      // React 18 自动批量：两个 setState 合并为一次渲染，确保 MarkdownEditor 拿到一致的 (fileName, content)
      setSelectedFile(path)
      setFileContent(data.content || '')
      selectedFileRevisionRef.current = data.revision || ''
    } catch (e) {
      console.error('读取文件失败', e)
    }
  }, [])

  /** 清空当前选中文件，用于关闭最后一个 tab 等场景 */
  const clearSelectedFile = useCallback(() => {
    setSelectedFile(null)
    setFileContent('')
    selectedFileRevisionRef.current = ''
  }, [])

  /** 读取指定文件内容 */
  const readFile = useCallback(async (path: string) => {
    const data = await readWorkspaceFile(path)
    return data.content || ''
  }, [])

  /** Agent 写入或创建文件后，刷新目录树并同步当前打开文件内容。 */
  const refreshAfterAgentFileChange = useCallback(async (changedPath?: string) => {
    if (!workspace) return
    await Promise.all([fetchTree(), fetchStyles(), fetchSummary()])
    const currentFile = selectedFileRef.current
    if (!currentFile) return

    // changedPath 可能是绝对路径，selectedFile 是相对路径
    // 判断是否为同一文件：相对路径匹配或绝对路径以相对路径结尾
    if (changedPath) {
      const isMatch = changedPath === currentFile || changedPath.endsWith('/' + currentFile)
      if (!isMatch) return
    }

    try {
      const data = await readWorkspaceFile(currentFile)
      // 仅当选中文件没有在异步期间改变时才更新内容
      if (selectedFileRef.current === currentFile) {
        setFileContent(data.content || '')
        selectedFileRevisionRef.current = data.revision || ''
      }
    } catch (e) {
      console.error('刷新当前文件失败', e)
    }
  }, [fetchTree, fetchStyles, fetchSummary, workspace])

  /** 保存当前文件内容 */
  const saveCurrentFile = useCallback(async (content: string): Promise<boolean> => {
    if (!workspace || !selectedFile) return false
    try {
      const result = await saveFile(selectedFile, content, selectedFileRevisionRef.current)
      if (result.revision) selectedFileRevisionRef.current = result.revision
      await fetchSummary()
      return true
    } catch (e) {
      console.error('保存文件失败', e)
      return false
    }
  }, [fetchSummary, selectedFile, workspace])

  /** 切换 workspace 后刷新所有状态 */
  const refreshAll = useCallback(async () => {
    setSelectedFile(null)
    setFileContent('')
    selectedFileRevisionRef.current = ''
    await Promise.all([fetchWorkspace(), fetchBooks()])
  }, [fetchWorkspace, fetchBooks])

  /** 新建文件或目录 */
  const createItem = useCallback(async (path: string, type: 'file' | 'dir') => {
    await createWorkspaceItem({ path, type, content: '' })
    await Promise.all([fetchTree(), fetchStyles(), fetchSummary()])
  }, [fetchTree, fetchStyles, fetchSummary])

  /** 删除文件或目录 */
  const deleteItem = useCallback(async (path: string) => {
    await deleteWorkspaceItem(path)
    if (selectedFile === path || selectedFile?.startsWith(`${path}/`)) {
      setSelectedFile(null)
      setFileContent('')
      selectedFileRevisionRef.current = ''
    }
    await Promise.all([fetchTree(), fetchStyles(), fetchSummary()])
  }, [fetchTree, fetchStyles, fetchSummary, selectedFile])

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
    await Promise.all([fetchTree(), fetchStyles(), fetchSummary()])
  }, [fetchTree, fetchStyles, fetchSummary, selectFile, selectedFile])

  /** 复制文件或目录 */
  const copyItem = useCallback(async (from: string, to: string) => {
    await copyWorkspaceItem({ from, to })
    await Promise.all([fetchTree(), fetchStyles(), fetchSummary()])
  }, [fetchTree, fetchStyles, fetchSummary])

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
    await Promise.all([fetchTree(), fetchStyles(), fetchSummary()])
  }, [fetchTree, fetchStyles, fetchSummary, selectFile, selectedFile])

  /** 刷新目录树和风格参考 */
  const refresh = useCallback(async () => {
    if (!workspace) {
      resetWorkspaceState()
      return
    }
    await Promise.all([fetchTree(), fetchStyles(), fetchSummary()])
  }, [fetchTree, fetchStyles, fetchSummary, resetWorkspaceState, workspace])

  return {
    tree,
    loading,
    selectedFile,
    fileContent,
    workspace,
    workspaceLoaded,
    summary,
    styles,
    books,
    selectFile,
    clearSelectedFile,
    saveCurrentFile,
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
