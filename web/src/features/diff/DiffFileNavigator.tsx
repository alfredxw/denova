import type { FileTreeRowDecoration } from '@pierre/trees'
import { Copy, ExternalLink } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { FileTreeMenu, FileTreeMenuItem, FileTreeMenuSeparator } from '@/components/file-tree/FileTreeMenu'
import { NovaFileTree } from '@/components/file-tree/NovaFileTree'
import { applicationFileTreePath, writeClipboardText } from '@/components/file-tree/paths'
import type { DiffFileNavigationItem } from './types'

interface DiffFileNavigatorProps {
  files: readonly DiffFileNavigationItem[]
  selectedPath: string
  onSelect: (path: string) => void
}

/** Shared Pierre navigator bridged to the active item in the continuous CodeView. */
export function DiffFileNavigator({ files, selectedPath, onSelect }: DiffFileNavigatorProps) {
  const { t } = useTranslation()
  const fileByPath = useMemo(() => new Map(files.map((file) => [file.path, file])), [files])
  const paths = useMemo(() => files.map((file) => file.path), [files])
  const gitStatus = useMemo(() => files.map((file) => ({ path: file.path, status: file.kind })), [files])
  const selectedPaths = useMemo(() => selectedPath ? [selectedPath] : [], [selectedPath])

  return (
    <aside data-review-file-navigator role="complementary" aria-label={t('changes.fileNavigator')} className="nova-review-file-navigator">
      <NovaFileTree
        paths={paths}
        ariaLabel={t('changes.fileNavigator')}
        searchLabel={t('changes.filterFiles')}
        initialExpansion="open"
        selectedPaths={selectedPaths}
        gitStatus={gitStatus}
        renderRowDecoration={({ item }): FileTreeRowDecoration | null => {
          const file = fileByPath.get(item.path)
          if (!file) return null
          const parts: { text: string; color: string }[] = []
          if (file.additions) parts.push({ text: `+${file.additions}`, color: 'light-dark(#0f9d6b, #34d399)' })
          if (file.deletions) parts.push({ text: `${parts.length ? '\u00a0' : ''}−${file.deletions}`, color: 'light-dark(#dc2626, #f87171)' })
          return parts.length ? { text: parts.map((part) => part.text).join(''), title: `+${file.additions ?? 0} −${file.deletions ?? 0}`, parts } : null
        }}
        onSelectionChange={(selected) => {
          const path = applicationFileTreePath(selected.at(-1) ?? '')
          if (fileByPath.has(path)) onSelect(path)
        }}
        renderContextMenu={(item, context) => {
          const path = applicationFileTreePath(item.path)
          if (!fileByPath.has(path)) return null
          const closeThen = (action: () => void) => {
            context.close()
            action()
          }
          return (
            <FileTreeMenu anchorRect={context.anchorRect}>
              <FileTreeMenuItem onClick={() => closeThen(() => onSelect(path))}><ExternalLink />{t('changes.openFile')}</FileTreeMenuItem>
              <FileTreeMenuSeparator />
              <FileTreeMenuItem onClick={() => closeThen(() => {
                void writeClipboardText(path).catch((cause) => {
                  console.error('[features/diff/DiffFileNavigator.tsx] copying diff file path failed', { path, cause })
                  toast.error(t('files.tree.copyPathFailed'))
                })
              })}><Copy />{t('changes.copyPath')}</FileTreeMenuItem>
            </FileTreeMenu>
          )
        }}
      />
    </aside>
  )
}
