import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { BookOpen } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { EmptyState } from '@/components/common/EmptyState'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { MarkdownEditor } from '@/components/Editor/MarkdownEditor'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { ChapterOutline } from '@/components/workbench/outline/ChapterOutline'
import type { DocumentReviewController } from '@/features/document-review/controller'
import type { FileNode } from '@/hooks/useWorkspace'
import { readFile, type WorkspaceSummary } from '@/lib/api'
import type { AgentChatDocumentReviewNavigation } from './types'

/** Same contract as the writing workbench's save handler, so the editor behaves identically. */
export type AgentChatSaveFile = (path: string, content: string, baseRevision: string) => Promise<{ revision?: string }>

interface AgentChatReaderProps {
  workspace: string
  tree: FileNode[]
  summary: WorkspaceSummary | null
  /** File opened first, normally whatever the writing workbench has selected. */
  initialPath?: string | null
  onSaveFile?: AgentChatSaveFile
  documentReview: DocumentReviewController
  navigationIntent?: AgentChatDocumentReviewNavigation | null
  onOpenLoreTab?: () => void | Promise<boolean | void>
  onSetChapterConfirmed: (path: string, confirmed: boolean) => void | Promise<void>
  onFlushHandlerChange: (handler: EditorFlushHandler | null) => void
}

/**
 * Project manuscript surface for AgentChat.
 *
 * It composes the same outline and editor used by the writing workbench. Only file selection
 * and loading are local to this tab, which lets it sit beside a conversation without changing
 * the foreground writing tab or duplicating editor behavior.
 */
export function AgentChatReader({
  workspace,
  tree,
  summary,
  initialPath,
  onSaveFile,
  documentReview,
  navigationIntent,
  onOpenLoreTab,
  onSetChapterConfirmed,
  onFlushHandlerChange,
}: AgentChatReaderProps) {
  const { t } = useTranslation()
  const chapters = useMemo(
    () => [...(summary?.chapters || [])].sort((left, right) => left.index - right.index),
    [summary?.chapters],
  )
  const defaultPath = initialPath || chapters[0]?.path || ''
  const [selectedPath, setSelectedPath] = useState(defaultPath)
  const [document, setDocument] = useState({ content: '', revision: '' })
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const editorFlushRef = useRef<EditorFlushHandler | null>(null)

  useEffect(() => {
    if (!selectedPath && defaultPath) setSelectedPath(defaultPath)
  }, [defaultPath, selectedPath])

  useEffect(() => {
    if (!selectedPath) {
      setDocument({ content: '', revision: '' })
      return
    }
    let cancelled = false
    setLoading(true)
    setError('')
    readFile(selectedPath)
      .then((file) => {
        if (!cancelled) setDocument({ content: file.content, revision: file.revision || '' })
      })
      .catch((cause) => {
        if (cancelled) return
        console.error('[features/agent-chat/AgentChatReader.tsx] reading workspace file failed', {
          workspace,
          path: selectedPath,
          cause,
        })
        setError(cause instanceof Error ? cause.message : String(cause))
        setDocument({ content: '', revision: '' })
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [selectedPath, workspace])

  const handleFlushHandlerChange = useCallback((handler: EditorFlushHandler | null) => {
    editorFlushRef.current = handler
    onFlushHandlerChange(handler)
  }, [onFlushHandlerChange])

  const selectFile = useCallback(async (path: string) => {
    if (!path || path === selectedPath) return true
    const flush = editorFlushRef.current
    if (flush && !(await flush())) return false
    setSelectedPath(path)
    return true
  }, [selectedPath])
  const selectFileFromOutline = useCallback((path: string) => {
    void selectFile(path)
  }, [selectFile])

  const navigationPath = navigationIntent?.workspace === workspace
    && navigationIntent.target.kind === 'workspace_file'
    ? navigationIntent.target.id
    : ''
  useEffect(() => {
    if (!navigationPath) return
    void selectFile(navigationPath)
  }, [navigationIntent?.nonce, navigationPath, selectFile])

  const save = useCallback(async (fileName: string, content: string, baseRevision: string) => {
    if (!onSaveFile) return false
    const result = await onSaveFile(fileName, content, baseRevision)
    if (fileName === selectedPath && result.revision) {
      setDocument((current) => ({ ...current, revision: result.revision || current.revision }))
    }
    return result
  }, [onSaveFile, selectedPath])

  const selectedChapter = chapters.find((chapter) => chapter.path === selectedPath)
  const directory = (
    <div className="nova-sidebar h-full min-h-0 bg-[var(--nova-surface-2)]">
      <ChapterOutline
        workspace={workspace}
        tree={tree}
        chapters={chapters}
        ideas={summary?.ideas}
        outline={summary?.outline}
        chapterPlans={summary?.chapter_plans || []}
        selectedFile={selectedPath || null}
        onSelectFile={selectFileFromOutline}
        onOpenLoreTab={onOpenLoreTab}
        onSetChapterConfirmed={onSetChapterConfirmed}
      />
    </div>
  )

  if (!workspace) {
    return <EmptyState variant="page" icon={BookOpen} title={t('agentChat.reader.noWorkspace')} />
  }

  return (
    <section className="h-full min-h-0 min-w-0 bg-[var(--nova-bg)]" aria-label={t('agentChat.page.reader')}>
      <AdaptiveSurface
        left={{
          id: 'agent-chat-reader-outline',
          side: 'left',
          title: t('agentChat.reader.outline'),
          icon: <BookOpen className="h-4 w-4" />,
          content: directory,
          desktopClassName: 'min-h-0 w-60 border-r border-[var(--nova-border)]',
          mobileClassName: 'w-[min(88vw,340px)]',
        }}
        desktopGridClassName="grid-cols-[15rem_minmax(0,1fr)]"
        collapseAt={720}
      >
        {({ isMobile, openLeft }) => (
          <div className="flex h-full min-h-0 min-w-0 flex-col">
            {error ? (
              <div className="p-3">
                <InlineErrorNotice message={error} title={t('agentChat.reader.loadFailed')} />
              </div>
            ) : loading ? (
              <div role="status" className="flex h-full items-center justify-center text-xs text-[var(--nova-text-faint)]">
                {t('router.loading')}
              </div>
            ) : selectedPath ? (
              <MarkdownEditor
                key={selectedPath}
                workspace={workspace}
                fileName={selectedPath}
                content={document.content}
                revision={document.revision}
                chapterSummary={selectedChapter}
                autoSaveEnabled={Boolean(onSaveFile)}
                onSave={save}
                onFlushHandlerChange={handleFlushHandlerChange}
                documentReview={documentReview}
                documentReviewNavigationIntent={navigationPath === selectedPath ? navigationIntent : null}
                onOpenOutline={isMobile ? openLeft : undefined}
              />
            ) : (
              <EmptyState variant="page" icon={BookOpen} title={t('agentChat.reader.noSelection')} />
            )}
          </div>
        )}
      </AdaptiveSurface>
    </section>
  )
}
