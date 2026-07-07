import { DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { GripVertical, Plus } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'

const iconActionClassName = 'nova-nav-item border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'

export function SortablePresetList<T>({
  items,
  activeId,
  getId,
  getTitle,
  getSubtitle,
  addLabel,
  addControl,
  emptyLabel,
  onAdd,
  onActiveIdChange,
  onItemsChange,
}: {
  items: T[]
  activeId: string
  getId: (item: T, index: number) => string
  getTitle: (item: T, index: number) => string
  getSubtitle?: (item: T, index: number) => string
  addLabel: string
  addControl?: ReactNode
  emptyLabel: string
  onAdd: () => void
  onActiveIdChange: (id: string) => void
  onItemsChange: (items: T[]) => void
}) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )
  const ids = items.map(getId)

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    if (!over || active.id === over.id) return
    const oldIndex = ids.indexOf(String(active.id))
    const newIndex = ids.indexOf(String(over.id))
    if (oldIndex < 0 || newIndex < 0) return
    onItemsChange(arrayMove(items, oldIndex, newIndex))
  }

  return (
    <aside className="flex min-h-0 max-h-[60vh] flex-col overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
      <div className="flex h-10 items-center justify-between border-b border-[var(--nova-border)] px-2">
        <span className="text-[11px] font-medium text-[var(--nova-text-muted)]">{emptyLabel}</span>
        {addControl || (
          <Button className={iconActionClassName} variant="outline" size="icon-sm" onClick={onAdd} aria-label={addLabel} title={addLabel}>
            <Plus className="h-3.5 w-3.5" />
          </Button>
        )}
      </div>
      <ScrollArea className="min-h-0 flex-1">
        {items.length === 0 ? (
          <div className="px-3 py-4 text-xs leading-5 text-[var(--nova-text-faint)]">{emptyLabel}</div>
        ) : (
          <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
            <SortableContext items={ids} strategy={verticalListSortingStrategy}>
              <div className="space-y-1 p-2">
                {items.map((item, index) => {
                  const id = getId(item, index)
                  return (
                    <SortablePresetListItem
                      key={id}
                      id={id}
                      title={getTitle(item, index)}
                      subtitle={getSubtitle?.(item, index) || ''}
                      active={id === activeId}
                      onSelect={() => onActiveIdChange(id)}
                    />
                  )
                })}
              </div>
            </SortableContext>
          </DndContext>
        )}
      </ScrollArea>
    </aside>
  )
}

function SortablePresetListItem({
  id,
  title,
  subtitle,
  active,
  onSelect,
}: {
  id: string
  title: string
  subtitle: string
  active: boolean
  onSelect: () => void
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id })
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  }

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`flex min-h-12 items-center gap-1 rounded-md border px-1.5 py-1.5 text-xs transition ${active ? 'border-[var(--nova-accent)]/45 bg-[var(--nova-active)] text-[var(--nova-text)] shadow-[inset_3px_0_0_var(--nova-accent)]' : 'border-transparent text-[var(--nova-text-muted)] hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'} ${isDragging ? 'opacity-60' : ''}`}
    >
      <button
        type="button"
        className="nova-nav-item flex h-7 w-6 shrink-0 items-center justify-center rounded text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
        aria-label={title}
        {...attributes}
        {...listeners}
      >
        <GripVertical className="h-3.5 w-3.5" />
      </button>
      <button type="button" onClick={onSelect} className="min-w-0 flex-1 text-left">
        <span className="block truncate font-medium">{title}</span>
        {subtitle ? <span className="mt-0.5 block truncate text-[11px] text-[var(--nova-text-faint)]">{subtitle}</span> : null}
      </button>
    </div>
  )
}
