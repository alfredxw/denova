import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { BookOpen } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { EmptyState } from '@/components/common/EmptyState'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { SearchPanel } from '@/components/Sidebar/SearchPanel'
import { MarkdownEditor } from '@/components/Editor/MarkdownEditor'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { ChapterOutline } from '@/components/workbench/outline/ChapterOutline'
import type { WorkspaceChangeMetadata } from '@/features/changes/types'
import type { DocumentReviewController } from '@/features/document-review/controller'
import type { AgentChatDocumentReviewNavigation } from '@/features/agent-chat/types'
import {
  APIError,
  getProjectBookSummary,
  getProjectBookSnapshot,
  readProjectFile,
  saveProjectFile,
  setProjectChapterConfirmed,
  type ProjectBookFileNode,
  type WorkspaceSearchResult,
  type WorkspaceSummary,
} from '@/lib/api'
import { WorkspaceFileRevisionConflictError } from '@/lib/autosave/workspace-file-revision-conflict'

interface ProjectWritingSurfaceProps {
  projectId: string
  initialPath?: string | null
  autoSaveEnabled?: boolean
  autoSaveDelayMs?: number
  documentReview: DocumentReviewController
  navigationIntent?: AgentChatDocumentReviewNavigation | null
  refreshSignal?: number
  onOpenLoreTab?: () => void
  onFlushHandlerChange: (handler: EditorFlushHandler | null) => void
  onWorkspaceChanged?: (paths: string[], metadata: WorkspaceChangeMetadata) => void | Promise<void>
}

interface OpenDocument {
  path: string
  content: string
  revision: string
}

const EMPTY_DOCUMENT: OpenDocument = { path: '', content: '', revision: '' }

/**
 * Project-scoped manuscript surface shared by embedded Writing hosts.
 *
 * All resource operations use Project ID. Workspace is presentation metadata,
 * so mounting this surface cannot switch the app's foreground Book or mode.
 */
export function ProjectWritingSurface({
  projectId,
  initialPath,
  autoSaveEnabled = true,
  autoSaveDelayMs,
  documentReview,
  navigationIntent,
  refreshSignal = 0,
  onOpenLoreTab,
  onFlushHandlerChange,
  onWorkspaceChanged,
}: ProjectWritingSurfaceProps) {
  const { t } = useTranslation()
  const [tree, setTree] = useState<ProjectBookFileNode[]>([])
  const [summary, setSummary] = useState<WorkspaceSummary | null>(null)
  const [selectedPath, setSelectedPath] = useState(initialPath || '')
  const [document, setDocument] = useState<OpenDocument>(EMPTY_DOCUMENT)
  const [sidebarView, setSidebarView] = useState<'outline' | 'search'>('outline')
  const [searchIntent, setSearchIntent] = useState<{ path: string; query: string; line: number; nonce: number } | null>(null)
  const [loadingBook, setLoadingBook] = useState(Boolean(projectId))
  const [loadingDocument, setLoadingDocument] = useState(false)
  const [error, setError] = useState('')
  const selectedPathRef = useRef(selectedPath)
  const snapshotRequestRef = useRef(0)
  const summaryRequestRef = useRef(0)
  const documentRequestRef = useRef(0)
  const refreshSignalRef = useRef(0)
  const editorFlushRef = useRef<EditorFlushHandler | null>(null)

  const chapters = useMemo(
    () => [...(summary?.chapters || [])].sort((left, right) => left.index - right.index),
    [summary?.chapters],
  )

  const loadSnapshot = useCallback(async () => {
    const request = ++snapshotRequestRef.current
    if (!projectId) {
      setTree([])
      setSummary(null)
      setLoadingBook(false)
      return
    }
    setLoadingBook(true)
    setError('')
    try {
      const snapshot = await getProjectBookSnapshot(projectId)
      if (request !== snapshotRequestRef.current) return
      setTree(snapshot.tree)
      setSummary(snapshot.summary)
      setSelectedPath((current) => {
        const preferred = current || initialPath || ''
        const nextPath = preferred && projectTreeContainsPath(snapshot.tree, preferred)
          ? preferred
          : snapshot.summary.chapters[0]?.path || ''
        selectedPathRef.current = nextPath
        return nextPath
      })
    } catch (cause) {
      if (request !== snapshotRequestRef.current) return
      console.error('[features/writing/ProjectWritingSurface.tsx] loading project Book failed', {
        projectId,
        cause,
      })
      setError(cause instanceof Error ? cause.message : String(cause))
      setTree([])
      setSummary(null)
    } finally {
      if (request === snapshotRequestRef.current) setLoadingBook(false)
    }
  }, [initialPath, projectId])

  const loadSummary = useCallback(async () => {
    const request = ++summaryRequestRef.current
    if (!projectId) {
      setSummary(null)
      return
    }
    try {
      const nextSummary = await getProjectBookSummary(projectId)
      if (request === summaryRequestRef.current) setSummary(nextSummary)
    } catch (cause) {
      if (request !== summaryRequestRef.current) return
      console.error('[features/writing/ProjectWritingSurface.tsx] loading project Book summary failed', {
        projectId,
        cause,
      })
    }
  }, [projectId])

  const loadDocument = useCallback(async (path: string) => {
    const request = ++documentRequestRef.current
    if (!projectId || !path) {
      setDocument(EMPTY_DOCUMENT)
      return
    }
    setLoadingDocument(true)
    setError('')
    try {
      const file = await readProjectFile(projectId, path)
      if (request !== documentRequestRef.current) return
      if (file.project_id !== projectId) return
      setDocument({ path: file.path, content: file.content || '', revision: file.revision })
    } catch (cause) {
      if (request !== documentRequestRef.current) return
      console.error('[features/writing/ProjectWritingSurface.tsx] reading project file failed', {
        projectId,
        path,
        cause,
      })
      setError(cause instanceof Error ? cause.message : String(cause))
      setDocument(EMPTY_DOCUMENT)
    } finally {
      if (request === documentRequestRef.current) setLoadingDocument(false)
    }
  }, [projectId])

  useEffect(() => {
    setTree([])
    setSummary(null)
    selectedPathRef.current = initialPath || ''
    setSelectedPath(selectedPathRef.current)
    setDocument(EMPTY_DOCUMENT)
    refreshSignalRef.current = refreshSignal
    void loadSnapshot()
    return () => {
      snapshotRequestRef.current += 1
      summaryRequestRef.current += 1
      documentRequestRef.current += 1
    }
  }, [initialPath, loadSnapshot, projectId])

  useEffect(() => {
    void loadDocument(selectedPath)
  }, [loadDocument, selectedPath])

  useEffect(() => {
    if (refreshSignal <= refreshSignalRef.current) return
    refreshSignalRef.current = refreshSignal
    void Promise.all([loadSnapshot(), selectedPath ? loadDocument(selectedPath) : Promise.resolve()])
  }, [loadDocument, loadSnapshot, refreshSignal, selectedPath])

  const handleFlushHandlerChange = useCallback((handler: EditorFlushHandler | null) => {
    editorFlushRef.current = handler
    onFlushHandlerChange(handler)
  }, [onFlushHandlerChange])

  const selectFile = useCallback(async (path: string) => {
    if (!path || path === selectedPathRef.current) return true
    if (editorFlushRef.current && !(await editorFlushRef.current())) return false
    selectedPathRef.current = path
    setSelectedPath(path)
    return true
  }, [])

  const selectOutlineFile = useCallback((path: string) => {
    void selectFile(path)
  }, [selectFile])

  const navigationPath = navigationIntent?.projectId === projectId
    && navigationIntent.target.kind === 'workspace_file'
    ? navigationIntent.target.id
    : ''
  useEffect(() => {
    if (navigationPath) void selectFile(navigationPath)
  }, [navigationIntent?.nonce, navigationPath, selectFile])

  const save = useCallback(async (path: string, content: string, baseRevision: string) => {
    try {
      const result = await saveProjectFile(projectId, path, content, baseRevision)
      setDocument((current) => current.path === path
        ? { ...current, content, revision: result.revision || current.revision }
        : current)
      void loadSummary()
      await onWorkspaceChanged?.([path], { impact: 'content', origin: 'project-page' })
      return result
    } catch (cause) {
      if (cause instanceof APIError && cause.code === 'revision_conflict') {
        const latest = await readProjectFile(projectId, path)
        if (latest.project_id !== projectId) throw cause
        throw new WorkspaceFileRevisionConflictError(cause, {
          workspace: projectId,
          content: latest.content || '',
          revision: latest.revision,
        })
      }
      throw cause
    }
  }, [loadSummary, onWorkspaceChanged, projectId])

  const setChapterConfirmed = useCallback(async (path: string, confirmed: boolean) => {
    await setProjectChapterConfirmed(projectId, path, confirmed)
    await loadSummary()
    await onWorkspaceChanged?.([path], { impact: 'content', origin: 'project-page' })
  }, [loadSummary, onWorkspaceChanged, projectId])

  const selectSearchResult = useCallback(async (result: WorkspaceSearchResult, query: string) => {
    if (!await selectFile(result.path)) return
    setSearchIntent((current) => ({
      path: result.path,
      query,
      line: result.line,
      nonce: (current?.nonce || 0) + 1,
    }))
  }, [selectFile])

  const handleSearchMutation = useCallback(async (paths: string[]) => {
    await onWorkspaceChanged?.(paths, { impact: 'content', origin: 'project-page' })
    await Promise.all([
      loadSnapshot(),
      selectedPath && paths.includes(selectedPath) ? loadDocument(selectedPath) : Promise.resolve(),
    ])
  }, [loadDocument, loadSnapshot, onWorkspaceChanged, selectedPath])

  const displayedChapter = chapters.find((chapter) => chapter.path === document.path)
  const directory = (
    <div className="nova-sidebar flex h-full min-h-0 flex-col bg-[var(--nova-surface-2)]">
      <div className="grid shrink-0 grid-cols-2 gap-1 border-b border-[var(--nova-border)] p-2">
        {(['outline', 'search'] as const).map((view) => (
          <button
            key={view}
            type="button"
            onClick={() => setSidebarView(view)}
            className={`nova-nav-item h-7 min-w-0 truncate px-2 text-[11px] ${sidebarView === view ? 'is-active' : 'bg-[var(--nova-surface)]'}`}
          >
            {t(view === 'outline' ? 'router.outline' : 'router.search')}
          </button>
        ))}
      </div>
      <div className="min-h-0 flex-1">
        {sidebarView === 'search' ? (
          <div className="h-full min-h-0 p-2">
            <SearchPanel
              projectId={projectId}
              onSelectResult={selectSearchResult}
              onBeforeReplace={() => editorFlushRef.current?.() ?? true}
              onWorkspaceChanged={handleSearchMutation}
            />
          </div>
        ) : (
          <ChapterOutline
            projectId={projectId}
            tree={tree}
            chapters={chapters}
            ideas={summary?.ideas}
            outline={summary?.outline}
            chapterPlans={summary?.chapter_plans || []}
            selectedFile={selectedPath || null}
            onSelectFile={selectOutlineFile}
            onOpenLoreTab={onOpenLoreTab}
            onSetChapterConfirmed={setChapterConfirmed}
          />
        )}
      </div>
    </div>
  )

  if (!projectId) {
    return <EmptyState variant="page" icon={BookOpen} title={t('agentChat.reader.noWorkspace')} />
  }

  return (
    <section className="h-full min-h-0 min-w-0 bg-[var(--nova-bg)]" aria-label={t('agentChat.page.reader')}>
      <AdaptiveSurface
        left={{
          id: `project-writing-outline:${projectId}`,
          side: 'left',
          title: t('agentChat.reader.outline'),
          icon: <BookOpen className="h-4 w-4" />,
          content: directory,
          desktopClassName: 'min-h-0 border-r border-[var(--nova-border)]',
          mobileClassName: 'w-[min(88vw,340px)]',
        }}
        leftResize={{
          layoutKey: 'nova-project-writing-outline-layout',
          label: t('layout.resize.sidebar'),
          defaultSize: '240px',
          minSize: '200px',
          maxSize: '36%',
        }}
        collapseAt={720}
      >
        {({ isMobile, openLeft }) => (
          <div className="flex h-full min-h-0 min-w-0 flex-col">
            {error ? (
              <div className="p-3">
                <InlineErrorNotice message={error} title={t('agentChat.reader.loadFailed')} />
              </div>
            ) : document.path ? (
              <div className="relative flex min-h-0 flex-1 flex-col">
                <MarkdownEditor
                  projectId={projectId}
                  fileName={document.path}
                  content={document.content}
                  revision={document.revision}
                  chapterSummary={displayedChapter}
                  searchIntent={searchIntent?.path === document.path ? searchIntent : null}
                  autoSaveEnabled={autoSaveEnabled}
                  autoSaveDelayMs={autoSaveDelayMs}
                  onSave={save}
                  onFlushHandlerChange={handleFlushHandlerChange}
                  documentReview={documentReview}
                  documentReviewNavigationIntent={navigationPath === document.path ? navigationIntent : null}
                  onOpenOutline={isMobile ? openLeft : undefined}
                />
                {loadingDocument ? (
                  <div role="status" className="absolute inset-0 z-10 flex items-center justify-center bg-[var(--nova-bg)]/80 text-xs text-[var(--nova-text-faint)] backdrop-blur-[1px]">
                    {t('router.loading')}
                  </div>
                ) : null}
              </div>
            ) : selectedPath || loadingBook || loadingDocument ? (
              <div role="status" className="flex h-full items-center justify-center text-xs text-[var(--nova-text-faint)]">
                {t('router.loading')}
              </div>
            ) : (
              <EmptyState variant="page" icon={BookOpen} title={t('agentChat.reader.noSelection')} />
            )}
          </div>
        )}
      </AdaptiveSurface>
    </section>
  )
}

function projectTreeContainsPath(tree: ProjectBookFileNode[], path: string): boolean {
  const segments = path.split('/').filter(Boolean)
  let level = tree
  for (const [index, segment] of segments.entries()) {
    const node = level.find((entry) => entry.name === segment)
    if (!node) return false
    if (index === segments.length - 1) return node.type === 'file'
    if (node.type !== 'dir') return false
    level = node.children || []
  }
  return false
}
