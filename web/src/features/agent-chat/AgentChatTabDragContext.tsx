import { useCallback, type ReactNode } from 'react'
import type { ClientRect, Data, DragEndEvent } from '@dnd-kit/core'
import {
  resolveWorkbenchTabDropEdge,
  WorkbenchTabDragContext,
  type WorkbenchTabDropTargetData,
} from '@/components/workbench/WorkbenchTabDrag'
import { tabGroup, tabsInGroup, type AgentChatWorkbenchState } from './tab-state'
import type { AgentChatGroupId, AgentChatTab } from './types'

export interface AgentChatTabDragData extends Data {
  kind: 'agent-chat-tab'
  projectId: string
  group: AgentChatGroupId
  tabId: string
  label: string
}

export interface AgentChatTabStripDropData extends Data, WorkbenchTabDropTargetData {
  kind: 'agent-chat-tab-strip'
  projectId: string
  group: AgentChatGroupId
  label: string
}

type AgentChatTabDropData = AgentChatTabDragData | AgentChatTabStripDropData

interface AgentChatTabDragContextProps {
  workbench: AgentChatWorkbenchState
  children: ReactNode
  onMoveTab: (
    projectId: string,
    sourceId: string,
    group: AgentChatGroupId,
    beforeId: string | null,
  ) => void
}

/** Namespace persisted tab ids before exposing them to the user-level drag context. */
export function agentChatTabSortableId(projectId: string, tabId: string) {
  return `agent-chat-tab\u0000${projectId}\u0000${tabId}`
}

/** Each pane is also a drop target so tabs can move into an empty strip or past its last tab. */
export function agentChatTabStripDropId(projectId: string, group: AgentChatGroupId) {
  return `agent-chat-tab-strip\u0000${projectId}\u0000${group}`
}

function isAgentChatTabDropData(value: Data | undefined): value is AgentChatTabDropData {
  if (!value || (value.kind !== 'agent-chat-tab' && value.kind !== 'agent-chat-tab-strip')) return false
  return typeof value.projectId === 'string'
    && (value.group === 'primary' || value.group === 'secondary')
    && (value.kind !== 'agent-chat-tab' || typeof value.tabId === 'string')
}

/**
 * Translate dnd-kit's "move onto this item" result into the workbench model's "insert before"
 * contract. Reordering within one strip follows arrayMove semantics; entering another strip uses
 * the drag preview's center to choose the same leading or trailing edge shown to the user.
 */
export function agentChatDropBeforeId(
  tabs: AgentChatTab[],
  sourceId: string,
  group: AgentChatGroupId,
  overId: string | null,
  placeAfter = false,
): string | null {
  const source = tabs.find((tab) => tab.id === sourceId)
  if (!source) return sourceId
  if (!overId) return null

  const targetTabs = tabsInGroup(tabs, group)
  const targetIndex = targetTabs.findIndex((tab) => tab.id === overId)
  if (targetIndex < 0) return sourceId

  if (tabGroup(source) === group) {
    const sourceIndex = targetTabs.findIndex((tab) => tab.id === sourceId)
    if (sourceIndex < 0 || sourceIndex === targetIndex) return sourceId
    const ids = targetTabs.map((tab) => tab.id)
    const [movedId] = ids.splice(sourceIndex, 1)
    ids.splice(targetIndex, 0, movedId)
    return ids[ids.indexOf(sourceId) + 1] ?? null
  }

  const ids = targetTabs.map((tab) => tab.id)
  ids.splice(targetIndex + (placeAfter ? 1 : 0), 0, sourceId)
  return ids[ids.indexOf(sourceId) + 1] ?? null
}

/** One drag context spans both Agent Chat panes so a single gesture can reorder or relocate a tab. */
export function AgentChatTabDragContext({
  workbench,
  children,
  onMoveTab,
}: AgentChatTabDragContextProps) {
  const handleDragEnd = useCallback((event: DragEndEvent, dragRect: ClientRect | null) => {
    if (!event.over || event.active.id === event.over.id) return
    const source = event.active.data.current
    const target = event.over.data.current
    if (!isAgentChatTabDropData(source) || source.kind !== 'agent-chat-tab') return
    if (!isAgentChatTabDropData(target) || source.projectId !== target.projectId) return

    const state = workbench.projects[source.projectId]
    const sourceTab = state?.tabs.find((tab) => tab.id === source.tabId)
    if (!state || !sourceTab) return
    const targetTabId = target.kind === 'agent-chat-tab' ? target.tabId : null
    if (targetTabId && !state.tabs.some((tab) => tab.id === targetTabId && tabGroup(tab) === target.group)) return

    const placeAfter = target.kind === 'agent-chat-tab'
      && tabGroup(sourceTab) !== target.group
      && resolveWorkbenchTabDropEdge(dragRect, event.over.rect) === 'end'
    const beforeId = agentChatDropBeforeId(state.tabs, source.tabId, target.group, targetTabId, placeAfter)
    if (beforeId === source.tabId) return
    onMoveTab(source.projectId, source.tabId, target.group, beforeId)
  }, [onMoveTab, workbench.projects])

  return <WorkbenchTabDragContext onDragEnd={handleDragEnd}>{children}</WorkbenchTabDragContext>
}
