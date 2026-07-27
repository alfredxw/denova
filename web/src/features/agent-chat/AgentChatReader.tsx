import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BookOpen, FileText } from 'lucide-react'
import { EmptyState } from '@/components/common/EmptyState'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { MarkdownEditor } from '@/components/Editor/MarkdownEditor'
import { readFile, type ChapterSummary } from '@/lib/api'

/** Same contract as the writing workbench's save handler, so the editor behaves identically. */
export type AgentChatSaveFile = (path: string, content: string, baseRevision: string) => Promise<{ revision?: string }>

interface AgentChatReaderProps {
  workspace: string
  chapters: ChapterSummary[]
  /** Chapter opened first, normally whatever the writing workbench has selected. */
  initialPath?: string | null
  /** Absent means the tab can only read: the editor is mounted, but saving is disabled. */
  onSaveFile?: AgentChatSaveFile
}

/**
 * Chapter reader: the outline plus the writing workbench's own editor.
 *
 * Reusing that editor rather than a second renderer is what makes the tab useful next to a
 * conversation — the same typography, the same autosave and the same commenting the writing
 * page has, so a passage the agent just changed can be fixed or annotated without leaving.
 */
export function AgentChatReader({ workspace, chapters, initialPath, onSaveFile }: AgentChatReaderProps) {
  const { t } = useTranslation()
  const orderedChapters = useMemo(() => [...chapters].sort((a, b) => a.index - b.index), [chapters])
  const [selectedPath, setSelectedPath] = useState(initialPath || orderedChapters[0]?.path || '')
  const [document, setDocument] = useState<{ content: string; revision: string }>({ content: '', revision: '' })
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    setSelectedPath(initialPath || orderedChapters[0]?.path || '')
  }, [initialPath, orderedChapters, workspace])

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
        console.error('[features/agent-chat/AgentChatReader.tsx] reading chapter failed', { path: selectedPath, cause })
        setError(cause instanceof Error ? cause.message : String(cause))
        setDocument({ content: '', revision: '' })
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [selectedPath])

  /**
   * Saves go through the same workspace handler the writing page uses, and the revision it
   * returns is kept so the next save still detects an edit made elsewhere.
   */
  const save = useCallback(async (fileName: string, content: string, baseRevision: string) => {
    if (!onSaveFile) return false
    const result = await onSaveFile(fileName, content, baseRevision)
    if (result?.revision) setDocument((current) => ({ ...current, revision: result.revision as string }))
    return result
  }, [onSaveFile])

  const selectedChapter = orderedChapters.find((chapter) => chapter.path === selectedPath)

  if (!workspace) {
    return <EmptyState variant="page" icon={BookOpen} title={t('agentChat.reader.noWorkspace')} />
  }

  return (
    <div className="grid h-full min-h-0 grid-cols-1 md:grid-cols-[minmax(160px,220px)_minmax(0,1fr)]">
      <nav className="hidden min-h-0 flex-col overflow-y-auto border-r border-[var(--nova-border)] bg-[var(--nova-surface)] p-1.5 md:flex">
        <div className="px-1.5 pb-1 pt-0.5 text-[10px] font-medium uppercase tracking-wide text-[var(--nova-text-faint)]">
          {t('agentChat.reader.outline')}
        </div>
        {orderedChapters.length === 0 ? (
          <p className="px-2 py-3 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('agentChat.reader.noChapters')}</p>
        ) : (
          orderedChapters.map((chapter) => (
            <button
              key={chapter.path}
              type="button"
              onClick={() => setSelectedPath(chapter.path)}
              aria-current={chapter.path === selectedPath ? 'true' : undefined}
              title={chapter.display_title}
              className={`flex items-center gap-1.5 rounded-[var(--nova-radius)] px-1.5 py-1.5 text-left text-xs ${
                chapter.path === selectedPath ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)]'
              }`}
            >
              <FileText className="size-3.5 shrink-0 text-[var(--nova-text-faint)]" />
              <span className="min-w-0 flex-1 truncate">{chapter.display_title}</span>
            </button>
          ))
        )}
      </nav>

      <section className="flex min-h-0 min-w-0 flex-col">
        {error ? (
          <div className="p-3">
            <InlineErrorNotice message={error} title={t('agentChat.reader.loadFailed')} />
          </div>
        ) : loading ? (
          <p className="p-3 text-xs text-[var(--nova-text-faint)]">{t('router.loading')}</p>
        ) : selectedPath ? (
          <MarkdownEditor
            // Remounting per chapter keeps the editor's own document state from carrying over.
            key={selectedPath}
            workspace={workspace}
            fileName={selectedPath}
            content={document.content}
            revision={document.revision}
            chapterSummary={selectedChapter}
            autoSaveEnabled={Boolean(onSaveFile)}
            onSave={save}
          />
        ) : (
          <EmptyState variant="page" icon={BookOpen} title={t('agentChat.reader.noSelection')} />
        )}
      </section>
    </div>
  )
}
