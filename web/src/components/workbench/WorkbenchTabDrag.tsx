import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type ComponentProps,
  type CSSProperties,
  type ReactNode,
  type RefObject,
} from 'react'
import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  MouseSensor,
  TouchSensor,
  closestCenter,
  useSensor,
  useSensors,
  type Announcements,
  type ClientRect,
  type Data,
  type DragEndEvent,
  type DragMoveEvent,
  type DragStartEvent,
  type UniqueIdentifier,
} from '@dnd-kit/core'
import { sortableKeyboardCoordinates, useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'

const TAB_DRAG_MOUSE_DISTANCE_PX = 5
const TAB_DRAG_TOUCH_DELAY_MS = 250
const TAB_DRAG_TOUCH_TOLERANCE_PX = 8

interface WorkbenchTabDragContextProps {
  children: ReactNode
  onDragEnd: (event: DragEndEvent, dragRect: ClientRect | null) => void
}

interface WorkbenchTabDragPreview {
  label: string
  icon?: ReactNode
}

type HorizontalDropRect = Pick<DOMRect, 'left' | 'width'>

export type WorkbenchTabDropEdge = 'start' | 'end'

interface WorkbenchTabDropIndicator {
  edge: WorkbenchTabDropEdge
  left: number
  top: number
  height: number
}

/** Metadata for a non-tab drop target that exposes its exact rendered insertion marker. */
export interface WorkbenchTabDropTargetData {
  workbenchTabContainerId: UniqueIdentifier
  workbenchTabDropIndicatorRect: () => DOMRect | null
}

/** Resolve the rendered insertion edge from the same geometry used by the final drop. */
export function resolveWorkbenchTabDropEdge(
  activeRect: HorizontalDropRect | null,
  overRect: HorizontalDropRect | null,
): WorkbenchTabDropEdge {
  if (!activeRect || !overRect) return 'start'
  return activeRect.left + activeRect.width / 2 > overRect.left + overRect.width / 2 ? 'end' : 'start'
}

function sortableContainerId(data: Data | undefined): UniqueIdentifier | null {
  const explicit = data?.workbenchTabContainerId
  if (typeof explicit === 'string' || typeof explicit === 'number') return explicit
  const sortable = data?.sortable as { containerId?: unknown } | undefined
  return typeof sortable?.containerId === 'string' || typeof sortable?.containerId === 'number'
    ? sortable.containerId
    : null
}

/**
 * Shared sensors, floating preview, insertion feedback, and screen-reader announcements for
 * document-tab dragging. One context may span several tab strips, including split workspaces.
 */
export function WorkbenchTabDragContext({ children, onDragEnd }: WorkbenchTabDragContextProps) {
  const { t } = useTranslation()
  const [dragPreview, setDragPreview] = useState<WorkbenchTabDragPreview | null>(null)
  const [dropIndicator, setDropIndicator] = useState<WorkbenchTabDropIndicator | null>(null)
  const dragPreviewRef = useRef<HTMLDivElement | null>(null)
  const dropIndicatorFrameRef = useRef<number | null>(null)
  const dragMoveRef = useRef<DragMoveEvent | null>(null)
  const sensors = useSensors(
    useSensor(MouseSensor, {
      activationConstraint: { distance: TAB_DRAG_MOUSE_DISTANCE_PX },
    }),
    useSensor(TouchSensor, {
      activationConstraint: { delay: TAB_DRAG_TOUCH_DELAY_MS, tolerance: TAB_DRAG_TOUCH_TOLERANCE_PX },
    }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )
  const announcements = useMemo<Announcements>(() => {
    const labelOf = (item: { id: UniqueIdentifier; data: { current?: Data } }) => {
      const label = item.data.current?.label
      return typeof label === 'string' && label ? label : String(item.id)
    }
    return {
      onDragStart: ({ active }) => (
        t('tab.reorderPickedUp', { label: labelOf(active) })
      ),
      onDragOver: ({ active, over }) => (
        over ? t('tab.reorderMoving', { label: labelOf(active), target: labelOf(over) }) : undefined
      ),
      onDragEnd: ({ active, over }) => (
        over
          ? t('tab.reorderDropped', { label: labelOf(active), target: labelOf(over) })
          : t('tab.reorderCancelled', { label: labelOf(active) })
      ),
      onDragCancel: ({ active }) => (
        t('tab.reorderCancelled', { label: labelOf(active) })
      ),
    }
  }, [t])

  const cancelDropIndicatorFrame = () => {
    if (dropIndicatorFrameRef.current === null || typeof window === 'undefined') return
    window.cancelAnimationFrame(dropIndicatorFrameRef.current)
    dropIndicatorFrameRef.current = null
  }

  const scheduleDropIndicatorSync = () => {
    if (typeof window === 'undefined') return
    cancelDropIndicatorFrame()
    dropIndicatorFrameRef.current = window.requestAnimationFrame(() => {
      dropIndicatorFrameRef.current = null
      const rect = dragPreviewRef.current?.getBoundingClientRect()
      const event = dragMoveRef.current
      const indicatorRectResolver = event?.over?.data.current?.workbenchTabDropIndicatorRect
      const indicatorRect = typeof indicatorRectResolver === 'function'
        ? indicatorRectResolver() as DOMRect | null
        : null
      if (indicatorRect) {
        setDropIndicator({
          edge: 'end',
          left: indicatorRect.left,
          top: indicatorRect.top,
          height: indicatorRect.height,
        })
        return
      }
      const activeContainer = sortableContainerId(event?.active.data.current)
      const overContainer = sortableContainerId(event?.over?.data.current)
      const overSortable = event?.over?.data.current?.sortable
      if (
        !rect
        || !event?.over
        || !overSortable
        || activeContainer === null
        || overContainer === null
        || activeContainer === overContainer
      ) {
        setDropIndicator(null)
        return
      }
      const edge = resolveWorkbenchTabDropEdge(rect, event.over.rect)
      setDropIndicator({
        edge,
        left: edge === 'end' ? event.over.rect.right - 2 : event.over.rect.left,
        top: event.over.rect.top,
        height: event.over.rect.height,
      })
    })
  }

  const clearDrag = () => {
    cancelDropIndicatorFrame()
    dragMoveRef.current = null
    setDragPreview(null)
    setDropIndicator(null)
  }

  useEffect(() => () => {
    if (dropIndicatorFrameRef.current !== null) window.cancelAnimationFrame(dropIndicatorFrameRef.current)
  }, [])

  const handleDragStart = (event: DragStartEvent) => {
    const label = event.active.data.current?.label
    setDragPreview({
      label: typeof label === 'string' && label ? label : String(event.active.id),
      icon: event.active.data.current?.workbenchTabPreviewIcon as ReactNode,
    })
    scheduleDropIndicatorSync()
  }

  const handleDragMove = (event: DragMoveEvent) => {
    dragMoveRef.current = event
    scheduleDropIndicatorSync()
  }

  const handleDragEnd = (event: DragEndEvent) => {
    onDragEnd(event, dragPreviewRef.current?.getBoundingClientRect() ?? event.active.rect.current.translated)
    clearDrag()
  }

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      accessibility={{
        announcements,
        screenReaderInstructions: { draggable: t('tab.reorderInstructions') },
      }}
      onDragStart={handleDragStart}
      onDragMove={handleDragMove}
      onDragCancel={clearDrag}
      onDragEnd={handleDragEnd}
    >
      {children}
      {typeof document !== 'undefined' ? createPortal(
        <>
          <DragOverlay adjustScale={false} zIndex={1000}>
            {dragPreview ? <WorkbenchTabDragPreview {...dragPreview} nodeRef={dragPreviewRef} /> : null}
          </DragOverlay>
          {dropIndicator ? (
            <span
              data-slot="workbench-tab-drop-indicator"
              data-edge={dropIndicator.edge}
              aria-hidden="true"
              className="pointer-events-none fixed z-[1010] w-0.5 bg-[var(--nova-accent)] shadow-[0_0_8px_var(--nova-accent)]"
              style={{ left: dropIndicator.left, top: dropIndicator.top, height: dropIndicator.height }}
            />
          ) : null}
        </>,
        document.body,
      ) : null}
    </DndContext>
  )
}

interface SortableWorkbenchTabItemProps {
  id: UniqueIdentifier
  label: string
  data?: Data
  containerId?: UniqueIdentifier
  previewIcon?: ReactNode
  className?: string
  children: (dragHandleProps: Omit<ComponentProps<typeof TabsTrigger>, 'value'>) => ReactNode
}

/** A sortable flex item whose activator remains the real Radix tab trigger. */
export function SortableWorkbenchTabItem({
  id,
  label,
  data,
  containerId,
  previewIcon,
  className,
  children,
}: SortableWorkbenchTabItemProps) {
  const { t } = useTranslation()
  const {
    attributes,
    listeners,
    setActivatorNodeRef,
    setNodeRef,
    transform,
    transition,
    isDragging,
    isOver,
    active,
    over,
  } = useSortable({
    id,
    data: {
      ...data,
      label,
      workbenchTabContainerId: containerId,
      workbenchTabPreviewIcon: previewIcon,
    },
    attributes: {
      role: 'tab',
      roleDescription: t('tab.reorderRole'),
      tabIndex: 0,
    },
  })
  const style: CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
  }
  const activeContainer = sortableContainerId(active?.data.current)
  const overContainer = sortableContainerId(over?.data.current)
  const crossContainer = activeContainer !== null
    && overContainer !== null
    && activeContainer !== overContainer

  return (
    <div
      ref={setNodeRef}
      style={style}
      data-dragging={isDragging ? 'true' : undefined}
      className={cn(
        "relative h-7 min-w-24 max-w-40 flex-[1_1_10rem] self-center after:absolute after:-right-0.5 after:top-2 after:h-3 after:w-px after:rounded-full after:bg-[var(--nova-border-soft)] after:content-[''] last-of-type:after:hidden",
        isDragging && 'z-20 opacity-35',
        className,
      )}
    >
      {isOver && !isDragging && !crossContainer ? (
        <span
          data-slot="workbench-tab-drop-indicator"
          data-edge="start"
          aria-hidden="true"
          className="pointer-events-none absolute inset-y-0 left-0 z-30 w-0.5 bg-[var(--nova-accent)] shadow-[0_0_8px_var(--nova-accent)]"
        />
      ) : null}
      {children({
        ...attributes,
        ...listeners,
        ref: setActivatorNodeRef,
      })}
    </div>
  )
}

function WorkbenchTabDragPreview({
  label,
  icon,
  nodeRef,
}: WorkbenchTabDragPreview & { nodeRef: RefObject<HTMLDivElement | null> }) {
  return (
    <div
      ref={nodeRef}
      data-slot="workbench-tab-drag-overlay"
      aria-hidden="true"
      className="nova-sidebar pointer-events-none flex h-7 w-full cursor-grabbing select-none items-center gap-1.5 overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-accent)]/60 bg-[var(--nova-surface-2)] px-2.5 text-xs text-[var(--nova-text)] shadow-[0_16px_38px_rgba(0,0,0,0.3)] ring-1 ring-[var(--nova-accent)]/20"
    >
      {icon ? <span className="shrink-0 text-[var(--nova-text-faint)]">{icon}</span> : null}
      <span className="min-w-0 flex-1 truncate text-left">{label}</span>
    </div>
  )
}
