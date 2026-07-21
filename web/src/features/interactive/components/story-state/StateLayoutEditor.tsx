import { useMemo, type ReactNode, type Ref } from 'react'
import { createPortal } from 'react-dom'
import {
  DragDropContext,
  Draggable,
  Droppable,
  type DraggableProvidedDragHandleProps,
  type DraggableProvidedDraggableProps,
  type DropResult,
} from '@hello-pangea/dnd'
import { ArrowDown, ArrowLeft, ArrowRight, ArrowUp, GripVertical, RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
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

const GROUPS_DROPPABLE_ID = 'state-layout-groups'
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
  const layout = useMemo(() => reconcileStoryStateLayout(groups, value), [groups, value])
  const fieldLabels = useMemo(() => new Map(groups.flatMap((group) => group.fields.map((field) => [field.id, field.label] as const))), [groups])

  const handleDragEnd = ({ draggableId, destination, source, type }: DropResult) => {
    if (!destination) return
    if (destination.droppableId === source.droppableId && destination.index === source.index) return
    if (type === 'group') {
      const overKey = layout.groups[destination.index]?.key
      if (!overKey) return
      onChange(moveStoryStateLayoutGroup(layout, draggableId.slice('group:'.length), overKey))
      return
    }
    onChange(moveStoryStateLayoutField(layout, draggableId.slice('field:'.length), destination.droppableId, destination.index))
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
        <DragDropContext onDragEnd={handleDragEnd}>
          <Droppable
            droppableId={GROUPS_DROPPABLE_ID}
            type="group"
            renderClone={(provided, _snapshot, rubric) => {
              const group = layout.groups.find((candidate) => groupDragId(candidate.key) === rubric.draggableId)
              if (!group) return null
              return createPortal(
                <GroupSectionView
                  label={builtinGroupLabel(group.key, t)}
                  fieldCount={group.field_ids.length}
                  innerRef={provided.innerRef}
                  draggableProps={provided.draggableProps}
                  dragHandleProps={provided.dragHandleProps}
                  dragging
                >
                  <div className="space-y-1">
                    {group.field_ids.map((fieldId) => (
                      <FieldRowView key={fieldId} label={fieldLabels.get(fieldId) || fieldId} />
                    ))}
                  </div>
                </GroupSectionView>,
                document.body,
              )
            }}
          >
            {(provided) => (
              <div
                ref={provided.innerRef}
                {...provided.droppableProps}
                className="min-h-0 space-y-2 overflow-y-auto px-4 pb-2"
              >
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
                {provided.placeholder}
              </div>
            )}
          </Droppable>
        </DragDropContext>
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
  return (
    <Draggable draggableId={groupDragId(group.key)} index={groupIndex}>
      {(provided, snapshot) => (
        <GroupSectionView
          label={label}
          fieldCount={group.field_ids.length}
          gripLabel={t('storyStage.state.layout.dragGroup', { name: label })}
          innerRef={provided.innerRef}
          draggableProps={provided.draggableProps}
          dragHandleProps={provided.dragHandleProps}
          dimmed={snapshot.isDragging}
          actions={(
            <>
              <MoveButton icon={ArrowUp} label={t('storyStage.state.layout.moveGroupUp', { name: label })} disabled={groupIndex === 0} onClick={() => onMoveGroup(groupIndex, -1)} />
              <MoveButton icon={ArrowDown} label={t('storyStage.state.layout.moveGroupDown', { name: label })} disabled={groupIndex === groupCount - 1} onClick={() => onMoveGroup(groupIndex, 1)} />
            </>
          )}
        >
          <Droppable
            droppableId={group.key}
            type="field"
            renderClone={(fieldProvided, _fieldSnapshot, rubric) => {
              const fieldId = rubric.draggableId.slice('field:'.length)
              return createPortal(
                <FieldRowView
                  label={fieldLabels.get(fieldId) || fieldId}
                  innerRef={fieldProvided.innerRef}
                  draggableProps={fieldProvided.draggableProps}
                  dragHandleProps={fieldProvided.dragHandleProps}
                  dragging
                />,
                document.body,
              )
            }}
          >
            {(fieldsProvided, fieldsSnapshot) => (
              <div
                ref={fieldsProvided.innerRef}
                {...fieldsProvided.droppableProps}
                className={cn(
                  'space-y-1 rounded-lg transition-[background-color,border-color]',
                  group.field_ids.length === 0 && (fieldsSnapshot.isDraggingOver
                    ? 'min-h-10 border border-dashed border-[var(--nova-accent)]'
                    : 'flex min-h-10 items-center justify-center border border-dashed border-[var(--nova-border)]'),
                  fieldsSnapshot.isDraggingOver && 'bg-[var(--nova-active)]',
                )}
              >
                {group.field_ids.length === 0 && !fieldsSnapshot.isDraggingOver ? (
                  <span className="text-[10px] text-[var(--nova-text-faint)]">{t('storyStage.state.layout.emptyGroup')}</span>
                ) : null}
                {group.field_ids.map((fieldId, fieldIndex) => (
                  <SortableFieldRow
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
                {fieldsProvided.placeholder}
              </div>
            )}
          </Droppable>
        </GroupSectionView>
      )}
    </Draggable>
  )
}

function SortableFieldRow({ fieldId, label, groupIndex, groupCount, fieldIndex, fieldCount, onMove, onMoveAcrossGroup }: {
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
  return (
    <Draggable draggableId={fieldDragId(fieldId)} index={fieldIndex}>
      {(provided, snapshot) => (
        <FieldRowView
          label={label}
          gripLabel={t('storyStage.state.layout.dragField', { name: label })}
          innerRef={provided.innerRef}
          draggableProps={provided.draggableProps}
          dragHandleProps={provided.dragHandleProps}
          dimmed={snapshot.isDragging}
          actions={(
            <>
              <MoveButton icon={ArrowLeft} label={t('storyStage.state.layout.moveFieldPreviousGroup', { name: label })} disabled={groupIndex === 0} onClick={() => onMoveAcrossGroup(groupIndex, fieldIndex, -1)} />
              <MoveButton icon={ArrowRight} label={t('storyStage.state.layout.moveFieldNextGroup', { name: label })} disabled={groupIndex === groupCount - 1} onClick={() => onMoveAcrossGroup(groupIndex, fieldIndex, 1)} />
              <MoveButton icon={ArrowUp} label={t('storyStage.state.layout.moveFieldUp', { name: label })} disabled={fieldIndex === 0} onClick={() => onMove(groupIndex, fieldIndex, -1)} />
              <MoveButton icon={ArrowDown} label={t('storyStage.state.layout.moveFieldDown', { name: label })} disabled={fieldIndex === fieldCount - 1} onClick={() => onMove(groupIndex, fieldIndex, 1)} />
            </>
          )}
        />
      )}
    </Draggable>
  )
}

/** Presentational group card shared by the sortable row and the drag clone. */
function GroupSectionView({ label, fieldCount, gripLabel, innerRef, draggableProps, dragHandleProps, dragging, dimmed, actions, children }: {
  label: string
  fieldCount: number
  gripLabel?: string
  innerRef?: Ref<HTMLElement>
  draggableProps?: DraggableProvidedDraggableProps
  dragHandleProps?: DraggableProvidedDragHandleProps | null
  dragging?: boolean
  dimmed?: boolean
  actions?: ReactNode
  children: ReactNode
}) {
  return (
    <section
      ref={innerRef}
      {...(draggableProps ?? {})}
      className={cn(
        'rounded-xl border border-[var(--nova-border)] bg-[var(--nova-surface)] p-2 transition-[border-color,background-color,opacity,box-shadow]',
        dragging && 'border-[var(--nova-accent)] shadow-xl',
        dimmed && 'opacity-40',
      )}
    >
      <header className="mb-2 flex min-w-0 items-center gap-1.5">
        <button
          type="button"
          {...(dragHandleProps ?? {})}
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

/** Presentational field row shared by the sortable row and the drag clone. */
function FieldRowView({ label, gripLabel, innerRef, draggableProps, dragHandleProps, dragging, dimmed, actions }: {
  label: string
  gripLabel?: string
  innerRef?: Ref<HTMLDivElement>
  draggableProps?: DraggableProvidedDraggableProps
  dragHandleProps?: DraggableProvidedDragHandleProps | null
  dragging?: boolean
  dimmed?: boolean
  actions?: ReactNode
}) {
  return (
    <div
      ref={innerRef}
      {...(draggableProps ?? {})}
      className={cn(
        'flex min-w-0 items-center gap-1 rounded-lg border border-[var(--nova-border-soft)] bg-[var(--nova-surface-2)] px-1.5 py-1 transition-[border-color,background-color,opacity,box-shadow]',
        dragging && 'border-[var(--nova-accent)] shadow-lg',
        dimmed && 'opacity-40',
      )}
    >
      <button
        type="button"
        {...(dragHandleProps ?? {})}
        className="flex size-7 shrink-0 cursor-grab items-center justify-center rounded-md text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] active:cursor-grabbing"
        aria-label={gripLabel}
      >
        <GripVertical className="size-3.5" />
      </button>
      <span className="min-w-0 flex-1 truncate text-xs text-[var(--nova-text)]" title={label}>{label}</span>
      {actions}
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

function builtinGroupLabel(key: string, t: ReturnType<typeof useTranslation>['t']) {
  return ['overview', 'holdings', 'details'].includes(key)
    ? t(`storyStage.state.group.${key}`)
    : key
}
