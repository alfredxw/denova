import { X } from 'lucide-react'
import type { KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { cn } from '@/lib/utils'
import type { TreeNode } from '../types'
import { StateTreeNode } from './StateTreeNode'

interface StateTreeNavigatorProps {
  attached?: boolean
  tree: TreeNode[]
  selectedId: string
  expandedIds: Set<string>
  onSelect: (id: string) => void
  onKeyboardSelect?: (id: string) => void
  onClose?: () => void
  onToggleExpanded: (id: string) => void
  onAddTemplate?: () => void
  onAddField?: (templateId: string) => void
  onAddActor?: (templateId: string) => void
  onAddPool?: () => void
  onAddTrait?: (poolId: string) => void
}

export function StateTreeNavigator({
  attached = false,
  tree,
  selectedId,
  expandedIds,
  onSelect,
  onKeyboardSelect = onSelect,
  onClose,
  onToggleExpanded,
  onAddTemplate,
  onAddField,
  onAddActor,
  onAddPool,
  onAddTrait,
}: StateTreeNavigatorProps) {
  const { t } = useTranslation()
  return (
    <aside className={cn(
      'flex h-full min-h-0 flex-col overflow-hidden',
      attached
        ? 'rounded-none border-r border-[var(--nova-border)] bg-[var(--nova-surface)]'
        : 'rounded-[20px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] shadow-[inset_0_1px_0_rgba(255,255,255,0.05)]',
    )}>
      <div className="flex min-h-10 items-center justify-between gap-2 border-b border-[var(--nova-border)] px-3 py-2">
        <span className="truncate text-xs font-semibold text-[var(--nova-text)]">
          {t('settingPanel.actorState.explorer.structure')}
        </span>
        {onClose ? (
          <Button type="button" variant="ghost" size="icon-sm" className="actor-state-navigation-close size-8 rounded-full" onClick={onClose} aria-label={t('settingPanel.actorState.explorer.closeStructure')}>
            <X />
          </Button>
        ) : null}
      </div>
      <ScrollArea className="actor-state-tree-scroll min-h-0 flex-1 overflow-hidden" data-testid="actor-state-tree-scroll">
        <div
          role="tree"
          aria-label={t('settingPanel.actorState.explorer.structure')}
          aria-orientation="vertical"
          className="w-full min-w-0 max-w-full overflow-hidden p-1.5 pr-2"
          onKeyDown={(event) => handleTreeArrowKey(event, onKeyboardSelect)}
        >
          {tree.map((node) => (
            <StateTreeNode
              key={node.id}
              node={node}
              selectedId={selectedId}
              expandedIds={expandedIds}
              indentLevel={0}
              onSelect={onSelect}
              onToggleExpanded={onToggleExpanded}
              onAddTemplate={onAddTemplate}
              onAddField={onAddField}
              onAddActor={onAddActor}
              onAddPool={onAddPool}
              onAddTrait={onAddTrait}
            />
          ))}
        </div>
      </ScrollArea>
    </aside>
  )
}

function handleTreeArrowKey(event: KeyboardEvent<HTMLDivElement>, onSelect: (id: string) => void) {
  if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
  const target = event.target instanceof Element
    ? event.target.closest<HTMLElement>('[role="treeitem"][data-node-id]')
    : null
  if (!target) return

  const items = Array.from(event.currentTarget.querySelectorAll<HTMLElement>('[role="treeitem"][data-node-id]'))
  const currentIndex = items.indexOf(target)
  const offset = event.key === 'ArrowDown' ? 1 : -1
  const nextItem = items[currentIndex + offset]
  const nextId = nextItem?.dataset.nodeId
  if (!nextItem || !nextId) return

  event.preventDefault()
  onSelect(nextId)
  nextItem.focus()
}
