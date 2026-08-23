import { AtSign } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { ProjectExplorerPane } from './ProjectExplorerPane'
import { useProjectExplorerPreferences } from './preferences'
import type { ProjectExplorerExtensions } from './types'
import { useProjectExplorer } from './use-project-explorer'

const WRITING_DEFAULT_EXPANDED_PATHS = ['setting', 'chapters'] as const

interface WritingProjectExplorerProps {
  projectId: string
  workspace: string
  selectedPath: string | null
  structureRefreshSignal: number
  onSelectFile: (path: string) => boolean | void | Promise<boolean | void>
  onReferenceFile: (path: string) => void
  onCreateItem: (path: string, type: 'file' | 'dir') => Promise<void>
  onDeleteItem: (path: string) => Promise<void>
  onRenameItem: (path: string, newName: string) => Promise<void>
  onCopyItem: (from: string, to: string) => Promise<void>
  onMoveItem: (from: string, to: string) => Promise<void>
  onRefreshWorkspace: () => void | Promise<void>
}

/** Adapts Writing-only presentation and workspace lifecycle to the shared project Explorer. */
export function WritingProjectExplorer({
  projectId,
  workspace,
  selectedPath,
  structureRefreshSignal,
  onSelectFile,
  onReferenceFile,
  onCreateItem,
  onDeleteItem,
  onRenameItem,
  onCopyItem,
  onMoveItem,
  onRefreshWorkspace,
}: WritingProjectExplorerProps) {
  const { t } = useTranslation()
  const preferences = useProjectExplorerPreferences(projectId, WRITING_DEFAULT_EXPANDED_PATHS)
  const tree = useProjectExplorer({
    projectId,
    expandedPaths: preferences.preferences.expandedPaths,
    selectedPath,
  })
  const structureRefreshSignalRef = useRef(structureRefreshSignal)

  const surfaceFailure = useCallback((operation: string, cause: unknown) => {
    console.error('[features/project-explorer/WritingProjectExplorer.tsx] writing file operation failed', {
      operation,
      projectId,
      cause,
    })
    toast.error(t('files.operation.failed'), {
      description: cause instanceof Error ? cause.message : String(cause),
    })
  }, [projectId, t])

  const refresh = useCallback(async () => {
    try {
      await Promise.all([tree.refresh(), onRefreshWorkspace()])
    } catch (cause) {
      console.error('[features/project-explorer/WritingProjectExplorer.tsx] refreshing writing project files failed', {
        projectId,
        cause,
      })
      toast.error(t('files.tree.refreshFailed'), {
        description: cause instanceof Error ? cause.message : String(cause),
      })
    }
  }, [onRefreshWorkspace, projectId, t, tree.refresh])

  useEffect(() => {
    if (structureRefreshSignalRef.current === structureRefreshSignal) return
    structureRefreshSignalRef.current = structureRefreshSignal
    void tree.refresh().catch((cause) => {
      console.error('[features/project-explorer/WritingProjectExplorer.tsx] synchronizing writing project files failed', {
        projectId,
        cause,
      })
    })
  }, [projectId, structureRefreshSignal, tree.refresh])

  const runMutation = useCallback(async (operation: string, mutate: () => Promise<void>) => {
    try {
      await mutate()
      await tree.refresh()
    } catch (cause) {
      surfaceFailure(operation, cause)
      throw cause
    }
  }, [surfaceFailure, tree.refresh])

  const createItem = useCallback(async (path: string, type: 'file' | 'dir') => {
    await runMutation('create', () => onCreateItem(path, type))
    if (type === 'file') await onSelectFile(path)
  }, [onCreateItem, onSelectFile, runMutation])
  const deleteItem = useCallback(async (path: string) => {
    await runMutation('delete', () => onDeleteItem(path))
    preferences.removeBranch(path)
  }, [onDeleteItem, preferences.removeBranch, runMutation])
  const renameItem = useCallback(async (path: string, newName: string) => {
    await runMutation('rename', () => onRenameItem(path, newName))
    preferences.relocateBranch(path, renamedPath(path, newName))
  }, [onRenameItem, preferences.relocateBranch, runMutation])
  const copyItem = useCallback((from: string, to: string) => (
    runMutation('copy', () => onCopyItem(from, to))
  ), [onCopyItem, runMutation])
  const moveItem = useCallback(async (from: string, to: string) => {
    await runMutation('move', () => onMoveItem(from, to))
    preferences.relocateBranch(from, to)
  }, [onMoveItem, preferences.relocateBranch, runMutation])

  const extensions = useMemo<ProjectExplorerExtensions>(() => ({
    deleteRecovery: 'version-history',
    getNodeActions: ({ node, paths }) => node.type === 'file' && paths.length === 1 ? [{
      id: 'reference-to-chat',
      label: t('sidebar.referenceToChat'),
      icon: <AtSign className="size-3.5" />,
      onSelect: () => onReferenceFile(node.path),
    }] : [],
  }), [onReferenceFile, t])

  return (
    <ProjectExplorerPane
      projectId={projectId}
      nodes={tree.nodes}
      workspace={workspace}
      selectedPath={selectedPath}
      expandedPaths={preferences.preferences.expandedPaths}
      gitStatus={tree.gitStatus}
      loading={tree.loading}
      loadingPaths={tree.loadingPaths}
      error={tree.error}
      onSelectFile={(path) => { void onSelectFile(path) }}
      onDirectoryExpand={tree.loadDirectory}
      onDirectoryExpandedChange={preferences.setDirectoryExpanded}
      onCollapseAll={preferences.collapseAll}
      onLoadMore={tree.loadMore}
      onCreateItem={createItem}
      onDeleteItem={deleteItem}
      onRenameItem={renameItem}
      onCopyItem={copyItem}
      onMoveItem={moveItem}
      onRefresh={refresh}
      extensions={extensions}
    />
  )
}

function renamedPath(path: string, newName: string) {
  const separator = path.lastIndexOf('/')
  return separator < 0 ? newName : `${path.slice(0, separator)}/${newName}`
}
