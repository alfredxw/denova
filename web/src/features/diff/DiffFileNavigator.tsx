import { useEffect, useRef } from 'react'
import { FileTree, type FileTreeRowDecoration } from '@pierre/trees'
import { useTheme } from 'next-themes'
import { useTranslation } from 'react-i18next'
import type { DiffFileNavigationItem } from './types'

interface DiffFileNavigatorProps {
  files: readonly DiffFileNavigationItem[]
  selectedPath: string
  onSelect: (path: string) => void
}

/** Pierre's path-first tree, bridged to the active item in the continuous CodeView. */
export function DiffFileNavigator({ files, selectedPath, onSelect }: DiffFileNavigatorProps) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const filterFilesLabel = t('changes.filterFiles')
  const containerRef = useRef<HTMLDivElement | null>(null)
  const treeRef = useRef<FileTree | null>(null)
  const onSelectRef = useRef(onSelect)
  const selectedPathRef = useRef(selectedPath)
  const syncingSelectionRef = useRef(false)
  onSelectRef.current = onSelect
  selectedPathRef.current = selectedPath

  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    const fileByPath = new Map(files.map((file) => [file.path, file]))
    const tree = new FileTree({
      paths: files.map((file) => file.path),
      gitStatus: files.map((file) => ({ path: file.path, status: file.kind })),
      density: 'compact',
      flattenEmptyDirectories: true,
      initialExpansion: 'open',
      fileTreeSearchMode: 'hide-non-matches',
      icons: { set: 'complete', colored: true },
      search: true,
      searchBlurBehavior: 'retain',
      renderRowDecoration: ({ item }): FileTreeRowDecoration | null => {
        const file = fileByPath.get(item.path)
        if (!file) return null
        const parts: { text: string; color: string }[] = []
        if (file.additions) parts.push({ text: `+${file.additions}`, color: 'light-dark(#0f9d6b, #34d399)' })
        if (file.deletions) parts.push({ text: `${parts.length ? '\u00a0' : ''}−${file.deletions}`, color: 'light-dark(#dc2626, #f87171)' })
        return parts.length ? { text: parts.map((part) => part.text).join(''), title: `+${file.additions ?? 0} −${file.deletions ?? 0}`, parts } : null
      },
      onSelectionChange: (paths) => {
        if (syncingSelectionRef.current) return
        const path = paths.findLast((candidate) => fileByPath.has(candidate))
        if (path) {
          onSelectRef.current(path)
          return
        }
        const currentTree = treeRef.current
        const activeItem = currentTree?.getItem(selectedPathRef.current)
        if (!currentTree || !activeItem) return
        syncingSelectionRef.current = true
        for (const selected of currentTree.getSelectedPaths()) currentTree.getItem(selected)?.deselect()
        activeItem.select()
        syncingSelectionRef.current = false
      },
    })
    treeRef.current = tree
    container.replaceChildren()
    tree.render({ fileTreeContainer: container })
    const searchInput = container.shadowRoot?.querySelector<HTMLInputElement>('[data-file-tree-search-input]')
    if (searchInput) {
      searchInput.placeholder = filterFilesLabel
      searchInput.setAttribute('aria-label', filterFilesLabel)
    }

    return () => {
      tree.cleanUp()
      treeRef.current = null
    }
  }, [files, filterFilesLabel])

  useEffect(() => {
    if (containerRef.current) containerRef.current.style.colorScheme = resolvedTheme === 'light' ? 'light' : 'dark'
  }, [resolvedTheme])

  useEffect(() => {
    const tree = treeRef.current
    const item = tree?.getItem(selectedPath)
    if (!tree || !item) return
    syncingSelectionRef.current = true
    try {
      for (const path of tree.getSelectedPaths()) {
        if (path !== selectedPath) tree.getItem(path)?.deselect()
      }
      if (!item.isSelected()) item.select()
      tree.scrollToPath(selectedPath, { focus: false })
    } finally {
      syncingSelectionRef.current = false
    }
  }, [files, selectedPath])

  return (
    <aside data-review-file-navigator role="complementary" aria-label={t('changes.fileNavigator')} className="nova-review-file-navigator">
      <div ref={containerRef} className="nova-pierre-tree" />
    </aside>
  )
}
