import { useCallback, useEffect, useState, type Dispatch, type SetStateAction } from 'react'
import type { AgentChatProject } from './api'
import { createTabId, setTerminalTabTitle, type AgentChatWorkbenchState } from './tab-state'
import { getTerminalRuntimeStatus } from './terminal/api'
import type { AgentChatTerminalStatus } from './terminal/TerminalTabView'
import type { AgentChatGroupId, AgentChatTab, TerminalCommandProfile, TerminalProfileId } from './types'

interface AgentChatTerminalTabsOptions {
  setWorkbench: Dispatch<SetStateAction<AgentChatWorkbenchState>>
  openTab: (tab: AgentChatTab) => void
}

/** Owns terminal-tab presentation state and the settings-backed command catalog. */
export function useAgentChatTerminalTabs({ setWorkbench, openTab }: AgentChatTerminalTabsOptions) {
  const [statuses, setStatuses] = useState<ReadonlyMap<string, AgentChatTerminalStatus>>(() => new Map())
  const [commands, setCommands] = useState<TerminalCommandProfile[]>([])

  const refreshCommands = useCallback(async () => {
    try {
      const runtime = await getTerminalRuntimeStatus()
      setCommands(runtime.commands ?? [])
    } catch (error) {
      console.warn('[features/agent-chat/use-agent-chat-terminal-tabs.ts] loading terminal commands failed', { error })
    }
  }, [])

  useEffect(() => {
    void refreshCommands()
    const onSettingsUpdated = () => void refreshCommands()
    window.addEventListener('nova:settings-updated', onSettingsUpdated)
    return () => window.removeEventListener('nova:settings-updated', onSettingsUpdated)
  }, [refreshCommands])

  const openTerminal = useCallback((
    project: AgentChatProject,
    group: AgentChatGroupId,
    profileID: TerminalProfileId,
    profileName?: string,
  ) => {
    openTab({
      kind: 'terminal',
      id: createTabId('terminal'),
      projectId: project.id,
      workspace: project.path,
      group,
      profileId: profileID,
      profileName,
      title: profileID === 'shell' ? '' : profileName || profileID,
    })
  }, [openTab])

  const updateTitle = useCallback((projectID: string, tabID: string, title: string) => {
    setWorkbench((current) => {
      const state = current.projects[projectID]
      if (!state) return current
      const tabs = setTerminalTabTitle(state.tabs, tabID, title)
      if (tabs.every((tab, index) => tab === state.tabs[index])) return current
      return {
        ...current,
        projects: { ...current.projects, [projectID]: { ...state, tabs } },
      }
    })
  }, [setWorkbench])

  const handleStatusChange = useCallback((tabID: string, status: AgentChatTerminalStatus | null) => {
    setStatuses((current) => {
      if (status === null && !current.has(tabID)) return current
      if (status !== null && current.get(tabID) === status) return current
      const next = new Map(current)
      if (status === null) next.delete(tabID)
      else next.set(tabID, status)
      return next
    })
  }, [])

  return { commands, statuses, openTerminal, updateTitle, handleStatusChange }
}
