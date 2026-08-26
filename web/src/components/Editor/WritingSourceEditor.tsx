import { Save, WrapText } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { ProjectFileSnapshotBreadcrumb } from '@/features/files/ProjectFileBreadcrumb'
import { ProjectTextEditor, type ProjectTextEditorHandle } from '@/features/files/ProjectSourceEditor'
import type { ProjectFileExplorerNode } from '@/features/project-explorer/model'
import {
  persistProjectFileEditorPreferences,
  readProjectFileEditorPreferences,
} from '@/features/files/preferences'
import type { TextSelection } from '@/lib/api'
import type { ProjectFileDocument } from '@/lib/api-client/project-files'
import { EditorPersistenceNotices } from './EditorPersistenceNotices'
import { EditorSaveStatus } from './EditorSaveStatus'
import {
  useEditorDraftPersistence,
  type EditorDraftAdapter,
  type EditorFlushHandler,
} from './useEditorDraftPersistence'

interface WritingSourceEditorProps {
  projectId: string
  workspace: string
  fileTree: readonly ProjectFileExplorerNode[]
  document: ProjectFileDocument
  onSelectFile: (path: string) => unknown
  onSave: (fileName: string, content: string, baseRevision: string) => Promise<boolean | { revision?: string }>
  onQuoteSelection?: (selection: TextSelection) => void
  saveSignal?: number
  autoSaveEnabled?: boolean
  autoSaveDelayMs?: number
  searchIntent?: { query: string; line: number; nonce: number } | null
  onFlushHandlerChange?: (handler: EditorFlushHandler | null) => void
}

/** Monaco source editor that shares Writing's autosave, flush, and conflict semantics. */
export function WritingSourceEditor({
  projectId,
  workspace,
  fileTree,
  document,
  onSelectFile,
  onSave,
  onQuoteSelection,
  saveSignal = 0,
  autoSaveEnabled = true,
  autoSaveDelayMs,
  searchIntent,
  onFlushHandlerChange,
}: WritingSourceEditorProps) {
  const { t } = useTranslation()
  const editorRef = useRef<ProjectTextEditorHandle>(null)
  const editorContainerRef = useRef<HTMLDivElement>(null)
  const updateListenersRef = useRef(new Set<() => void>())
  const activePathRef = useRef(document.path)
  const contentRef = useRef(document.content ?? '')
  const lastSearchIntentNonceRef = useRef<number | null>(null)
  const [wordWrap, setWordWrap] = useState(() => readProjectFileEditorPreferences().wordWrap)

  if (activePathRef.current !== document.path) {
    activePathRef.current = document.path
    contentRef.current = document.content ?? ''
  }

  const draftAdapter = useMemo<EditorDraftAdapter>(() => ({
    // The fallback snapshot is valid before Monaco mounts, so persistence can
    // establish the initial revision without waiting for an editor callback.
    isAvailable: () => true,
    readText: () => editorRef.current?.getValue() ?? contentRef.current,
    subscribe: (onUpdate) => {
      updateListenersRef.current.add(onUpdate)
      return () => updateListenersRef.current.delete(onUpdate)
    },
  }), [])

  const applyExternalContent = useCallback((_fileName: string | null, content: string) => {
    contentRef.current = content
    editorRef.current?.replaceValue(content)
  }, [])

  const {
    saveStatus,
    externalConflict,
    externalConflictSaving,
    handleSave,
    keepLocalVersion,
    loadExternalVersion,
  } = useEditorDraftPersistence({
    workspace: projectId,
    fileName: document.path,
    content: document.content ?? '',
    revision: document.revision,
    editor: draftAdapter,
    editorContainerRef,
    onSave,
    saveSignal,
    autoSaveEnabled: autoSaveEnabled && document.editable,
    autoSaveDelayMs,
    applyExternalContent,
    onFlushHandlerChange,
  })

  useEffect(() => {
    if (!searchIntent || lastSearchIntentNonceRef.current === searchIntent.nonce) return
    lastSearchIntentNonceRef.current = searchIntent.nonce
    editorRef.current?.revealLine(searchIntent.line)
  }, [searchIntent])

  const handleChange = useCallback((content: string) => {
    contentRef.current = content
    updateListenersRef.current.forEach((listener) => listener())
  }, [])

  const toggleWordWrap = useCallback(() => {
    setWordWrap((current) => {
      const next = !current
      persistProjectFileEditorPreferences({ wordWrap: next })
      return next
    })
  }, [])

  return (
    <div className="flex min-h-0 flex-1 flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <div className="nova-editor-toolbar flex h-9 shrink-0 items-center justify-between gap-3 overflow-hidden border-b px-3">
        <ProjectFileSnapshotBreadcrumb
          workspace={workspace}
          nodes={fileTree}
          selectedPath={document.path}
          onSelectFile={onSelectFile}
        />
        <div className="flex shrink-0 items-center gap-1">
          <EditorSaveStatus status={saveStatus} />
          <TooltipIconButton
            label={t(wordWrap ? 'files.editor.disableWordWrap' : 'files.editor.enableWordWrap')}
            size="icon-xs"
            tooltipSide="bottom"
            className={wordWrap ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'}
            aria-pressed={wordWrap}
            onClick={toggleWordWrap}
          >
            <WrapText className="h-3.5 w-3.5" />
          </TooltipIconButton>
          {document.editable ? (
            <TooltipIconButton
              label={t('editor.save')}
              size="icon-xs"
              tooltipSide="bottom"
              className="text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
              onClick={() => void handleSave()}
            >
              <Save className="h-3.5 w-3.5" />
            </TooltipIconButton>
          ) : (
            <span className="rounded border border-[var(--nova-border)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">
              {t('files.editor.readOnly')}
            </span>
          )}
        </div>
      </div>
      <EditorPersistenceNotices
        workspace={projectId}
        fileName={document.path}
        revision={document.revision}
        externalConflict={externalConflict}
        externalConflictSaving={externalConflictSaving}
        onKeepLocal={keepLocalVersion}
        onLoadExternal={loadExternalVersion}
      />
      <div ref={editorContainerRef} className="min-h-0 flex-1">
        <ProjectTextEditor
          ref={editorRef}
          projectId={projectId}
          document={document}
          value={document.content ?? ''}
          wordWrap={wordWrap}
          onWordWrapToggle={toggleWordWrap}
          onChange={handleChange}
          onSave={() => void handleSave()}
          onQuoteSelection={onQuoteSelection ? (selection) => onQuoteSelection({ fileName: document.path, ...selection }) : undefined}
          syncExternalValue={false}
        />
      </div>
    </div>
  )
}
