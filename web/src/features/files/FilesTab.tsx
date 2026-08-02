import { FileCode2, FileWarning, Loader2, PanelRightClose, PanelRightOpen, Save } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { EmptyState } from '@/components/common/EmptyState'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import { AutosaveStatusIndicator } from '@/components/forms/autosave-status'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { Button } from '@/components/ui/button'
import { ProjectFilesSidebar } from './ProjectFilesSidebar'
import { ProjectSourceEditor } from './ProjectSourceEditor'
import {
  persistProjectFilesPreferences,
  readProjectFilesPreferences,
  type ProjectFilesPreferences,
} from './preferences'
import { useProjectFileEditor } from './use-project-file-editor'
import { useProjectFileTree } from './use-project-file-tree'

interface FilesTabProps {
  projectId: string
  workspace: string
  selectedPath: string | null
  autoSaveEnabled: boolean
  autoSaveDelayMs: number
  refreshSignal?: number
  onSelectedPathChange: (path: string | null) => void
  onFlushHandlerChange?: (handler: EditorFlushHandler | null) => void
  onWorkspaceChanged?: (workspace: string, paths: string[]) => void | Promise<void>
}

/** A general project tab: Monaco on the left, an adaptive project file tree on the right. */
export function FilesTab({
  projectId,
  workspace,
  selectedPath,
  autoSaveEnabled,
  autoSaveDelayMs,
  refreshSignal = 0,
  onSelectedPathChange,
  onFlushHandlerChange,
  onWorkspaceChanged,
}: FilesTabProps) {
  const { t } = useTranslation()
  const [preferences, setPreferences] = useState<ProjectFilesPreferences>(() => readProjectFilesPreferences(projectId))
  const tree = useProjectFileTree({ projectId, includeIgnored: preferences.showIgnored })
  const editor = useProjectFileEditor({
    projectId,
    selectedPath,
    autoSaveEnabled,
    autoSaveDelayMs,
    onSaved: (path) => onWorkspaceChanged?.(workspace, [path]),
  })
  const refreshSignalRef = useRef(refreshSignal)

  useEffect(() => {
    persistProjectFilesPreferences(projectId, preferences)
  }, [preferences, projectId])

  useEffect(() => {
    onFlushHandlerChange?.(editor.flush)
    return () => onFlushHandlerChange?.(null)
  }, [editor.flush, onFlushHandlerChange])

  const refresh = useCallback(async () => {
    try {
      await Promise.all([tree.refresh(), editor.reload()])
    } catch (cause) {
      console.error('[features/files/FilesTab.tsx] refreshing files failed', { projectId, cause })
      toast.error(t('files.tree.refreshFailed'), {
        description: cause instanceof Error ? cause.message : String(cause),
      })
    }
  }, [editor.reload, projectId, t, tree.refresh])

  useEffect(() => {
    if (refreshSignalRef.current === refreshSignal) return
    refreshSignalRef.current = refreshSignal
    void refresh()
  }, [refresh, refreshSignal])

  const selectFile = useCallback(async (path: string) => {
    if (path === selectedPath) return
    if (!await editor.flush()) return
    onSelectedPathChange(path)
  }, [editor.flush, onSelectedPathChange, selectedPath])

  const runOperation = useCallback(async <Result,>(paths: string[], operation: () => Promise<Result>): Promise<Result> => {
    try {
      const result = await operation()
      await onWorkspaceChanged?.(workspace, paths)
      return result
    } catch (cause) {
      console.error('[features/files/FilesTab.tsx] project file operation failed', { projectId, paths, cause })
      toast.error(t('files.operation.failed'), {
        description: cause instanceof Error ? cause.message : String(cause),
      })
      throw cause
    }
  }, [onWorkspaceChanged, projectId, t, workspace])

  const createItem = useCallback(async (path: string, type: 'file' | 'dir') => {
    await runOperation([path], () => tree.createItem(path, type))
    if (type === 'file') onSelectedPathChange(path)
  }, [onSelectedPathChange, runOperation, tree.createItem])

  const deleteItem = useCallback(async (path: string) => {
    if (selectedPath === path || selectedPath?.startsWith(`${path}/`)) {
      if (!await editor.flush()) throw new Error(t('files.operation.failed'))
    }
    await runOperation([path], () => tree.deleteItem(path))
    if (selectedPath === path || selectedPath?.startsWith(`${path}/`)) onSelectedPathChange(null)
  }, [editor.flush, onSelectedPathChange, runOperation, selectedPath, t, tree.deleteItem])

  const renameItem = useCallback(async (path: string, newName: string) => {
    if (selectedPath === path || selectedPath?.startsWith(`${path}/`)) {
      if (!await editor.flush()) throw new Error(t('files.operation.failed'))
    }
    const renamedPath = await runOperation([path], () => tree.renameItem(path, newName))
    if (selectedPath === path) onSelectedPathChange(renamedPath)
    else if (selectedPath?.startsWith(`${path}/`)) onSelectedPathChange(`${renamedPath}${selectedPath.slice(path.length)}`)
  }, [editor.flush, onSelectedPathChange, runOperation, selectedPath, t, tree.renameItem])

  const copyItem = useCallback(async (from: string, to: string) => {
    await runOperation([from, to], () => tree.copyItem(from, to))
  }, [runOperation, tree.copyItem])

  const moveItem = useCallback(async (from: string, to: string) => {
    if (selectedPath === from || selectedPath?.startsWith(`${from}/`)) {
      if (!await editor.flush()) throw new Error(t('files.operation.failed'))
    }
    await runOperation([from, to], () => tree.moveItem(from, to))
    if (selectedPath === from) onSelectedPathChange(to)
    else if (selectedPath?.startsWith(`${from}/`)) onSelectedPathChange(`${to}${selectedPath.slice(from.length)}`)
  }, [editor.flush, onSelectedPathChange, runOperation, selectedPath, t, tree.moveItem])

  const setTreeVisible = useCallback((treeVisible: boolean) => {
    setPreferences((current) => ({ ...current, treeVisible }))
  }, [])
  const setShowIgnored = useCallback((showIgnored: boolean) => {
    setPreferences((current) => ({ ...current, showIgnored }))
  }, [])
  const setDirectoryExpanded = useCallback((path: string, expanded: boolean) => {
    setPreferences((current) => {
      const paths = new Set(current.expandedPaths)
      if (expanded) paths.add(path)
      else paths.delete(path)
      return { ...current, expandedPaths: [...paths] }
    })
  }, [])

  const sidebar = (
    <ProjectFilesSidebar
      nodes={tree.nodes}
      selectedPath={selectedPath}
      expandedPaths={preferences.expandedPaths}
      loading={tree.loading}
      loadingPaths={tree.loadingPaths}
      error={tree.error}
      showIgnored={preferences.showIgnored}
      onShowIgnoredChange={setShowIgnored}
      onSelectFile={(path) => void selectFile(path)}
      onDirectoryExpand={tree.loadDirectory}
      onDirectoryExpandedChange={setDirectoryExpanded}
      onCreateItem={createItem}
      onDeleteItem={deleteItem}
      onRenameItem={renameItem}
      onCopyItem={copyItem}
      onMoveItem={moveItem}
      onRefresh={refresh}
    />
  )

  return (
    <AdaptiveSurface
      className="h-full min-h-0"
      collapseAt={700}
      mobilePaneScope="surface"
      rightResize={{
        layoutKey: `nova-project-files-layout:v1:${projectId}`,
        label: t('layout.resize.right'),
        defaultSize: '280px',
        minSize: '210px',
        maxSize: '45%',
        mainMinSize: '300px',
      }}
      right={{
        id: `project-files-tree-${projectId}`,
        side: 'right',
        title: t('files.tree.title'),
        content: sidebar,
        desktopVisible: preferences.treeVisible,
        desktopClassName: 'h-full min-h-0 min-w-0 border-l border-[var(--nova-border)]',
        mobileClassName: 'w-[min(88vw,360px)]',
        onOpen: () => setTreeVisible(true),
        onClose: () => setTreeVisible(false),
      }}
    >
      {(controls) => (
        <main className="flex h-full min-h-0 min-w-0 flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
          <div className="flex h-11 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 sm:px-3">
            <FileCode2 className="size-4 shrink-0 text-[var(--nova-text-muted)]" aria-hidden="true" />
            <div className="min-w-0 flex-1">
              <div className="truncate text-xs font-medium">{selectedPath?.split('/').at(-1) || t('files.title')}</div>
              <div className="truncate font-mono text-[10px] text-[var(--nova-text-faint)]" title={selectedPath || workspace}>
                {selectedPath || workspace}
              </div>
            </div>
            {editor.document?.kind === 'text' && editor.document.editable ? (
              <AutosaveStatusIndicator status={editor.status} error={editor.autoSaveError} onRetry={editor.retry} />
            ) : editor.document ? (
              <span className="hidden shrink-0 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)] sm:inline">
                {t('files.editor.readOnly')}
              </span>
            ) : null}
            {editor.document?.kind === 'text' && editor.document.editable ? (
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                disabled={!editor.dirty || editor.status === 'saving'}
                onClick={() => void editor.flush(true)}
                aria-label={t('files.editor.save')}
                title={t('files.editor.save')}
                className="text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
              >
                <Save />
              </Button>
            ) : null}
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              onClick={() => {
                if (controls.isMobile) controls.openRight()
                else setTreeVisible(!preferences.treeVisible)
              }}
              aria-label={t(preferences.treeVisible && !controls.isMobile ? 'files.tree.hide' : 'files.tree.open')}
              title={t(preferences.treeVisible && !controls.isMobile ? 'files.tree.hide' : 'files.tree.open')}
              className="text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
            >
              {preferences.treeVisible && !controls.isMobile ? <PanelRightClose /> : <PanelRightOpen />}
            </Button>
          </div>
          <div className="flex min-h-0 min-w-0 flex-1 flex-col">
            {editor.loading ? (
              <div className="flex h-full items-center justify-center gap-2 text-xs text-[var(--nova-text-faint)]">
                <Loader2 className="size-4 animate-spin" />
                {t('files.editor.loading')}
              </div>
            ) : editor.error ? (
              <EmptyState
                variant="page"
                icon={FileWarning}
                title={t('files.editor.loadFailed')}
                description={editor.error}
                action={{ label: t('common.retry'), onClick: () => void editor.reload() }}
              />
            ) : editor.document ? (
              <ProjectSourceEditor
                projectId={projectId}
                document={editor.document}
                value={editor.draft}
                onChange={editor.setDraft}
                onSave={() => void editor.flush(true)}
              />
            ) : (
              <EmptyState
                variant="page"
                icon={FileCode2}
                title={t('files.editor.noSelection')}
                description={t('files.editor.noSelectionDescription')}
              />
            )}
          </div>
        </main>
      )}
    </AdaptiveSurface>
  )
}
