import { Copy, ExternalLink } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { FileTreeMenu, FileTreeMenuItem, FileTreeMenuSeparator } from '@/components/file-tree/FileTreeMenu'
import { NovaFileTree } from '@/components/file-tree/NovaFileTree'
import { applicationFileTreePath, canonicalFileTreePath, writeClipboardText } from '@/components/file-tree/paths'
import type { FileNode } from '@/hooks/useWorkspace'

interface SkillFileTreeProps {
  nodes: readonly FileNode[]
  selectedFile: string
  defaultExpandedPaths?: readonly string[]
  onSelectFile: (path: string) => void
}

const fileNameCollator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

/** Read-only Pierre tree for browsing the files that belong to one Skill. */
export function SkillFileTree({
  nodes,
  selectedFile,
  defaultExpandedPaths = [],
  onSelectFile,
}: SkillFileTreeProps) {
  const { t } = useTranslation()
  const projection = useMemo(() => projectSkillFiles(nodes), [nodes])
  const expanded = useMemo(() => defaultExpandedPaths.map((path) => canonicalFileTreePath(path, true)), [defaultExpandedPaths])
  const selected = useMemo(() => selectedFile ? [selectedFile] : [], [selectedFile])

  return (
    <NovaFileTree
      paths={projection.paths}
      presorted
      ariaLabel={t('skills.files.title')}
      searchLabel={t('files.tree.search')}
      initialExpandedPaths={expanded}
      selectedPaths={selected}
      onSelectionChange={(paths) => {
        const path = applicationFileTreePath(paths.at(-1) ?? '')
        if (projection.files.has(path)) onSelectFile(path)
      }}
      renderContextMenu={(item, context) => {
        const path = applicationFileTreePath(item.path)
        const file = projection.files.has(path)
        const closeThen = (action: () => void) => {
          context.close()
          action()
        }
        return (
          <FileTreeMenu anchorRect={context.anchorRect}>
            {file ? (
              <>
                <FileTreeMenuItem onClick={() => closeThen(() => onSelectFile(path))}><ExternalLink />{t('changes.openFile')}</FileTreeMenuItem>
                <FileTreeMenuSeparator />
              </>
            ) : null}
            <FileTreeMenuItem onClick={() => closeThen(() => {
              void writeClipboardText(path).catch((cause) => {
                console.error('[features/skills/SkillFileTree.tsx] copying Skill file path failed', { path, cause })
                toast.error(t('files.tree.copyPathFailed'))
              })
            })}><Copy />{t('sidebar.copyRelativePath')}</FileTreeMenuItem>
          </FileTreeMenu>
        )
      }}
    />
  )
}

function projectSkillFiles(nodes: readonly FileNode[]) {
  const paths: string[] = []
  const files = new Set<string>()
  const visit = (items: readonly FileNode[], parent = '') => {
    for (const node of sortNodes(items)) {
      const path = parent ? `${parent}/${node.name}` : node.name
      paths.push(canonicalFileTreePath(path, node.type === 'dir'))
      if (node.type === 'dir') visit(node.children ?? [], path)
      else files.add(path)
    }
  }
  visit(nodes)
  return { paths, files }
}

function sortNodes(nodes: readonly FileNode[]) {
  return [...nodes].sort((left, right) => {
    if (left.type !== right.type) return left.type === 'dir' ? -1 : 1
    return fileNameCollator.compare(left.name, right.name)
  })
}
