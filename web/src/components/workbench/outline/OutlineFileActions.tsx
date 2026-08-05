import { useCallback, useState, type MouseEvent, type ReactNode } from 'react'
import { AtSign, FolderSearch, MoreHorizontal, Pencil, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
import { DeleteConfirmDialog } from '@/components/Sidebar/DeleteConfirmDialog'
import { FileOperationDialog } from '@/components/Sidebar/FileOperationDialog'
import { workspaceFileName } from '@/lib/workspace-path'

interface OutlineFileActionsProps {
  path: string
  children: ReactNode
  triggerPlacement?: 'center' | 'top'
  showTrigger?: boolean
  onReferenceFile?: (path: string) => void
  onRevealFile?: (path: string) => void | Promise<void>
  onRenameItem?: (path: string, newName: string) => Promise<void>
  onDeleteItem?: (path: string) => Promise<void>
}

interface OutlineFileAction {
  label?: string
  icon?: ReactNode
  danger?: boolean
  separator?: boolean
  onSelect?: () => void
}

const MENU_CONTENT_CLASS =
  'min-w-[180px] rounded-lg border-[var(--nova-border)] bg-[var(--nova-menu-bg)] p-1 text-[var(--nova-text)] shadow-[0_12px_32px_rgba(0,0,0,0.18)] backdrop-blur'
const MENU_ITEM_CLASS =
  'cursor-pointer gap-2 rounded-md px-2 py-1.5 text-xs text-[var(--nova-text-muted)] transition-colors focus:bg-[var(--nova-menu-item-hover-bg)] focus:text-[var(--nova-text)] data-[highlighted]:bg-[var(--nova-menu-item-hover-bg)] data-[highlighted]:text-[var(--nova-text)] [&_svg]:text-[var(--nova-tree-icon)] focus:[&_svg]:text-[var(--nova-text)] data-[highlighted]:[&_svg]:text-[var(--nova-text)]'
const MENU_DANGER_CLASS =
  'text-[var(--nova-danger)] focus:bg-[var(--nova-danger-bg)] focus:text-[var(--nova-danger)] data-[highlighted]:bg-[var(--nova-danger-bg)] data-[highlighted]:text-[var(--nova-danger)] [&_svg]:text-[var(--nova-danger)]'
const MENU_SEPARATOR_CLASS = 'mx-1 my-1 h-px bg-[var(--nova-border)]'

/** File actions shared by the discoverable trigger and the native context-menu gesture. */
export function OutlineFileActions({
  path,
  children,
  triggerPlacement = 'center',
  showTrigger = true,
  onReferenceFile,
  onRevealFile,
  onRenameItem,
  onDeleteItem,
}: OutlineFileActionsProps) {
  const { t } = useTranslation()
  const [renameOpen, setRenameOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const actions = compactActions([
    ...(onReferenceFile ? [{ label: t('sidebar.referenceToChat'), icon: <AtSign className="h-3.5 w-3.5" />, onSelect: () => onReferenceFile(path) }] : []),
    ...(onRevealFile ? [{ label: t('sidebar.revealInProjectFiles'), icon: <FolderSearch className="h-3.5 w-3.5" />, onSelect: () => { void onRevealFile(path) } }] : []),
    ...(onRenameItem || onDeleteItem ? [{ separator: true }] : []),
    ...(onRenameItem ? [{ label: t('sidebar.renameFile'), icon: <Pencil className="h-3.5 w-3.5" />, onSelect: () => setRenameOpen(true) }] : []),
    ...(onDeleteItem ? [{ label: t('sidebar.delete'), icon: <Trash2 className="h-3.5 w-3.5" />, danger: true, onSelect: () => setDeleteOpen(true) }] : []),
  ])

  const openContextMenuFromTrigger = useCallback((event: MouseEvent<HTMLButtonElement>) => {
    event.preventDefault()
    event.stopPropagation()
    const rect = event.currentTarget.getBoundingClientRect()
    event.currentTarget.dispatchEvent(new window.MouseEvent('contextmenu', {
      bubbles: true,
      cancelable: true,
      clientX: rect.right,
      clientY: rect.bottom,
    }))
  }, [])

  if (actions.length === 0) return <>{children}</>

  return (
    <>
      <ContextMenu>
        <ContextMenuTrigger asChild>
          <div className="group relative min-w-0">
            {children}
            {showTrigger ? (
              <button
                type="button"
                aria-label={t('sidebar.moreActions')}
                aria-haspopup="menu"
                title={t('sidebar.moreActions')}
                className={`pointer-events-none absolute right-1 z-10 flex h-6 w-6 items-center justify-center rounded text-[var(--nova-text-faint)] opacity-0 transition-opacity hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] focus-visible:pointer-events-auto focus-visible:opacity-100 group-hover:pointer-events-auto group-hover:opacity-100 max-md:pointer-events-auto max-md:opacity-100 ${triggerPlacement === 'top' ? 'top-1' : 'top-1/2 -translate-y-1/2'}`}
                onPointerDown={(event) => event.stopPropagation()}
                onClick={openContextMenuFromTrigger}
              >
                <MoreHorizontal className="h-3.5 w-3.5" />
              </button>
            ) : null}
          </div>
        </ContextMenuTrigger>
        <ContextMenuContent className={MENU_CONTENT_CLASS}>
          {renderActions(actions)}
        </ContextMenuContent>
      </ContextMenu>
      {onRenameItem && renameOpen ? (
        <FileOperationDialog
          open={renameOpen}
          mode="rename"
          targetPath={path}
          defaultValue={workspaceFileName(path)}
          onOpenChange={setRenameOpen}
          onSubmit={(newName) => onRenameItem(path, newName)}
        />
      ) : null}
      {onDeleteItem && deleteOpen ? (
        <DeleteConfirmDialog
          open={deleteOpen}
          path={path}
          onOpenChange={setDeleteOpen}
          onConfirm={() => onDeleteItem(path)}
        />
      ) : null}
    </>
  )
}

function compactActions(actions: OutlineFileAction[]) {
  const compacted: OutlineFileAction[] = []
  for (const action of actions) {
    if (action.separator) {
      if (compacted.length > 0 && !compacted[compacted.length - 1].separator) compacted.push(action)
      continue
    }
    if (action.label) compacted.push(action)
  }
  if (compacted[compacted.length - 1]?.separator) compacted.pop()
  return compacted
}

function renderActions(actions: OutlineFileAction[]) {
  return actions.map((action, index) => {
    if (action.separator) {
      return <ContextMenuSeparator key={index} className={MENU_SEPARATOR_CLASS} />
    }
    const className = `${MENU_ITEM_CLASS} ${action.danger ? MENU_DANGER_CLASS : ''}`
    return (
      <ContextMenuItem key={action.label} className={className} onSelect={action.onSelect}>
        {action.icon}
        {action.label}
      </ContextMenuItem>
    )
  })
}
