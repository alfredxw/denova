import { useMemo, useState, type CSSProperties, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
  type DraggableAttributes,
  type DraggableSyntheticListeners,
} from '@dnd-kit/core'
import { SortableContext, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { ArrowDown, ArrowLeft, ArrowRight, ArrowUp, GripVertical, RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { moveStoryStateLayoutField, moveStoryStateLayoutGroup, reconcileStoryStateLayout, type StoryStateTemplateLayout } from './layout-preference'
import type { LedgerFieldGroup } from './model'

const groupDragId = (key: string) => `group:${key}`
const groupDropId = (key: string) => `group-fields:${key}`
const fieldDragId = (id: string) => `field:${id}`

interface StateLayoutEditorProps {
  open: boolean
  title: string
  groups: LedgerFieldGroup[]
  value?: StoryStateTemplateLayout
  onOpenChange: (open: boolean) => void
  onChange: (value: StoryStateTemplateLayout) => void
  onReset: () => void
}

export function StateLayoutEditor({ open, title, groups, value, onOpenChange, onChange, onReset }: StateLayoutEditorProps) {
  const { t } = useTranslation()
  const layout = useMemo(() => reconcileStoryStateLayout(groups, value), [groups, value])
  const fieldLabels = useMemo(() => new Map(groups.flatMap((group) => group.fields.map((field) => [field.id, field.label] as const))), [groups])
  const [activeDragId, setActiveDragId] = useState<string | null>(null)
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  const handleDragEnd = ({ active, over }: DragEndEvent) => {
    setActiveDragId(null)
    if (!over || active.id === over.id) return
    const activeData = active.data.current
    const overData = over.data.current
    if (activeData?.type === 'group') {
      const targetGroupKey = overData?.groupKey
      if (typeof targetGroupKey === 'string') {
        onChange(moveStoryStateLayoutGroup(layout, activeData.groupKey, targetGroupKey))
      }
      return
    }
    if (activeData?.type !== 'field' || typeof overData?.groupKey !== 'string') return
    const targetGroup = layout.groups.find((group) => group.key === overData.groupKey)
    if (!targetGroup) return
    const targetIndex = overData.type === 'field'
      ? targetGroup.field_ids.indexOf(overData.fieldId)
      : targetGroup.field_ids.length
    onChange(moveStoryStateLayoutField(layout, activeData.fieldId, targetGroup.key, targetIndex))
  }

  const moveGroup = (groupIndex: number, delta: number) => {
    const target = groupIndex + delta
    if (target < 0 || target >= layout.groups.length) return
    onChange(moveStoryStateLayoutGroup(layout, layout.groups[groupIndex].key, layout.groups[target].key))
  }

  const moveField = (groupIndex: number, fieldIndex: number, delta: number) => {
    const group = layout.groups[groupIndex]
    const fieldId = group?.field_ids[fieldIndex]
    if (!group || !fieldId) return
    const target = fieldIndex + delta
    if (target < 0 || target >= group.field_ids.length) return
    onChange(moveStoryStateLayoutField(layout, fieldId, group.key, target))
  }

  const moveFieldAcrossGroup = (groupIndex: number, fieldIndex: number, delta: number) => {
    const fieldId = layout.groups[groupIndex]?.field_ids[fieldIndex]
    const target = layout.groups[groupIndex + delta]
    if (!fieldId || !target) return
    onChange(moveStoryStateLayoutField(layout, fieldId, target.key, target.field_ids.length))
  }

  const activeGroup = activeDragId?.startsWith('group:')
    ? layout.groups.find((group) => groupDragId(group.key) === activeDragId)
    : undefined
  const activeFieldId = activeDragId?.startsWith('field:') ? activeDragId.slice('field:'.length) : ''

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} className="max-h-[min(90dvh,48rem)] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden p-0 sm:max-w-[42rem]">
        <DialogHeader className="px-4 pt-4">
          <DialogTitle>{t('storyStage.state.layout.title', { name: title })}</DialogTitle>
          <DialogDescription>{t('storyStage.state.layout.description')}</DialogDescription>
        </DialogHeader>
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          onDragStart={({ active }: DragStartEvent) => setActiveDragId(String(active.id))}
          onDragCancel={() => setActiveDragId(null)}
          onDragEnd={handleDragEnd}
        >
          <div className="min-h-0 space-y-2 overflow-y-auto px-4 pb-2">
            <SortableContext items={layout.groups.map((group) => groupDragId(group.key))} strategy={verticalListSortingStrategy}>
              {layout.groups.map((group, groupIndex) => (
                <SortableGroupSection
                  key={group.key}
                  group={group}
                  groupIndex={groupIndex}
                  groupCount={layout.groups.length}
                  fieldLabels={fieldLabels}
                  onMoveGroup={moveGroup}
                  onMoveField={moveField}
                  onMoveFieldAcrossGroup={moveFieldAcrossGroup}
                />
              ))}
            </SortableContext>
          </div>
          {createPortal(
            <DragOverlay>
              {activeGroup ? (
                <GroupSectionView label={builtinGroupLabel(activeGroup.key, t)} fieldCount={activeGroup.field_ids.length} dragging>
                  <div className="space-y-1">
                    {activeGroup.field_ids.map((fieldId) => <FieldRowView key={fieldId} label={fieldLabels.get(fieldId) || fieldId} />)}
                  </div>
                </GroupSectionView>
              ) : activeFieldId ? (
                <FieldRowView label={fieldLabels.get(activeFieldId) || activeFieldId} dragging />
              ) : null}
            </DragOverlay>,
            document.body,
          )}
        </DndContext>
        <DialogFooter className="m-0">
          <Button type="button" variant="outline" onClick={onReset}>
            <RotateCcw data-icon="inline-start" />
            {t('storyStage.state.layout.reset')}
          </Button>
          <DialogClose asChild>
            <Button type="button">{t('common.close')}</Button>
          </DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function SortableGroupSection({ group, groupIndex, groupCount, fieldLabels, onMoveGroup, onMoveField, onMoveFieldAcrossGroup }: {
  group: StoryStateTemplateLayout['groups'][number]
  groupIndex: number
  groupCount: number
  fieldLabels: Map<string, string>
  onMoveGroup: (groupIndex: number, delta: number) => void
  onMoveField: (groupIndex: number, fieldIndex: number, delta: number) => void
  onMoveFieldAcrossGroup: (groupIndex: number, fieldIndex: number, delta: number) => void
}) {
  const { t } = useTranslation()
  const label = builtinGroupLabel(group.key, t)
  const sortable = useSortable({ id: groupDragId(group.key), data: { type: 'group', groupKey: group.key } })
  const droppable = useDroppable({ id: groupDropId(group.key), data: { type: 'group-drop', groupKey: group.key } })
  return (
    <GroupSectionView
      label={label}
      fieldCount={group.field_ids.length}
      gripLabel={t('storyStage.state.layout.dragGroup', { name: label })}
      innerRef={sortable.setNodeRef}
      style={{ transform: CSS.Transform.toString(sortable.transform), transition: sortable.transition }}
      dragAttributes={sortable.attributes}
      dragListeners={sortable.listeners}
      dimmed={sortable.isDragging}
      actions={(
        <>
          <MoveButton icon={ArrowUp} label={t('storyStage.state.layout.moveGroupUp', { name: label })} disabled={groupIndex === 0} onClick={() => onMoveGroup(groupIndex, -1)} />
          <MoveButton icon={ArrowDown} label={t('storyStage.state.layout.moveGroupDown', { name: label })} disabled={groupIndex === groupCount - 1} onClick={() => onMoveGroup(groupIndex, 1)} />
        </>
      )}
    >
      <SortableContext items={group.field_ids.map(fieldDragId)} strategy={verticalListSortingStrategy}>
        <div
          ref={droppable.setNodeRef}
          className={cn(
            'space-y-1 rounded-lg transition-[background-color,border-color]',
            group.field_ids.length === 0 && (droppable.isOver
              ? 'min-h-10 border border-dashed border-[var(--nova-accent)]'
              : 'flex min-h-10 items-center justify-center border border-dashed border-[var(--nova-border)]'),
            droppable.isOver && 'bg-[var(--nova-active)]',
          )}
        >
          {group.field_ids.length === 0 && !droppable.isOver ? (
            <span className="text-[10px] text-[var(--nova-text-faint)]">{t('storyStage.state.layout.emptyGroup')}</span>
          ) : null}
          {group.field_ids.map((fieldId, fieldIndex) => (
            <SortableFieldRow
              key={fieldId}
              fieldId={fieldId}
              label={fieldLabels.get(fieldId) || fieldId}
              groupKey={group.key}
              groupIndex={groupIndex}
              groupCount={groupCount}
              fieldIndex={fieldIndex}
              fieldCount={group.field_ids.length}
              onMove={onMoveField}
              onMoveAcrossGroup={onMoveFieldAcrossGroup}
            />
          ))}
        </div>
      </SortableContext>
    </GroupSectionView>
  )
}

function SortableFieldRow({ fieldId, label, groupKey, groupIndex, groupCount, fieldIndex, fieldCount, onMove, onMoveAcrossGroup }: {
  fieldId: string
  label: string
  groupKey: string
  groupIndex: number
  groupCount: number
  fieldIndex: number
  fieldCount: number
  onMove: (groupIndex: number, fieldIndex: number, delta: number) => void
  onMoveAcrossGroup: (groupIndex: number, fieldIndex: number, delta: number) => void
}) {
  const { t } = useTranslation()
  const sortable = useSortable({ id: fieldDragId(fieldId), data: { type: 'field', fieldId, groupKey } })
  return (
    <FieldRowView
      label={label}
      gripLabel={t('storyStage.state.layout.dragField', { name: label })}
      innerRef={sortable.setNodeRef}
      style={{ transform: CSS.Transform.toString(sortable.transform), transition: sortable.transition }}
      dragAttributes={sortable.attributes}
      dragListeners={sortable.listeners}
      dimmed={sortable.isDragging}
      actions={(
        <>
          <MoveButton icon={ArrowLeft} label={t('storyStage.state.layout.moveFieldPreviousGroup', { name: label })} disabled={groupIndex === 0} onClick={() => onMoveAcrossGroup(groupIndex, fieldIndex, -1)} />
          <MoveButton icon={ArrowRight} label={t('storyStage.state.layout.moveFieldNextGroup', { name: label })} disabled={groupIndex === groupCount - 1} onClick={() => onMoveAcrossGroup(groupIndex, fieldIndex, 1)} />
          <MoveButton icon={ArrowUp} label={t('storyStage.state.layout.moveFieldUp', { name: label })} disabled={fieldIndex === 0} onClick={() => onMove(groupIndex, fieldIndex, -1)} />
          <MoveButton icon={ArrowDown} label={t('storyStage.state.layout.moveFieldDown', { name: label })} disabled={fieldIndex === fieldCount - 1} onClick={() => onMove(groupIndex, fieldIndex, 1)} />
        </>
      )}
    />
  )
}

function GroupSectionView({ label, fieldCount, gripLabel, innerRef, style, dragAttributes, dragListeners, dragging, dimmed, actions, children }: {
  label: string
  fieldCount: number
  gripLabel?: string
  innerRef?: (node: HTMLElement | null) => void
  style?: CSSProperties
  dragAttributes?: DraggableAttributes
  dragListeners?: DraggableSyntheticListeners
  dragging?: boolean
  dimmed?: boolean
  actions?: ReactNode
  children: ReactNode
}) {
  return (
    <section
      ref={innerRef}
      style={style}
      className={cn(
        'rounded-xl border border-[var(--nova-border)] bg-[var(--nova-surface)] p-2 transition-[border-color,background-color,opacity,box-shadow]',
        dragging && 'border-[var(--nova-accent)] shadow-xl',
        dimmed && 'opacity-40',
      )}
    >
      <header className="mb-2 flex min-w-0 items-center gap-1.5">
        <button
          type="button"
          {...dragAttributes}
          {...dragListeners}
          className="flex size-7 shrink-0 cursor-grab items-center justify-center rounded-md text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] active:cursor-grabbing"
          aria-label={gripLabel}
        >
          <GripVertical className="size-4" />
        </button>
        <h3 className="min-w-0 flex-1 truncate text-xs font-semibold text-[var(--nova-text)]">{label}</h3>
        <span className="text-[10px] text-[var(--nova-text-faint)]">{fieldCount}</span>
        {actions}
      </header>
      {children}
    </section>
  )
}

function FieldRowView({ label, gripLabel, innerRef, style, dragAttributes, dragListeners, dragging, dimmed, actions }: {
  label: string
  gripLabel?: string
  innerRef?: (node: HTMLDivElement | null) => void
  style?: CSSProperties
  dragAttributes?: DraggableAttributes
  dragListeners?: DraggableSyntheticListeners
  dragging?: boolean
  dimmed?: boolean
  actions?: ReactNode
}) {
  return (
    <div
      ref={innerRef}
      style={style}
      className={cn(
        'flex min-w-0 items-center gap-1 rounded-lg border border-[var(--nova-border-soft)] bg-[var(--nova-surface-2)] px-1.5 py-1 transition-[border-color,background-color,opacity,box-shadow]',
        dragging && 'border-[var(--nova-accent)] shadow-lg',
        dimmed && 'opacity-40',
      )}
    >
      <button
        type="button"
        {...dragAttributes}
        {...dragListeners}
        className="flex size-7 shrink-0 cursor-grab items-center justify-center rounded-md text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] active:cursor-grabbing"
        aria-label={gripLabel}
      >
        <GripVertical className="size-3.5" />
      </button>
      <span className="min-w-0 flex-1 truncate text-xs text-[var(--nova-text)]">{label}</span>
      {actions}
    </div>
  )
}

function MoveButton({ icon: Icon, label, disabled, onClick }: { icon: typeof ArrowUp; label: string; disabled: boolean; onClick: () => void }) {
  return (
    <Button type="button" variant="ghost" size="icon-sm" disabled={disabled} aria-label={label} onClick={onClick}>
      <Icon className="size-3.5" />
    </Button>
  )
}

function builtinGroupLabel(key: string, t: ReturnType<typeof useTranslation>['t']) {
  return ['overview', 'holdings', 'details'].includes(key)
    ? t(`storyStage.state.group.${key}`)
    : key
}
