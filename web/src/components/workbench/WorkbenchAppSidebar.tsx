import { useState, type KeyboardEventHandler, type PointerEventHandler, type ReactNode } from 'react'
import { DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { EyeOff, PanelLeft, Settings, SlidersHorizontal } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuSeparator, ContextMenuTrigger } from '@/components/ui/context-menu'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { verticalAxisModifiers } from '@/lib/dnd'
import { cn } from '@/lib/utils'
import { desktopBridge } from '@/lib/desktop'
import { WorkbenchSidebarCustomizationDialog } from './WorkbenchSidebarCustomizationDialog'

export interface WorkbenchSidebarItem {
  id: string
  label: string
  icon: ReactNode
  active: boolean
  onClick: () => void
}

interface WorkbenchAppSidebarProps {
  expanded: boolean
  activityOrderScope: string
  activityItems: WorkbenchSidebarItem[]
  customizationItems: WorkbenchSidebarItem[]
  hiddenActivityIDs: string[]
  dragDisabled: boolean
  contextSwitcher: ReactNode
  notice?: ReactNode
  messageCenter: ReactNode
  sidebarLabel: string
  settingsLabel: string
  settingsActive: boolean
  toggleLabel: string
  resizeLabel: string
  maxWidth: number
  currentWidth: number
  onOpenSettings: () => void
  onToggle: () => void
  onReorder: (activeID: string, overID: string) => void
  onCustomizationReorder: (activeID: string, overID: string) => void
  onActivityVisibilityChange: (id: string, visible: boolean) => void
  onResizePointerDown: PointerEventHandler<HTMLDivElement>
  onResizeKeyDown: KeyboardEventHandler<HTMLDivElement>
}

/** Full-height application navigation; project structure remains a separate context pane. */
export function WorkbenchAppSidebar({
  expanded,
  activityOrderScope,
  activityItems,
  customizationItems,
  hiddenActivityIDs,
  dragDisabled,
  contextSwitcher,
  notice,
  messageCenter,
  sidebarLabel,
  settingsLabel,
  settingsActive,
  toggleLabel,
  resizeLabel,
  maxWidth,
  currentWidth,
  onOpenSettings,
  onToggle,
  onReorder,
  onCustomizationReorder,
  onActivityVisibilityChange,
  onResizePointerDown,
  onResizeKeyDown,
}: WorkbenchAppSidebarProps) {
  const { t } = useTranslation()
  const [customizationOpen, setCustomizationOpen] = useState(false)
  const [contextActivityID, setContextActivityID] = useState<string | null>(null)
  const sensors = useSensors(
    // Keep the complete row draggable while allowing small pointer movement during clicks.
    useSensor(PointerSensor, { activationConstraint: { distance: 10 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )
  const sortableIDs = activityItems.map((item) => sortableActivityID(activityOrderScope, item.id))
  const contextActivity = activityItems.find((item) => item.id === contextActivityID)

  const handleDragEnd = (event: DragEndEvent) => {
    const activeID = activityIDFromSortable(event.active.id, activityOrderScope)
    const overID = activityIDFromSortable(event.over?.id, activityOrderScope)
    if (activeID && overID && activeID !== overID) onReorder(activeID, overID)
  }

  return (
    <>
      <DndContext key={activityOrderScope} sensors={sensors} collisionDetection={closestCenter} modifiers={verticalAxisModifiers} onDragEnd={handleDragEnd}>
        <ContextMenu>
          <ContextMenuTrigger asChild>
            <Sidebar
              collapsible="icon"
              data-slot="sidebar-container"
              className="nova-activity-bar select-none border-r-0! text-[var(--nova-text-muted)]"
              role="navigation"
              aria-label={sidebarLabel}
              onContextMenuCapture={(event) => {
                const target = event.target instanceof Element ? event.target.closest<HTMLElement>('[data-activity-id]') : null
                setContextActivityID(target?.dataset.activityId || null)
              }}
            >
              <SidebarHeader className="gap-2 p-2">
                <ActivityBarBrandRow expanded={expanded} toggleLabel={toggleLabel} onToggle={onToggle} />
                {contextSwitcher}
              </SidebarHeader>
              <SidebarContent>
                <SidebarGroup className="p-2">
                  <SidebarMenu className="gap-0.5">
                    <SortableContext key={activityOrderScope} items={sortableIDs} strategy={verticalListSortingStrategy}>
                      {activityItems.map((item) => {
                        const sortableID = sortableActivityID(activityOrderScope, item.id)
                        return (
                          <SidebarMenuItem key={sortableID}>
                            <SortableActivityButton
                              id={sortableID}
                              activityID={item.id}
                              dragDisabled={dragDisabled}
                              expanded={expanded}
                              label={item.label}
                              onClick={item.onClick}
                              active={item.active}
                            >
                              {item.icon}
                            </SortableActivityButton>
                          </SidebarMenuItem>
                        )
                      })}
                    </SortableContext>
                  </SidebarMenu>
                </SidebarGroup>
              </SidebarContent>
              <SidebarFooter className="gap-1 p-2">
                {notice}
                <SidebarMenu className="gap-0.5">
                  <SidebarMenuItem>{messageCenter}</SidebarMenuItem>
                  <SidebarMenuItem>
                    <ActivityButton expanded={expanded} label={settingsLabel} onClick={onOpenSettings} active={settingsActive} data-onboarding-anchor="activity-settings">
                      <Settings data-icon="inline-start" />
                    </ActivityButton>
                  </SidebarMenuItem>
                </SidebarMenu>
                <div
                  role="separator"
                  tabIndex={0}
                  aria-label={resizeLabel}
                  aria-orientation="vertical"
                  aria-valuemin={0}
                  aria-valuemax={maxWidth}
                  aria-valuenow={expanded ? Math.round(currentWidth) : 0}
                  aria-valuetext={expanded ? t('workbench.sidebar.expandedWidth', { width: Math.round(currentWidth) }) : t('workbench.sidebar.collapsed')}
                  title={resizeLabel}
                  className="nova-activity-bar-resize-handle"
                  onPointerDown={onResizePointerDown}
                  onKeyDown={onResizeKeyDown}
                />
              </SidebarFooter>
            </Sidebar>
          </ContextMenuTrigger>
          <ContextMenuContent className="min-w-48">
            <ContextMenuItem onSelect={onToggle}>
              <PanelLeft />
              {expanded ? t('workbench.sidebar.collapseAction') : t('workbench.sidebar.expandAction')}
            </ContextMenuItem>
            <ContextMenuSeparator />
            {contextActivity ? (
              <>
                <ContextMenuItem
                  disabled={activityItems.length === 1}
                  onSelect={() => onActivityVisibilityChange(contextActivity.id, false)}
                >
                  <EyeOff />
                  {t('workbench.sidebar.hideAction')}
                </ContextMenuItem>
                <ContextMenuSeparator />
              </>
            ) : null}
            <ContextMenuItem onSelect={() => setCustomizationOpen(true)}>
              <SlidersHorizontal />
              {t('workbench.sidebar.customizeAction')}
            </ContextMenuItem>
          </ContextMenuContent>
        </ContextMenu>
      </DndContext>
      <WorkbenchSidebarCustomizationDialog
        open={customizationOpen}
        items={customizationItems}
        hiddenItemIDs={hiddenActivityIDs}
        onOpenChange={setCustomizationOpen}
        onReorder={onCustomizationReorder}
        onVisibilityChange={onActivityVisibilityChange}
      />
    </>
  )
}

export function WorkbenchBrandIcon() {
  return <img src="/favicon.svg" alt="Denova" className="size-6 shrink-0 rounded-[7px]" draggable={false} />
}

/** Desktop uses the context menu and resize rail; Web retains its visible toggle. */
function ActivityBarBrandRow({
  expanded,
  toggleLabel,
  onToggle,
}: Pick<WorkbenchAppSidebarProps, 'expanded' | 'toggleLabel' | 'onToggle'>) {
  const desktop = desktopBridge()
  // Windows already presents the brand in its shared titlebar.
  if (desktop?.platform === 'win32') return null
  // MyGo supplies the native control area; keep a real element for dragging.
  if (desktop?.platform === 'darwin') return <div className="nova-activity-brand-row" aria-hidden="true" />
  if (desktop) {
    return (
      <div className="nova-activity-brand-row flex h-8 items-center gap-2">
        <span className="flex size-8 shrink-0 items-center justify-center"><WorkbenchBrandIcon /></span>
        {expanded && <span className="truncate text-xs font-semibold tracking-wide text-[var(--nova-text)]">Denova</span>}
      </div>
    )
  }
  const toggle = (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={toggleLabel}
          data-activity-bar-toggle="true"
          className="size-8 rounded-[var(--nova-radius)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] focus-visible:ring-[var(--nova-field-focus-border)]"
          onClick={onToggle}
        >
          <PanelLeft />
        </Button>
      </TooltipTrigger>
      <TooltipContent side={expanded ? 'bottom' : 'right'} align="center">
        {toggleLabel}
      </TooltipContent>
    </Tooltip>
  )

  if (expanded) {
    return (
      <div className="nova-activity-brand-row grid h-8 grid-cols-[2rem_minmax(0,1fr)_2rem] items-center gap-2">
        <span className="nova-activity-brand-icon flex size-8 items-center justify-center"><WorkbenchBrandIcon /></span>
        <span className="truncate text-xs font-semibold tracking-wide text-[var(--nova-text)]">Denova</span>
        {toggle}
      </div>
    )
  }

  return (
    <div className="nova-activity-brand-row group/activity-toggle relative flex h-8 items-center justify-center">
      <span className="nova-activity-brand-icon pointer-events-none flex size-8 items-center justify-center opacity-100 transition-opacity group-hover/activity-toggle:opacity-0 group-focus-within/activity-toggle:opacity-0 motion-reduce:transition-none">
        <WorkbenchBrandIcon />
      </span>
      <span className="nova-activity-brand-toggle absolute inset-0 flex items-center justify-center opacity-0 transition-opacity group-hover/activity-toggle:opacity-100 group-focus-within/activity-toggle:opacity-100 motion-reduce:transition-none">
        {toggle}
      </span>
    </div>
  )
}

function SortableActivityButton({
  id,
  activityID,
  dragDisabled,
  ...props
}: Omit<React.ComponentProps<'button'>, 'id'> & {
  id: string
  activityID: string
  dragDisabled: boolean
  expanded: boolean
  label: string
  children: ReactNode
  active: boolean
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id, disabled: dragDisabled })
  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={isDragging ? 'relative z-20 opacity-80' : undefined}
    >
      <ActivityButton
        data-activity-id={activityID}
        data-onboarding-anchor={`activity-${activityID}`}
        {...(dragDisabled ? {} : attributes)}
        {...(dragDisabled ? {} : listeners)}
        {...props}
      />
    </div>
  )
}

function ActivityButton({
  expanded,
  label,
  children,
  className,
  active = false,
  ...props
}: React.ComponentProps<'button'> & {
  expanded: boolean
  label: string
  children: ReactNode
  active?: boolean
}) {
  return (
    <SidebarMenuButton
      tooltip={!expanded ? label : undefined}
      isActive={active}
      className={cn(
        'relative h-9 gap-2 text-xs text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] data-[active=true]:bg-[var(--nova-active)] data-[active=true]:text-[var(--nova-text)]',
        desktopBridge()
          ? 'group-data-[collapsible=icon]:h-12! group-data-[collapsible=icon]:w-full! group-data-[collapsible=icon]:p-1!'
          : 'group-data-[collapsible=icon]:h-9! group-data-[collapsible=icon]:w-8!',
        className,
      )}
      {...props}
      aria-current={active ? 'page' : undefined}
    >
      <span className="relative z-10 flex size-4 shrink-0 items-center justify-center">{children}</span>
      <span className="relative z-10 min-w-0 truncate text-left font-medium">{label}</span>
    </SidebarMenuButton>
  )
}

function sortableActivityID(scope: string, activityID: string) {
  return `${scope}:${activityID}`
}

function activityIDFromSortable(value: unknown, scope: string) {
  if (typeof value !== 'string') return null
  const prefix = `${scope}:`
  return value.startsWith(prefix) ? value.slice(prefix.length) : null
}
