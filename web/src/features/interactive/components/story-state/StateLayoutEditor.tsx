import { useMemo } from 'react'
import {
  closestCenter,
  DndContext,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { ArrowDown, ArrowLeft, ArrowRight, ArrowUp, GripVertical, RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  moveStoryStateLayoutField,
  moveStoryStateLayoutGroup,
  reconcileStoryStateLayout,
  type StoryStateTemplateLayout,
} from './layout-preference'
import type { LedgerFieldGroup } from './model'

const groupDragId = (key: string) => `group:${key}`
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
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )
  const layout = useMemo(() => reconcileStoryStateLayout(groups, value), [groups, value])
  const fieldLabels = useMemo(() => new Map(groups.flatMap((group) => group.fields.map((field) => [field.id, field.label] as const))), [groups])

  const handleDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return
    const activeId = String(active.id)
    const overId = String(over.id)
    if (activeId.startsWith('group:')) {
      const activeKey = activeId.slice('group:'.length)
      const overKey = overId.startsWith('group:')
        ? overId.slice('group:'.length)
        : groupForField(layout, overId.slice('field:'.length))
      if (overKey) onChange(moveStoryStateLayoutGroup(layout, activeKey, overKey))
      return
    }
    if (!activeId.startsWith('field:')) return
    const fieldId = activeId.slice('field:'.length)
    const targetGroup = overId.startsWith('group:')
      ? overId.slice('group:'.length)
      : groupForField(layout, overId.slice('field:'.length))
    if (!targetGroup) return
    const targetFields = layout.groups.find((group) => group.key === targetGroup)?.field_ids || []
    const targetIndex = overId.startsWith('field:')
      ? Math.max(0, targetFields.indexOf(overId.slice('field:'.length)))
      : targetFields.length
    onChange(moveStoryStateLayoutField(layout, fieldId, targetGroup, targetIndex))
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
    const group = layout.groups[groupIndex]
    const fieldId = group?.field_ids[fieldIndex]
    const target = layout.groups[groupIndex + delta]
    if (!fieldId || !target) return
    onChange(moveStoryStateLayoutField(layout, fieldId, target.key, target.field_ids.length))
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} className="max-h-[min(90dvh,48rem)] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden p-0 sm:max-w-[42rem]">
        <DialogHeader className="px-4 pt-4">
          <DialogTitle>{t('storyStage.state.layout.title', { name: title })}</DialogTitle>
          <DialogDescription>{t('storyStage.state.layout.description')}</DialogDescription>
        </DialogHeader>
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
          <SortableContext items={layout.groups.map((group) => groupDragId(group.key))} strategy={verticalListSortingStrategy}>
            <div className="min-h-0 space-y-2 overflow-y-auto px-4 pb-2">
              {layout.groups.map((group, groupIndex) => (
                <SortableLayoutGroup
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
            </div>
          </SortableContext>
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

function SortableLayoutGroup({ group, groupIndex, groupCount, fieldLabels, onMoveGroup, onMoveField, onMoveFieldAcrossGroup }: {
  group: StoryStateTemplateLayout['groups'][number]
  groupIndex: number
  groupCount: number
  fieldLabels: Map<string, string>
  onMoveGroup: (groupIndex: number, delta: number) => void
  onMoveField: (groupIndex: number, fieldIndex: number, delta: number) => void
  onMoveFieldAcrossGroup: (groupIndex: number, fieldIndex: number, delta: number) => void
}) {
  const { t } = useTranslation()
  const sortable = useSortable({ id: groupDragId(group.key) })
  const style = { transform: CSS.Transform.toString(sortable.transform), transition: sortable.transition }
  const label = builtinGroupLabel(group.key, t)
  return (
    <section ref={sortable.setNodeRef} style={style} className="rounded-xl border border-[var(--nova-border)] bg-[var(--nova-surface)] p-2">
      <header className="mb-2 flex min-w-0 items-center gap-1.5">
        <button
          ref={sortable.setActivatorNodeRef}
          type="button"
          className="flex size-7 shrink-0 cursor-grab items-center justify-center rounded-md text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] active:cursor-grabbing"
          aria-label={t('storyStage.state.layout.dragGroup', { name: label })}
          {...sortable.attributes}
          {...sortable.listeners}
        >
          <GripVertical className="size-4" />
        </button>
        <h3 className="min-w-0 flex-1 truncate text-xs font-semibold text-[var(--nova-text)]">{label}</h3>
        <span className="text-[10px] text-[var(--nova-text-faint)]">{group.field_ids.length}</span>
        <MoveButton icon={ArrowUp} label={t('storyStage.state.layout.moveGroupUp', { name: label })} disabled={groupIndex === 0} onClick={() => onMoveGroup(groupIndex, -1)} />
        <MoveButton icon={ArrowDown} label={t('storyStage.state.layout.moveGroupDown', { name: label })} disabled={groupIndex === groupCount - 1} onClick={() => onMoveGroup(groupIndex, 1)} />
      </header>
      <SortableContext items={group.field_ids.map(fieldDragId)} strategy={verticalListSortingStrategy}>
        <div className="space-y-1">
          {group.field_ids.map((fieldId, fieldIndex) => (
            <SortableLayoutField
              key={fieldId}
              fieldId={fieldId}
              label={fieldLabels.get(fieldId) || fieldId}
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
    </section>
  )
}

function SortableLayoutField({ fieldId, label, groupIndex, groupCount, fieldIndex, fieldCount, onMove, onMoveAcrossGroup }: {
  fieldId: string
  label: string
  groupIndex: number
  groupCount: number
  fieldIndex: number
  fieldCount: number
  onMove: (groupIndex: number, fieldIndex: number, delta: number) => void
  onMoveAcrossGroup: (groupIndex: number, fieldIndex: number, delta: number) => void
}) {
  const { t } = useTranslation()
  const sortable = useSortable({ id: fieldDragId(fieldId) })
  const style = { transform: CSS.Transform.toString(sortable.transform), transition: sortable.transition }
  return (
    <div ref={sortable.setNodeRef} style={style} className="flex min-w-0 items-center gap-1 rounded-lg border border-[var(--nova-border-soft)] bg-[var(--nova-surface-2)] px-1.5 py-1">
      <button
        ref={sortable.setActivatorNodeRef}
        type="button"
        className="flex size-7 shrink-0 cursor-grab items-center justify-center rounded-md text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] active:cursor-grabbing"
        aria-label={t('storyStage.state.layout.dragField', { name: label })}
        {...sortable.attributes}
        {...sortable.listeners}
      >
        <GripVertical className="size-3.5" />
      </button>
      <span className="min-w-0 flex-1 truncate text-xs text-[var(--nova-text)]" title={label}>{label}</span>
      <MoveButton icon={ArrowLeft} label={t('storyStage.state.layout.moveFieldPreviousGroup', { name: label })} disabled={groupIndex === 0} onClick={() => onMoveAcrossGroup(groupIndex, fieldIndex, -1)} />
      <MoveButton icon={ArrowRight} label={t('storyStage.state.layout.moveFieldNextGroup', { name: label })} disabled={groupIndex === groupCount - 1} onClick={() => onMoveAcrossGroup(groupIndex, fieldIndex, 1)} />
      <MoveButton icon={ArrowUp} label={t('storyStage.state.layout.moveFieldUp', { name: label })} disabled={fieldIndex === 0} onClick={() => onMove(groupIndex, fieldIndex, -1)} />
      <MoveButton icon={ArrowDown} label={t('storyStage.state.layout.moveFieldDown', { name: label })} disabled={fieldIndex === fieldCount - 1} onClick={() => onMove(groupIndex, fieldIndex, 1)} />
    </div>
  )
}

function MoveButton({ icon: Icon, label, disabled, onClick }: { icon: typeof ArrowUp; label: string; disabled: boolean; onClick: () => void }) {
  return (
    <Button type="button" variant="ghost" size="icon-sm" disabled={disabled} aria-label={label} title={label} onClick={onClick}>
      <Icon className="size-3.5" />
    </Button>
  )
}

function groupForField(layout: StoryStateTemplateLayout, fieldId: string) {
  return layout.groups.find((group) => group.field_ids.includes(fieldId))?.key || ''
}

function builtinGroupLabel(key: string, t: ReturnType<typeof useTranslation>['t']) {
  return ['overview', 'holdings', 'details', 'spoiler'].includes(key)
    ? t(`storyStage.state.group.${key}`)
    : key
}
