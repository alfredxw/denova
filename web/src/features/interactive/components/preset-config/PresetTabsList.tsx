import { DndContext, DragOverlay, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent, type DragStartEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { GripVertical, Plus } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'

const iconActionClassName = 'nova-nav-item rounded-[10px] border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)] transition-colors hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'

interface PresetTabsListEntry {
  id: string
  title: string
  subtitle: string
}

export function PresetTabsList<T>({ items, activeId, getId, getTitle, getSubtitle, addLabel, addControl, emptyLabel, layout = 'panel', testIdPrefix = 'preset-tabs-list', onAdd, onActiveIdChange, onItemsChange }: { items: T[]; activeId: string; getId: (item: T, index: number) => string; getTitle: (item: T, index: number) => string; getSubtitle?: (item: T, index: number) => string; addLabel: string; addControl?: ReactNode; emptyLabel: string; layout?: 'panel' | 'rail'; testIdPrefix?: string; onAdd: () => void; onActiveIdChange: (id: string) => void; onItemsChange: (items: T[]) => void }) {
  const { t } = useTranslation()
  const [draggingId, setDraggingId] = useState<string | null>(null)
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  )
  const entries = items.map((item, index) => ({
    id: getId(item, index),
    title: getTitle(item, index),
    subtitle: getSubtitle?.(item, index) || '',
  }))
  const ids = entries.map((entry) => entry.id)
  const draggingEntry = draggingId ? entries.find((entry) => entry.id === draggingId) : null

  const handleDragStart = (event: DragStartEvent) => {
    setDraggingId(String(event.active.id))
  }

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    setDraggingId(null)
    if (!over || active.id === over.id) return
    const nextItems = reorderPresetTabsListItems(items, ids, String(active.id), String(over.id))
    if (nextItems !== items) onItemsChange(nextItems)
  }

  return (
    <aside className={cn('flex min-h-0 flex-col overflow-hidden rounded-[12px] border border-[var(--nova-border-soft)] bg-transparent', layout === 'rail' && 'h-full')}>
      <div className="flex min-h-10 items-center justify-between gap-2 border-b border-[var(--nova-border-soft)] px-3 py-2">
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate text-xs font-semibold text-[var(--nova-text)]">{emptyLabel}</span>
          <span className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-0.5 text-[10px] text-[var(--nova-text-faint)]">{items.length}</span>
        </div>
        {addControl || (
          <Button className={iconActionClassName} variant="outline" size="icon-sm" onClick={onAdd} aria-label={addLabel} title={addLabel}>
            <Plus />
          </Button>
        )}
      </div>
      <ScrollArea className="min-h-0 flex-1">
        {items.length === 0 ? (
          <div className="m-3 flex min-h-32 items-center justify-center rounded-[12px] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-5 text-center text-xs leading-5 text-[var(--nova-text-faint)]">{emptyLabel}</div>
        ) : (
          <Tabs value={activeId} onValueChange={onActiveIdChange} orientation="vertical" activationMode="automatic" className="min-h-0 gap-0">
            <DndContext sensors={sensors} collisionDetection={closestCenter} onDragStart={handleDragStart} onDragCancel={() => setDraggingId(null)} onDragEnd={handleDragEnd}>
              <SortableContext items={ids} strategy={verticalListSortingStrategy}>
                <TabsList variant="line" aria-label={emptyLabel} className="flex h-auto w-full flex-col items-stretch justify-start gap-0.5 rounded-none bg-transparent p-1.5">
                  {entries.map((entry) => (
                    <PresetTabsListItem key={entry.id} id={entry.id} title={entry.title} subtitle={entry.subtitle} active={entry.id === activeId} testIdPrefix={testIdPrefix} dragLabel={t('settingPanel.presetConfig.dragItem', { name: entry.title })} />
                  ))}
                </TabsList>
              </SortableContext>
              <DragOverlay>{draggingEntry ? <PresetTabsListItemContent title={draggingEntry.title} subtitle={draggingEntry.subtitle} active={draggingEntry.id === activeId} overlay /> : null}</DragOverlay>
            </DndContext>
          </Tabs>
        )}
      </ScrollArea>
    </aside>
  )
}

export function reorderPresetTabsListItems<T>(items: T[], ids: string[], activeId: string, overId: string): T[] {
  if (activeId === overId) return items
  const oldIndex = ids.indexOf(activeId)
  const newIndex = ids.indexOf(overId)
  if (oldIndex < 0 || newIndex < 0) return items
  return arrayMove(items, oldIndex, newIndex)
}

function PresetTabsListItem({
  id,
  title,
  subtitle,
  active,
  testIdPrefix,
  dragLabel,
}: PresetTabsListEntry & {
  active: boolean
  testIdPrefix: string
  dragLabel: string
}) {
  const { attributes, listeners, setActivatorNodeRef, setNodeRef, transform, transition, isDragging } = useSortable({ id })
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  }

  return (
    <div ref={setNodeRef} style={style} className={cn(presetTabsListItemClassName(active), isDragging && 'opacity-35')} data-testid={`${testIdPrefix}-item-${id}`}>
      <button ref={setActivatorNodeRef} type="button" className="flex size-5 shrink-0 cursor-grab items-center justify-center rounded text-[var(--nova-text-faint)] opacity-45 transition-[opacity,color,background] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] hover:opacity-100 focus-visible:opacity-100 active:cursor-grabbing" aria-label={dragLabel} onClick={(event) => event.stopPropagation()} {...attributes} {...listeners}>
        <GripVertical className="size-3" />
      </button>
      <TabsTrigger value={id} className="h-auto min-h-9 min-w-0 flex-1 justify-start rounded-[8px] border-0 bg-transparent px-1.5 py-1 text-left text-xs font-normal text-inherit shadow-none whitespace-normal transition-none after:hidden data-active:bg-transparent data-active:text-inherit dark:data-active:bg-transparent dark:data-active:text-inherit" data-testid={`${testIdPrefix}-trigger-${id}`}>
        <PresetTabsListItemText title={title} subtitle={subtitle} />
      </TabsTrigger>
    </div>
  )
}

function PresetTabsListItemContent({ title, subtitle, active, overlay = false }: { title: string; subtitle: string; active: boolean; overlay?: boolean }) {
  return (
    <div className={cn(presetTabsListItemClassName(active), overlay && 'w-[280px] shadow-[0_18px_45px_rgba(0,0,0,0.22)] ring-1 ring-[var(--nova-accent)]/25')}>
      <div className="flex size-5 shrink-0 items-center justify-center text-[var(--nova-text-faint)]">
        <GripVertical className="size-3" />
      </div>
      <div className="min-w-0 flex-1 px-1.5 py-1">
        <PresetTabsListItemText title={title} subtitle={subtitle} />
      </div>
    </div>
  )
}

function PresetTabsListItemText({ title, subtitle }: { title: string; subtitle: string }) {
  return (
    <span className="block min-w-0 flex-1">
      <span className="block truncate font-semibold">{title}</span>
      {subtitle ? <span className="mt-px block truncate text-[10px] leading-4 text-[var(--nova-text-faint)]">{subtitle}</span> : null}
    </span>
  )
}

function presetTabsListItemClassName(active: boolean) {
  return cn(
    'group flex min-h-11 items-center gap-0.5 rounded-[9px] px-1.5 py-1 text-xs transition-colors',
    active
      ? 'bg-[var(--nova-active)] text-[var(--nova-text)] shadow-[inset_0_0_0_1px_var(--nova-border)]'
      : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]',
  )
}
