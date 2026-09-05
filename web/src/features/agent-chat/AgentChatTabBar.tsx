import { useRef, useState, type ReactNode } from 'react'
import { useDroppable } from '@dnd-kit/core'
import { SortableContext, horizontalListSortingStrategy } from '@dnd-kit/sortable'
import { useTranslation } from 'react-i18next'
import {
  Bot,
  FileDiff,
  FolderTree,
  MessageCircle,
  Pencil,
  Pin,
  PinOff,
  SplitSquareHorizontal,
  TerminalSquare,
  X,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuSeparator, ContextMenuTrigger } from '@/components/ui/context-menu'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { SortableWorkbenchTabItem } from '@/components/workbench/WorkbenchTabDrag'
import {
  WorkbenchTab,
  WorkbenchTabAddButton,
  WorkbenchTabStrip,
} from '@/components/workbench/WorkbenchTabStrip'
import { cn } from '@/lib/utils'
import { AGENT_CHAT_PAGE_ICONS, AgentChatNewTabMenuItems } from './AgentChatNewTabMenuItems'
import {
  agentChatTabSortableId,
  agentChatTabStripDropId,
  type AgentChatTabDragData,
  type AgentChatTabStripDropData,
} from './AgentChatTabDragContext'
import {
  type AgentChatGroupId,
  type AgentChatPageId,
  type AgentChatTab,
  type TerminalCommandProfile,
  type TerminalProfileId,
} from './types'

interface AgentChatTabBarProps {
  /** Stable owner of this independently persisted workbench. */
  projectId: string
  /** Which side of the split this strip drives. */
  group: AgentChatGroupId
  /** Tabs of this group only, already in display order. */
  tabs: AgentChatTab[]
  activeTabId: string | null
  /** Resolves what a tab is called, user override included. */
  tabTitle: (tab: AgentChatTab) => string
  /** No project means there is nothing to start a conversation in yet. */
  newChatDisabled?: boolean
  terminalCommands: TerminalCommandProfile[]
  /** Page capabilities valid for this Project type. */
  pageIds: readonly AgentChatPageId[]
  /** Persistent controls remain reachable when document tabs overflow. */
  endActions?: ReactNode
  onActivate: (tabId: string) => void
  onClose: (tabId: string) => void
  onCloseOthers: (tabId: string) => void
  onCloseToRight: (tabId: string) => void
  onRename: (tabId: string, title: string) => void
  onTogglePin: (tabId: string) => void
  /** Reorder inside the strip, or hand the tab to the other side of the split. */
  onMoveTab: (sourceId: string, group: AgentChatGroupId, beforeId: string | null) => void
  onNewAgentTab: (group: AgentChatGroupId) => void
  onNewTerminalTab: (group: AgentChatGroupId, profileId: TerminalProfileId, profileName?: string) => void
  onOpenFiles: (group: AgentChatGroupId) => void
  onOpenPage: (group: AgentChatGroupId, pageId: AgentChatPageId) => void
}

/** The group a tab moves to when it is sent across the split. */
function oppositeGroup(group: AgentChatGroupId): AgentChatGroupId {
  return group === 'primary' ? 'secondary' : 'primary'
}

export function AgentChatTabBar({
  projectId,
  group,
  tabs,
  activeTabId,
  tabTitle,
  newChatDisabled = false,
  terminalCommands,
  pageIds,
  endActions,
  onActivate,
  onClose,
  onCloseOthers,
  onCloseToRight,
  onRename,
  onTogglePin,
  onMoveTab,
  onNewAgentTab,
  onNewTerminalTab,
  onOpenFiles,
  onOpenPage,
}: AgentChatTabBarProps) {
  const { t } = useTranslation()
  const [renaming, setRenaming] = useState<{
    id: string
    value: string
  } | null>(null)
  const stripDropIndicatorRef = useRef<HTMLSpanElement | null>(null)
  const stripDropId = agentChatTabStripDropId(projectId, group)
  const stripDropData = {
    kind: 'agent-chat-tab-strip',
    projectId,
    group,
    label: t(group === 'primary' ? 'agentChat.tabs.primaryWorkspace' : 'agentChat.tabs.secondaryWorkspace'),
    workbenchTabContainerId: stripDropId,
    workbenchTabDropIndicatorRect: () => stripDropIndicatorRef.current?.getBoundingClientRect() ?? null,
  } satisfies AgentChatTabStripDropData
  const { isOver: isStripDropTarget, setNodeRef: setStripDropRef } = useDroppable({
    id: stripDropId,
    data: stripDropData,
  })
  const tabIcon = (tab: AgentChatTab) => {
    switch (tab.kind) {
      case 'agent':
        return <MessageCircle className="size-3.5" />
      case 'subagent':
        return <Bot className="size-3.5" />
      case 'terminal':
        return <TerminalSquare className="size-3.5" />
      case 'files':
        return <FolderTree className="size-3.5" />
      case 'page':
        return AGENT_CHAT_PAGE_ICONS[tab.pageId]
      case 'review':
        return <FileDiff className="size-3.5" />
    }
  }

  const submitRename = () => {
    if (!renaming) return
    onRename(renaming.id, renaming.value)
    setRenaming(null)
  }

  /**
   * The new-tab button trails the last tab so it reads as "add one more here". Once the strip
   * scrolls it would be scrolled out of reach, so it pins to the end of the bar instead.
   */
  const newTabMenu = (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <WorkbenchTabAddButton aria-label={t('agentChat.tabs.new')} />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-48">
        <AgentChatNewTabMenuItems
          group={group}
          newChatDisabled={newChatDisabled}
          terminalCommands={terminalCommands}
          pageIds={pageIds}
          onNewAgentTab={onNewAgentTab}
          onNewTerminalTab={onNewTerminalTab}
          onOpenFiles={onOpenFiles}
          onOpenPage={onOpenPage}
        />
      </DropdownMenuContent>
    </DropdownMenu>
  )

  return (
    <>
      <div ref={setStripDropRef} className="h-full">
        <SortableContext
          id={stripDropId}
          items={tabs.map((tab) => agentChatTabSortableId(projectId, tab.id))}
          strategy={horizontalListSortingStrategy}
        >
          <WorkbenchTabStrip
            value={activeTabId ?? ''}
            onValueChange={onActivate}
            flowAction={newTabMenu}
            endActions={endActions}
            endActionsVariant="inline"
            // Tabs sit flush against the pane edge; only the trailing new-tab button gets breathing room.
            className={cn('pr-1', isStripDropTarget && 'bg-[var(--nova-hover)]')}
          >
            {tabs.map((tab) => {
              const label = tabTitle(tab)
              const icon = tabIcon(tab)
              const dragData = {
                kind: 'agent-chat-tab',
                projectId,
                group,
                tabId: tab.id,
                label,
              } satisfies AgentChatTabDragData
              return (
                <SortableWorkbenchTabItem
                  key={tab.id}
                  id={agentChatTabSortableId(projectId, tab.id)}
                  label={label}
                  data={dragData}
                  containerId={stripDropId}
                  previewIcon={icon}
                >
                  {(dragHandleProps) => (
                    <ContextMenu>
                      <ContextMenuTrigger asChild>
                        <WorkbenchTab
                          {...dragHandleProps}
                          value={tab.id}
                          label={label}
                          icon={icon}
                          className="h-full w-full min-w-0 max-w-none flex-none"
                          trailing={
                            tab.pinned ? (
                              <Pin className="size-3 shrink-0 text-[var(--nova-text-faint)]" aria-hidden="true" />
                            ) : (
                              <span
                                role="button"
                                tabIndex={-1}
                                aria-label={t('agentChat.tabs.close', { title: label })}
                                data-slot="workbench-tab-close"
                                className="pointer-events-none -ml-1.5 grid h-4 w-0 shrink-0 place-items-center overflow-hidden rounded-sm opacity-0 transition-[width,margin,opacity,background-color] group-hover/tab:pointer-events-auto group-hover/tab:ml-0 group-hover/tab:w-4 group-hover/tab:opacity-100 group-aria-[selected=true]/tab:pointer-events-auto group-aria-[selected=true]/tab:ml-0 group-aria-[selected=true]/tab:w-4 group-aria-[selected=true]/tab:opacity-100 max-md:pointer-events-auto max-md:ml-0 max-md:w-4 max-md:opacity-100 hover:bg-[var(--nova-hover)]"
                                // Radix activates a trigger on pointer down, so the close hit area has to
                                // stop the event or closing a background tab would select it first.
                                onPointerDown={(event) => event.stopPropagation()}
                                onClick={(event) => {
                                  event.stopPropagation()
                                  onClose(tab.id)
                                }}
                              >
                                <X className="size-3" />
                              </span>
                            )
                          }
                          onDoubleClick={tab.kind === 'subagent' ? undefined : () => setRenaming({ id: tab.id, value: label })}
                        />
                      </ContextMenuTrigger>
                      <ContextMenuContent className="min-w-44">
                        {tab.kind !== 'subagent' ? (
                          <>
                            <ContextMenuItem onSelect={() => setRenaming({ id: tab.id, value: label })}>
                              <Pencil />
                              {t('agentChat.tabs.rename')}
                            </ContextMenuItem>
                            <ContextMenuItem onSelect={() => onTogglePin(tab.id)}>
                              {tab.pinned ? <PinOff /> : <Pin />}
                              {tab.pinned ? t('agentChat.tabs.unpin') : t('agentChat.tabs.pin')}
                            </ContextMenuItem>
                          </>
                        ) : null}
                        <ContextMenuItem onSelect={() => onMoveTab(tab.id, oppositeGroup(group), null)}>
                          <SplitSquareHorizontal />
                          {group === 'primary' ? t('agentChat.tabs.moveRight') : t('agentChat.tabs.moveLeft')}
                        </ContextMenuItem>
                        <ContextMenuSeparator />
                        <ContextMenuItem onSelect={() => onClose(tab.id)}>{t('agentChat.tabs.closeTab')}</ContextMenuItem>
                        <ContextMenuItem onSelect={() => onCloseOthers(tab.id)}>{t('agentChat.tabs.closeOthers')}</ContextMenuItem>
                        <ContextMenuItem onSelect={() => onCloseToRight(tab.id)}>{t('agentChat.tabs.closeToRight')}</ContextMenuItem>
                      </ContextMenuContent>
                    </ContextMenu>
                  )}
                </SortableWorkbenchTabItem>
              )
            })}
            {isStripDropTarget ? (
              <span
                ref={stripDropIndicatorRef}
                data-slot="workbench-tab-strip-drop-indicator"
                aria-hidden="true"
                className="h-full w-0.5 shrink-0 opacity-0"
              />
            ) : null}
          </WorkbenchTabStrip>
        </SortableContext>
      </div>

      <Dialog
        open={renaming !== null}
        onOpenChange={(open) => {
          if (!open) setRenaming(null)
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t('agentChat.tabs.renameTitle')}</DialogTitle>
            <DialogDescription>{t('agentChat.tabs.renameDescription')}</DialogDescription>
          </DialogHeader>
          <Input
            autoFocus
            value={renaming?.value ?? ''}
            onChange={(event) => setRenaming((current) => (current ? { ...current, value: event.target.value } : current))}
            onKeyDown={(event) => {
              if (event.key === 'Enter') submitRename()
            }}
            aria-label={t('agentChat.tabs.renameTitle')}
          />
          <DialogFooter>
            <Button type="button" variant="ghost" size="sm" onClick={() => setRenaming(null)}>
              {t('common.cancel')}
            </Button>
            <Button type="button" size="sm" onClick={submitRename}>
              {t('common.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
