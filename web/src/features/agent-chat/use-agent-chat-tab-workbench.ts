import { useCallback, type Dispatch, type SetStateAction } from 'react'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import {
  appendTab,
  emptyProjectTabState,
  moveTab,
  nextActiveTabId,
  setTabPinned,
  setTabTitle,
  tabGroup,
  tabsInGroup,
  type AgentChatWorkbenchState,
} from './tab-state'
import { AGENT_CHAT_GROUP_IDS, type AgentChatGroupId, type AgentChatTab } from './types'

interface AgentChatTabWorkbenchOptions {
  workbenchRef: { current: AgentChatWorkbenchState }
  setWorkbench: Dispatch<SetStateAction<AgentChatWorkbenchState>>
  setMountedTabKeys: Dispatch<SetStateAction<ReadonlySet<string>>>
  flushTabDrafts: (keys?: ReadonlySet<string>) => Promise<boolean>
  tabFlushHandlersRef: { current: Map<string, EditorFlushHandler> }
  markTerminalTabsClosing: (tabs: AgentChatTab[]) => void
}

export function mountedAgentChatTabKey(projectID: string, tabID: string) {
  return `${projectID}\u0000${tabID}`
}

/** Owns generic tab mutations independently from the project/content rendered inside a tab. */
export function useAgentChatTabWorkbench({
  workbenchRef,
  setWorkbench,
  setMountedTabKeys,
  flushTabDrafts,
  tabFlushHandlersRef,
  markTerminalTabsClosing,
}: AgentChatTabWorkbenchOptions) {
  const openTab = useCallback((tab: AgentChatTab) => {
    const state = workbenchRef.current.projects[tab.projectId] ?? emptyProjectTabState()
    const mountedID = appendTab(state.tabs, tab).activeId
    setWorkbench((current) => {
      const currentState = current.projects[tab.projectId] ?? emptyProjectTabState()
      const appended = appendTab(currentState.tabs, tab)
      const opened = appended.tabs.find((item) => item.id === appended.activeId)
      const group = opened ? tabGroup(opened) : tabGroup(tab)
      return {
        activeProjectId: tab.projectId,
        projects: {
          ...current.projects,
          [tab.projectId]: {
            ...currentState,
            tabs: appended.tabs,
            activeTabIds: { ...currentState.activeTabIds, [group]: appended.activeId },
            focusedGroup: group,
            secondaryVisible: group === 'secondary' || currentState.secondaryVisible,
          },
        },
      }
    })
    setMountedTabKeys((current) => new Set(current).add(mountedAgentChatTabKey(tab.projectId, mountedID)))
  }, [setMountedTabKeys, setWorkbench, workbenchRef])

  const closeTabs = useCallback(async (projectID: string, tabIDs: string[]): Promise<boolean> => {
    if (tabIDs.length === 0) return true
    const closingKeys = new Set(tabIDs.map((id) => mountedAgentChatTabKey(projectID, id)))
    if (!(await flushTabDrafts(closingKeys))) return false
    const closing = new Set(tabIDs)
    const doomed = workbenchRef.current.projects[projectID]?.tabs.filter((tab) => closing.has(tab.id)) ?? []
    if (doomed.length === 0) return true
    markTerminalTabsClosing(doomed)

    setWorkbench((current) => {
      const currentState = current.projects[projectID]
      if (!currentState) return current
      const activeTabIds = { ...currentState.activeTabIds }
      for (const group of AGENT_CHAT_GROUP_IDS) {
        const activeID = activeTabIds[group]
        if (!activeID || !closing.has(activeID)) continue
        const candidates = tabsInGroup(currentState.tabs, group).filter((tab) => tab.id === activeID || !closing.has(tab.id))
        activeTabIds[group] = nextActiveTabId(candidates, activeID, activeID)
      }
      const tabs = currentState.tabs.filter((tab) => !closing.has(tab.id))
      return {
        ...current,
        projects: {
          ...current.projects,
          [projectID]: {
            ...currentState,
            tabs,
            activeTabIds,
            secondaryVisible: currentState.secondaryVisible && tabsInGroup(tabs, 'secondary').length > 0,
          },
        },
      }
    })
    closingKeys.forEach((key) => tabFlushHandlersRef.current.delete(key))
    setMountedTabKeys((current) => new Set([...current].filter((key) => !closingKeys.has(key))))
    return true
  }, [flushTabDrafts, markTerminalTabsClosing, setMountedTabKeys, setWorkbench, tabFlushHandlersRef, workbenchRef])

  const activateTab = useCallback((projectID: string, group: AgentChatGroupId, tabID: string) => {
    setWorkbench((current) => {
      const state = current.projects[projectID] ?? emptyProjectTabState()
      return {
        activeProjectId: projectID,
        projects: {
          ...current.projects,
          [projectID]: {
            ...state,
            activeTabIds: { ...state.activeTabIds, [group]: tabID },
            focusedGroup: group,
            secondaryVisible: group === 'secondary' || state.secondaryVisible,
          },
        },
      }
    })
    setMountedTabKeys((current) => new Set(current).add(mountedAgentChatTabKey(projectID, tabID)))
  }, [setMountedTabKeys, setWorkbench])

  const focusGroup = useCallback((projectID: string, group: AgentChatGroupId) => {
    setWorkbench((current) => {
      const state = current.projects[projectID] ?? emptyProjectTabState()
      if (current.activeProjectId === projectID && state.focusedGroup === group) return current
      return {
        activeProjectId: projectID,
        projects: { ...current.projects, [projectID]: { ...state, focusedGroup: group } },
      }
    })
  }, [setWorkbench])

  const showSecondaryPane = useCallback((projectID: string) => {
    setWorkbench((current) => {
      const state = current.projects[projectID]
      if (!state || state.secondaryVisible || tabsInGroup(state.tabs, 'secondary').length === 0) return current
      return { ...current, projects: { ...current.projects, [projectID]: { ...state, secondaryVisible: true } } }
    })
  }, [setWorkbench])

  const hideSecondaryPane = useCallback((projectID: string) => {
    setWorkbench((current) => {
      const state = current.projects[projectID]
      if (!state || !state.secondaryVisible) return current
      return {
        ...current,
        projects: { ...current.projects, [projectID]: { ...state, secondaryVisible: false, focusedGroup: 'primary' } },
      }
    })
  }, [setWorkbench])

  const renameTab = useCallback((projectID: string, tabID: string, title: string) => {
    setWorkbench((current) => {
      const state = current.projects[projectID]
      if (!state) return current
      return {
        ...current,
        projects: { ...current.projects, [projectID]: { ...state, tabs: setTabTitle(state.tabs, tabID, title) } },
      }
    })
  }, [setWorkbench])

  const togglePinTab = useCallback((projectID: string, tabID: string) => {
    setWorkbench((current) => {
      const state = current.projects[projectID]
      if (!state) return current
      const pinned = !state.tabs.find((tab) => tab.id === tabID)?.pinned
      return {
        ...current,
        projects: { ...current.projects, [projectID]: { ...state, tabs: setTabPinned(state.tabs, tabID, pinned) } },
      }
    })
  }, [setWorkbench])

  const relocateTab = useCallback((projectID: string, sourceID: string, group: AgentChatGroupId, beforeID: string | null) => {
    setWorkbench((current) => {
      const state = current.projects[projectID]
      const source = state?.tabs.find((tab) => tab.id === sourceID)
      if (!state || !source) return current
      const from = tabGroup(source)
      const tabs = moveTab(state.tabs, sourceID, group, beforeID)
      return {
        ...current,
        projects: {
          ...current.projects,
          [projectID]: {
            ...state,
            tabs,
            focusedGroup: group,
            secondaryVisible: group === 'secondary' || (state.secondaryVisible && tabsInGroup(tabs, 'secondary').length > 0),
            activeTabIds: from === group
              ? state.activeTabIds
              : {
                  ...state.activeTabIds,
                  [from]: state.activeTabIds[from] === sourceID
                    ? nextActiveTabId(tabsInGroup(state.tabs, from), sourceID, sourceID)
                    : state.activeTabIds[from],
                  [group]: sourceID,
                },
          },
        },
      }
    })
  }, [setWorkbench])

  return {
    activateTab,
    closeTabs,
    focusGroup,
    hideSecondaryPane,
    openTab,
    relocateTab,
    renameTab,
    showSecondaryPane,
    togglePinTab,
  }
}
