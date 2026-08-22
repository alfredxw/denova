import type { ReactNode } from 'react'
import { DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { GripVertical, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'

export interface SidebarCustomizationItem {
  id: string
  label: string
  icon: ReactNode
}

interface WorkbenchSidebarCustomizationDialogProps {
  open: boolean
  items: SidebarCustomizationItem[]
  hiddenItemIDs: string[]
  onOpenChange: (open: boolean) => void
  onReorder: (activeID: string, overID: string) => void
  onVisibilityChange: (id: string, visible: boolean) => void
}

/** User-level primary-navigation preferences shared by every workbench route. */
export function WorkbenchSidebarCustomizationDialog({
  open,
  items,
  hiddenItemIDs,
  onOpenChange,
  onReorder,
  onVisibilityChange,
}: WorkbenchSidebarCustomizationDialogProps) {
  const { t } = useTranslation()
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )
  const hiddenIDs = new Set(hiddenItemIDs)
  const visibleCount = items.filter((item) => !hiddenIDs.has(item.id)).length

  const handleDragEnd = ({ active, over }: DragEndEvent) => {
    if (typeof active.id !== 'string' || typeof over?.id !== 'string' || active.id === over.id) return
    onReorder(active.id, over.id)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="max-h-[min(90dvh,44rem)] grid-rows-[auto_minmax(0,1fr)] gap-5 overflow-hidden p-0 sm:max-w-[36rem]"
      >
        <DialogHeader className="relative gap-1.5 px-5 pt-5 pr-14">
          <DialogTitle className="text-lg">{t('workbench.sidebar.customizeTitle')}</DialogTitle>
          <DialogDescription>{t('workbench.sidebar.customizeDescription')}</DialogDescription>
          <DialogClose asChild>
            <Button type="button" variant="ghost" size="icon-sm" className="absolute top-3 right-3">
              <X />
              <span className="sr-only">{t('common.close')}</span>
            </Button>
          </DialogClose>
        </DialogHeader>

        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
          <SortableContext items={items.map((item) => item.id)} strategy={verticalListSortingStrategy}>
            <div className="min-h-0 overflow-y-auto px-5 pb-5">
              <div className="overflow-hidden rounded-xl border border-[var(--nova-border)] bg-[var(--nova-surface)]">
                {items.map((item) => {
                  const visible = !hiddenIDs.has(item.id)
                  return (
                    <CustomizationRow
                      key={item.id}
                      item={item}
                      visible={visible}
                      visibilityDisabled={visible && visibleCount === 1}
                      onVisibilityChange={(nextVisible) => onVisibilityChange(item.id, nextVisible)}
                    />
                  )
                })}
              </div>
            </div>
          </SortableContext>
        </DndContext>
      </DialogContent>
    </Dialog>
  )
}

function CustomizationRow({
  item,
  visible,
  visibilityDisabled,
  onVisibilityChange,
}: {
  item: SidebarCustomizationItem
  visible: boolean
  visibilityDisabled: boolean
  onVisibilityChange: (visible: boolean) => void
}) {
  const { t } = useTranslation()
  const { attributes, listeners, setActivatorNodeRef, setNodeRef, transform, transition, isDragging } = useSortable({ id: item.id })

  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={cn(
        'flex min-h-12 items-center gap-2 border-b border-[var(--nova-border)] px-3 text-sm last:border-b-0',
        isDragging && 'relative z-10 bg-[var(--nova-active)] shadow-sm',
      )}
    >
      <button
        ref={setActivatorNodeRef}
        type="button"
        className="flex size-7 touch-none items-center justify-center rounded-md text-[var(--nova-text-faint)] outline-none hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] focus-visible:ring-2 focus-visible:ring-[var(--nova-field-focus-border)] active:cursor-grabbing"
        aria-label={t('workbench.sidebar.dragItem', { label: item.label })}
        {...attributes}
        {...listeners}
      >
        <GripVertical className="size-4" />
      </button>
      <span className="flex size-5 shrink-0 items-center justify-center text-[var(--nova-text-muted)]">{item.icon}</span>
      <span className={cn('min-w-0 flex-1 truncate font-medium', visible ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)]')}>
        {item.label}
      </span>
      <Switch
        checked={visible}
        disabled={visibilityDisabled}
        aria-label={t('workbench.sidebar.visibilityLabel', { label: item.label })}
        onCheckedChange={onVisibilityChange}
      />
    </div>
  )
}
