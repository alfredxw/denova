import { useEffect, useRef, useState, type DragEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Bot,
  BookOpen,
  Clock3,
  Database,
  FileDiff,
  MessageSquareText,
  Pencil,
  Pin,
  PinOff,
  Plus,
  SlidersHorizontal,
  Sparkles,
  SplitSquareHorizontal,
  TerminalSquare,
  X,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  AGENT_CHAT_PAGE_IDS,
  type AgentChatGroupId,
  type AgentChatPageId,
  type AgentChatTab,
  type TerminalProfileId,
} from './types'

/**
 * Private mime type for tab drags. `dataTransfer.getData` is unreadable during `dragover`, so
 * the type itself is what tells a strip whether the thing being dragged is one of its tabs.
 */
const TAB_DRAG_MIME = 'application/x-nova-agentchat-tab'

interface AgentChatTabBarProps {
  /** Which side of the split this strip drives. */
  group: AgentChatGroupId
  /** Tabs of this group only, already in display order. */
  tabs: AgentChatTab[]
  activeTabId: string | null
  /** Resolves what a tab is called, user override included. */
  tabTitle: (tab: AgentChatTab) => string
  /** No project means there is nothing to start a conversation in yet. */
  newChatDisabled?: boolean
  onActivate: (tabId: string) => void
  onClose: (tabId: string) => void
  onCloseOthers: (tabId: string) => void
  onCloseToRight: (tabId: string) => void
  onRename: (tabId: string, title: string) => void
  onTogglePin: (tabId: string) => void
  /** Reorder inside the strip, or hand the tab to the other side of the split. */
  onMoveTab: (sourceId: string, group: AgentChatGroupId, beforeId: string | null) => void
  onNewAgentTab: (group: AgentChatGroupId) => void
  onNewTerminalTab: (group: AgentChatGroupId, profileId: TerminalProfileId, command?: string) => void
  onOpenPage: (group: AgentChatGroupId, pageId: AgentChatPageId) => void
}

/** Icon per project page, kept beside the page ids so a new page cannot ship without one. */
const PAGE_ICONS: Record<AgentChatPageId, ReactNode> = {
  reader: <BookOpen className="size-3.5" />,
  lore: <Database className="size-3.5" />,
  presets: <SlidersHorizontal className="size-3.5" />,
  skills: <Sparkles className="size-3.5" />,
  agents: <Bot className="size-3.5" />,
  automations: <Clock3 className="size-3.5" />,
}

/**
 * Editor-style tab chrome on top of the shared Tabs primitive: square tabs divided by a hairline,
 * and the active one painted in the content background so it reads as the front of the pane. The
 * shadcn defaults are overridden rather than avoided — their pill-on-muted look belongs to a
 * segmented control, not to a strip of open documents, and its colours ignore the workbench theme.
 */
const TAB_TRIGGER_CLASS = [
  'group/tab relative h-full max-w-56 flex-none justify-start gap-1.5 px-3 text-xs',
  'rounded-none border-0 border-r border-[var(--nova-border)] after:hidden',
  'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]',
  // ContextMenuTrigger also owns `data-state` and overwrites the tab's active/closed value when
  // composed with asChild. `aria-selected` belongs to the Tabs contract and remains unambiguous.
  'aria-[selected=true]:bg-[var(--nova-active)] aria-[selected=true]:text-[var(--nova-text)]',
  'dark:aria-[selected=true]:border-[var(--nova-border)] dark:aria-[selected=true]:bg-[var(--nova-active)] dark:aria-[selected=true]:text-[var(--nova-text)]',
].join(' ')

/** The group a tab moves to when it is sent across the split. */
function oppositeGroup(group: AgentChatGroupId): AgentChatGroupId {
  return group === 'primary' ? 'secondary' : 'primary'
}

export function AgentChatTabBar({
  group,
  tabs,
  activeTabId,
  tabTitle,
  newChatDisabled = false,
  onActivate,
  onClose,
  onCloseOthers,
  onCloseToRight,
  onRename,
  onTogglePin,
  onMoveTab,
  onNewAgentTab,
  onNewTerminalTab,
  onOpenPage,
}: AgentChatTabBarProps) {
  const { t } = useTranslation()
  const [renaming, setRenaming] = useState<{ id: string; value: string } | null>(null)
  /** Tab the pointer is currently over during a drag, used to draw the insertion marker. */
  const [dropTargetId, setDropTargetId] = useState<string | null>(null)
  /** True once the strip scrolls, which is what moves the new-tab button to the bar's end. */
  const [overflowing, setOverflowing] = useState(false)
  const stripRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    const strip = stripRef.current
    if (!strip) return
    const measure = () => setOverflowing(strip.scrollWidth > strip.clientWidth + 1)
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(strip)
    return () => observer.disconnect()
  }, [tabs])

  const tabIcon = (tab: AgentChatTab) => {
    switch (tab.kind) {
      case 'agent':
        return <MessageSquareText className="size-3.5" />
      case 'terminal':
        return <TerminalSquare className="size-3.5" />
      case 'page':
        return PAGE_ICONS[tab.pageId]
      case 'review':
        return <FileDiff className="size-3.5" />
    }
  }

  const submitRename = () => {
    if (!renaming) return
    onRename(renaming.id, renaming.value)
    setRenaming(null)
  }

  const acceptsTabDrag = (event: DragEvent) => event.dataTransfer.types.includes(TAB_DRAG_MIME)

  const startDrag = (event: DragEvent<HTMLElement>, tab: AgentChatTab) => {
    event.dataTransfer.setData(TAB_DRAG_MIME, tab.id)
    event.dataTransfer.effectAllowed = 'move'
  }

  /** Drop onto a tab: the pointer's side of its midpoint decides which edge it lands on. */
  const dropOnTab = (event: DragEvent<HTMLElement>, target: AgentChatTab) => {
    if (!acceptsTabDrag(event)) return
    event.preventDefault()
    event.stopPropagation()
    setDropTargetId(null)
    const sourceId = event.dataTransfer.getData(TAB_DRAG_MIME)
    if (!sourceId) return
    const rect = event.currentTarget.getBoundingClientRect()
    const index = tabs.findIndex((tab) => tab.id === target.id)
    const after = event.clientX > rect.left + rect.width / 2
    onMoveTab(sourceId, group, after ? tabs[index + 1]?.id ?? null : target.id)
  }

  /** Drop onto the empty part of the strip: the tab joins this group at the end. */
  const dropOnStrip = (event: DragEvent<HTMLElement>) => {
    if (!acceptsTabDrag(event)) return
    event.preventDefault()
    setDropTargetId(null)
    const sourceId = event.dataTransfer.getData(TAB_DRAG_MIME)
    if (sourceId) onMoveTab(sourceId, group, null)
  }

  /**
   * The new-tab button trails the last tab so it reads as "add one more here". Once the strip
   * scrolls it would be scrolled out of reach, so it pins to the end of the bar instead.
   */
  const newTabMenu = (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button type="button" variant="ghost" size="icon-xs" className="shrink-0" aria-label={t('agentChat.tabs.new')} title={t('agentChat.tabs.new')}>
          <Plus />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-48">
          <DropdownMenuItem disabled={newChatDisabled} onSelect={() => onNewAgentTab(group)}>
            <MessageSquareText />
            {t('agentChat.tabs.newChat')}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => onNewTerminalTab(group, 'shell')}>
            <TerminalSquare />
            {t('agentChat.tabs.newTerminal')}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => onNewTerminalTab(group, 'codex')}>
            <TerminalSquare />
            {t('agentChat.terminal.profile.codex')}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => onNewTerminalTab(group, 'claude')}>
            <TerminalSquare />
            {t('agentChat.terminal.profile.claude')}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          {AGENT_CHAT_PAGE_IDS.map((pageId) => (
            <DropdownMenuItem key={pageId} onSelect={() => onOpenPage(group, pageId)}>
              {PAGE_ICONS[pageId]}
              {t(`agentChat.page.${pageId}`)}
            </DropdownMenuItem>
          ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )

  return (
    <div
      // Tabs sit flush against the pane edge; only the trailing new-tab button gets breathing room.
      className="flex h-9 shrink-0 items-center border-b border-[var(--nova-border)] bg-[var(--nova-surface)] pr-1"
      onDragOver={(event) => { if (acceptsTabDrag(event)) event.preventDefault() }}
      onDrop={dropOnStrip}
    >
      <Tabs
        value={activeTabId ?? ''}
        onValueChange={onActivate}
        className="h-full min-w-0 flex-1 gap-0"
      >
        <TabsList
          ref={stripRef}
          // The default variant is kept: its `line` sibling forces a transparent background on the
          // active trigger, which would erase the highlight below.
          className="!h-full w-full justify-start gap-0 rounded-none bg-transparent p-0 overflow-x-auto overflow-y-hidden"
        >
          {tabs.map((tab) => {
            const label = tabTitle(tab)
            return (
              <ContextMenu key={tab.id}>
                <ContextMenuTrigger asChild>
                  <TabsTrigger
                    value={tab.id}
                    draggable
                    title={label}
                    className={`${TAB_TRIGGER_CLASS} ${dropTargetId === tab.id ? 'shadow-[inset_2px_0_0_0_var(--nova-accent)]' : ''}`}
                    onDragStart={(event) => startDrag(event, tab)}
                    onDragOver={(event) => {
                      if (!acceptsTabDrag(event)) return
                      event.preventDefault()
                      event.stopPropagation()
                      setDropTargetId(tab.id)
                    }}
                    onDragLeave={() => setDropTargetId((current) => (current === tab.id ? null : current))}
                    onDragEnd={() => setDropTargetId(null)}
                    onDrop={(event) => dropOnTab(event, tab)}
                    onDoubleClick={() => setRenaming({ id: tab.id, value: label })}
                  >
                    {/*
                      The accent rule is an element rather than a border or a shadow: the base
                      trigger already styles both, from rules specific enough to win, and this
                      keeps the marker independent of them.
                    */}
                    <span
                      aria-hidden="true"
                      className="pointer-events-none absolute inset-x-0 top-0 h-0.5 bg-[var(--nova-accent)] opacity-0 group-aria-[selected=true]/tab:opacity-100"
                    />
                    <span className="shrink-0 text-[var(--nova-text-faint)]">{tabIcon(tab)}</span>
                    <span className="min-w-0 truncate">{label}</span>
                    {tab.pinned ? (
                      <Pin className="size-3 shrink-0 text-[var(--nova-text-faint)]" aria-hidden="true" />
                    ) : (
                      <span
                        role="button"
                        tabIndex={-1}
                        aria-label={t('agentChat.tabs.close', { title: label })}
                        className="grid size-4 shrink-0 place-items-center rounded-sm opacity-0 transition-opacity hover:bg-[var(--nova-hover)] group-hover/tab:opacity-100"
                        // Radix activates a trigger on pointer down, so the close hit area has to
                        // stop the event or closing a background tab would select it first.
                        onPointerDown={(event) => event.stopPropagation()}
                        onClick={(event) => { event.stopPropagation(); onClose(tab.id) }}
                      >
                        <X className="size-3" />
                      </span>
                    )}
                  </TabsTrigger>
                </ContextMenuTrigger>
                <ContextMenuContent className="min-w-44">
                  <ContextMenuItem onSelect={() => setRenaming({ id: tab.id, value: label })}>
                    <Pencil />
                    {t('agentChat.tabs.rename')}
                  </ContextMenuItem>
                  <ContextMenuItem onSelect={() => onTogglePin(tab.id)}>
                    {tab.pinned ? <PinOff /> : <Pin />}
                    {tab.pinned ? t('agentChat.tabs.unpin') : t('agentChat.tabs.pin')}
                  </ContextMenuItem>
                  <ContextMenuItem onSelect={() => onMoveTab(tab.id, oppositeGroup(group), null)}>
                    <SplitSquareHorizontal />
                    {group === 'primary' ? t('agentChat.tabs.moveRight') : t('agentChat.tabs.moveLeft')}
                  </ContextMenuItem>
                  <ContextMenuSeparator />
                  <ContextMenuItem onSelect={() => onClose(tab.id)}>
                    {t('agentChat.tabs.closeTab')}
                  </ContextMenuItem>
                  <ContextMenuItem onSelect={() => onCloseOthers(tab.id)}>
                    {t('agentChat.tabs.closeOthers')}
                  </ContextMenuItem>
                  <ContextMenuItem onSelect={() => onCloseToRight(tab.id)}>
                    {t('agentChat.tabs.closeToRight')}
                  </ContextMenuItem>
                </ContextMenuContent>
              </ContextMenu>
            )
          })}
          {!overflowing && newTabMenu}
        </TabsList>
      </Tabs>

      {overflowing && newTabMenu}

      <Dialog open={renaming !== null} onOpenChange={(open) => { if (!open) setRenaming(null) }}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t('agentChat.tabs.renameTitle')}</DialogTitle>
            <DialogDescription>{t('agentChat.tabs.renameDescription')}</DialogDescription>
          </DialogHeader>
          <Input
            autoFocus
            value={renaming?.value ?? ''}
            onChange={(event) => setRenaming((current) => (current ? { ...current, value: event.target.value } : current))}
            onKeyDown={(event) => { if (event.key === 'Enter') submitRename() }}
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
    </div>
  )
}
