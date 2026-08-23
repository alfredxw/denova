import {
  prepareFileTreeInput,
  preparePresortedFileTreeInput,
  type FileTree as FileTreeModel,
  type ContextMenuItem,
  type ContextMenuOpenContext,
  type FileTreeDragAndDropConfig,
  type ContextMenuTriggerMode,
  type FileTreeInitialExpansion,
  type FileTreeRenamingConfig,
  type FileTreeRowDecorationRenderer,
  type GitStatusEntry,
} from '@pierre/trees'
import { FileTree, useFileTree } from '@pierre/trees/react'
import { useTheme } from 'next-themes'
import type { KeyboardEventHandler, ReactNode } from 'react'
import { forwardRef, useEffect, useImperativeHandle, useLayoutEffect, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { expandedFileTreePathsForReset } from './paths'
import './NovaFileTree.css'

const NOVA_FILE_TREE_UNSAFE_CSS = `
[data-type="item"][data-item-focused="true"]:not(:focus-visible)::before {
  outline: none;
}

[data-type="item"]:not([data-item-git-status]):not([data-item-contains-git-change="true"]) > [data-item-section="git"] {
  display: none;
}

[data-type="item"][data-item-context-menu-button-visibility="when-needed"]:not(:hover):not(:focus-visible):not([data-item-context-hover="true"]) > [data-item-section="action"] {
  display: none;
}
`

const COLOR_ONLY_GIT_STATUS_UNSAFE_CSS = `
[data-type="item"] > [data-item-section="git"] {
  display: none;
}
`

export interface NovaFileTreeProps {
  paths: readonly string[]
  presorted?: boolean
  ariaLabel: string
  searchLabel: string
  className?: string
  initialExpansion?: FileTreeInitialExpansion
  initialExpandedPaths?: readonly string[]
  selectedPaths?: readonly string[]
  gitStatus?: readonly GitStatusEntry[]
  gitStatusPresentation?: 'lane' | 'color-only'
  dragAndDrop?: false | FileTreeDragAndDropConfig
  renaming?: false | FileTreeRenamingConfig
  renderRowDecoration?: FileTreeRowDecorationRenderer
  renderContextMenu?: (item: ContextMenuItem, context: ContextMenuOpenContext) => ReactNode
  contextMenuTriggerMode?: ContextMenuTriggerMode
  onSelectionChange?: (paths: readonly string[]) => void
  onDirectoryExpandedChange?: (path: string, expanded: boolean) => void
  onDirectoryExpand?: (path: string) => void | Promise<void>
  onScrollOffsetChange?: (offset: number) => void
  onKeyDownCapture?: KeyboardEventHandler<HTMLElement>
}

/**
 * Denova's shared Pierre surface. Business state remains authoritative in each
 * consumer while this component owns common options and model synchronization.
 */
export const NovaFileTree = forwardRef<FileTreeModel, NovaFileTreeProps>(function NovaFileTree({
  paths,
  presorted = false,
  ariaLabel,
  searchLabel,
  className,
  initialExpansion = 'closed',
  initialExpandedPaths = [],
  selectedPaths = [],
  gitStatus = [],
  gitStatusPresentation = 'lane',
  dragAndDrop = false,
  renaming = false,
  renderRowDecoration,
  renderContextMenu,
  contextMenuTriggerMode = 'both',
  onSelectionChange,
  onDirectoryExpandedChange,
  onDirectoryExpand,
  onScrollOffsetChange,
  onKeyDownCapture,
}, ref) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const preparedInput = useMemo(() => (
    presorted ? preparePresortedFileTreeInput(paths) : prepareFileTreeInput(paths)
  ), [paths, presorted])
  const live = useRef({ dragAndDrop, renaming, renderRowDecoration, onSelectionChange, onDirectoryExpandedChange, onDirectoryExpand })
  live.current = { dragAndDrop, renaming, renderRowDecoration, onSelectionChange, onDirectoryExpandedChange, onDirectoryExpand }
  const syncingSelectionRef = useRef(false)
  const syncingPathsRef = useRef(false)
  const expandedStateRef = useRef(new Map<string, boolean>())
  const initialPreparedInputRef = useRef(preparedInput)

  const { model } = useFileTree({
    preparedInput,
    density: 'default',
    flattenEmptyDirectories: true,
    initialExpansion,
    initialExpandedPaths,
    initialSelectedPaths: selectedPaths,
    fileTreeSearchMode: 'hide-non-matches',
    icons: { set: 'complete', colored: true },
    search: true,
    searchBlurBehavior: 'retain',
    stickyFolders: true,
    unsafeCSS: gitStatusPresentation === 'color-only'
      ? `${NOVA_FILE_TREE_UNSAFE_CSS}${COLOR_ONLY_GIT_STATUS_UNSAFE_CSS}`
      : NOVA_FILE_TREE_UNSAFE_CSS,
    composition: renderContextMenu ? {
      contextMenu: {
        enabled: true,
        triggerMode: contextMenuTriggerMode,
        buttonVisibility: 'when-needed',
      },
    } : undefined,
    dragAndDrop: dragAndDrop ? {
      canDrag: (draggedPaths) => resolveDragConfig(live.current.dragAndDrop)?.canDrag?.(draggedPaths) ?? true,
      canDrop: (event) => resolveDragConfig(live.current.dragAndDrop)?.canDrop?.(event) ?? true,
      onDropComplete: (event) => resolveDragConfig(live.current.dragAndDrop)?.onDropComplete?.(event),
      onDropError: (error, event) => resolveDragConfig(live.current.dragAndDrop)?.onDropError?.(error, event),
    } : false,
    renaming: renaming ? {
      canRename: (item) => resolveRenamingConfig(live.current.renaming)?.canRename?.(item) ?? true,
      onRename: (event) => resolveRenamingConfig(live.current.renaming)?.onRename?.(event),
      onError: (error) => resolveRenamingConfig(live.current.renaming)?.onError?.(error),
    } : false,
    renderRowDecoration: (context) => live.current.renderRowDecoration?.(context) ?? null,
    onSelectionChange: (nextPaths) => {
      if (!syncingSelectionRef.current) live.current.onSelectionChange?.(nextPaths)
    },
  })
  useImperativeHandle(ref, () => model, [model])

  useLayoutEffect(() => {
    if (initialPreparedInputRef.current === preparedInput) return
    initialPreparedInputRef.current = preparedInput
    syncingPathsRef.current = true
    try {
      expandedStateRef.current.clear()
      model.resetPaths({
        preparedInput,
        initialExpandedPaths: expandedFileTreePathsForReset(paths, initialExpansion, initialExpandedPaths),
      })
      rememberExpansion(model, expandedStateRef.current)
    } finally {
      syncingPathsRef.current = false
    }
  }, [initialExpandedPaths, initialExpansion, model, paths, preparedInput])

  useLayoutEffect(() => {
    model.setGitStatus(gitStatus)
  }, [gitStatus, model])

  useLayoutEffect(() => {
    syncingSelectionRef.current = true
    try {
      const desired = new Set(selectedPaths)
      for (const path of model.getSelectedPaths()) {
        if (!desired.has(path)) model.getItem(path)?.deselect()
      }
      for (const path of desired) {
        const item = model.getItem(path)
        if (item && !item.isSelected()) item.select()
      }
    } finally {
      syncingSelectionRef.current = false
    }
  }, [model, selectedPaths])

  useEffect(() => {
    rememberExpansion(model, expandedStateRef.current)
    return model.subscribe(() => {
      if (syncingPathsRef.current || model.isSearchOpen()) return
      const callbacks = live.current
      for (const row of model.getVisibleRows(0, Math.max(0, model.getVisibleCount() - 1))) {
        if (row.kind !== 'directory') continue
        const previous = expandedStateRef.current.get(row.path)
        expandedStateRef.current.set(row.path, row.isExpanded)
        if (previous === undefined || previous === row.isExpanded) continue
        callbacks.onDirectoryExpandedChange?.(row.path, row.isExpanded)
        if (row.isExpanded) {
          void Promise.resolve(callbacks.onDirectoryExpand?.(row.path)).catch((cause) => {
            console.error('[components/file-tree/NovaFileTree.tsx] expanding a directory failed', { path: row.path, cause })
          })
        }
      }
    })
  }, [model])

  useEffect(() => {
    let observer: MutationObserver | null = null
    let retry = 0
    const connect = () => {
      const host = model.getFileTreeContainer()
      if (!host) {
        retry = window.setTimeout(connect, 0)
        return
      }
      host.style.colorScheme = resolvedTheme === 'light' ? 'light' : 'dark'
      const localize = () => localizeTree(host, {
        ariaLabel,
        searchLabel,
        optionsLabel: t('sidebar.moreActions'),
        renameLabel: t('files.tree.renameInput'),
        gitStatusLabels: {
          added: t('files.tree.git.added'),
          deleted: t('files.tree.git.deleted'),
          ignored: t('files.tree.git.ignored'),
          modified: t('files.tree.git.modified'),
          renamed: t('files.tree.git.renamed'),
          untracked: t('files.tree.git.untracked'),
          descendant: t('files.tree.git.descendant'),
        },
      })
      localize()
      observer = new MutationObserver(localize)
      if (host.shadowRoot) observer.observe(host.shadowRoot, { childList: true, subtree: true })
    }
    connect()
    return () => {
      window.clearTimeout(retry)
      observer?.disconnect()
    }
  }, [ariaLabel, model, resolvedTheme, searchLabel, t])

  useEffect(() => {
    if (!onScrollOffsetChange) return
    let scroll: HTMLElement | null = null
    let retry = 0
    const report = () => {
      if (scroll) onScrollOffsetChange(scroll.scrollTop)
    }
    const connect = () => {
      scroll = model.getFileTreeContainer()?.shadowRoot?.querySelector<HTMLElement>('[data-file-tree-virtualized-scroll]') ?? null
      if (!scroll) {
        retry = window.setTimeout(connect, 0)
        return
      }
      scroll.addEventListener('scroll', report, { passive: true })
      report()
    }
    connect()
    return () => {
      window.clearTimeout(retry)
      scroll?.removeEventListener('scroll', report)
    }
  }, [model, onScrollOffsetChange])

  return (
    <FileTree
      model={model}
      aria-label={ariaLabel}
      className={cn('nova-file-tree', className)}
      renderContextMenu={renderContextMenu}
      onKeyDownCapture={onKeyDownCapture}
    />
  )
})

function resolveDragConfig(value: false | FileTreeDragAndDropConfig) {
  return value || null
}

function resolveRenamingConfig(value: false | FileTreeRenamingConfig) {
  return value || null
}

function rememberExpansion(model: FileTreeModel, state: Map<string, boolean>) {
  for (const row of model.getVisibleRows(0, Math.max(0, model.getVisibleCount() - 1))) {
    if (row.kind === 'directory') state.set(row.path, row.isExpanded)
  }
}

interface TreeLabels {
  ariaLabel: string
  searchLabel: string
  optionsLabel: string
  renameLabel: string
  gitStatusLabels: Record<'added' | 'deleted' | 'ignored' | 'modified' | 'renamed' | 'untracked' | 'descendant', string>
}

function localizeTree(host: HTMLElement, labels: TreeLabels) {
  const root = host.shadowRoot
  if (!root) return
  setAttribute(root.querySelector('[role="tree"]'), 'aria-label', labels.ariaLabel)
  const search = root.querySelector<HTMLInputElement>('[data-file-tree-search-input]')
  if (search) {
    if (search.placeholder !== labels.searchLabel) search.placeholder = labels.searchLabel
    setAttribute(search, 'aria-label', labels.searchLabel)
  }
  setAttribute(root.querySelector('[data-type="context-menu-trigger"]'), 'aria-label', labels.optionsLabel)
  for (const input of root.querySelectorAll<HTMLInputElement>('[data-item-rename-input]')) {
    const name = input.closest<HTMLElement>('[data-item-path]')?.dataset.itemPath ?? ''
    setAttribute(input, 'aria-label', labels.renameLabel.replace('{{name}}', name))
  }
  for (const row of root.querySelectorAll<HTMLElement>('[data-item-path]')) {
    const status = row.dataset.itemGitStatus as keyof TreeLabels['gitStatusLabels'] | undefined
    const git = row.querySelector('[data-item-section="git"]')
    if (git) setAttribute(git, 'title', status ? labels.gitStatusLabels[status] : labels.gitStatusLabels.descendant)
  }
}

function setAttribute(element: Element | null, name: string, value: string) {
  if (element?.getAttribute(name) !== value) element?.setAttribute(name, value)
}
