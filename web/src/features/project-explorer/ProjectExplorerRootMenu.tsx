import { ClipboardPaste, FilePlus2, FolderPlus } from 'lucide-react'
import { useRef, type ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import { ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuSeparator, ContextMenuTrigger } from '@/components/ui/context-menu'
import '@/components/file-tree/FileTreeMenu.css'

interface ProjectExplorerRootMenuProps {
  children: ReactElement
  busy: boolean
  canPaste: boolean
  onCreate: (type: 'file' | 'dir') => void
  onPaste: () => void
}

/** Blank space acts on the project root; Pierre retains ownership of row menus. */
export function ProjectExplorerRootMenu({ children, busy, canPaste, onCreate, onPaste }: ProjectExplorerRootMenuProps) {
  const { t } = useTranslation()
  const pendingCreate = useRef<'file' | 'dir' | null>(null)

  return (
    <ContextMenu>
      <ContextMenuTrigger
        asChild
        onContextMenu={(event) => {
          if (event.nativeEvent.composedPath().some((target) => target instanceof HTMLElement && target.hasAttribute('data-item-path'))) {
            event.preventDefault()
          }
        }}
      >
        {children}
      </ContextMenuTrigger>
      <ContextMenuContent
        className="nova-file-tree-menu-surface nova-file-tree-menu-radix"
        onCloseAutoFocus={(event) => {
          const type = pendingCreate.current
          if (!type) return
          pendingCreate.current = null
          // Transfer focus only after the menu releases its focus scope.
          event.preventDefault()
          onCreate(type)
        }}
      >
        <div role="group">
          <ContextMenuItem className="nova-file-tree-menu-item" disabled={busy} onSelect={() => { pendingCreate.current = 'file' }}>
            <FilePlus2 />{t('sidebar.createFile')}
          </ContextMenuItem>
          <ContextMenuItem className="nova-file-tree-menu-item" disabled={busy} onSelect={() => { pendingCreate.current = 'dir' }}>
            <FolderPlus />{t('sidebar.createDir')}
          </ContextMenuItem>
        </div>
        <ContextMenuSeparator className="nova-file-tree-menu-separator" />
        <ContextMenuItem className="nova-file-tree-menu-item" disabled={busy || !canPaste} onSelect={onPaste}>
          <ClipboardPaste />{t('sidebar.paste')}
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  )
}
