import { useCallback, useEffect, useRef, useState } from 'react'
import type { AgentChatProject, AgentChatSession } from './api'
import {
  AGENT_CHAT_SESSION_NAVIGATION_EVENT,
  consumeAgentChatSessionNavigation,
  type AgentChatSessionNavigationTarget,
} from './session-navigation'
import { agentChatSessionBindingKey } from './sidebar-activity'

interface AgentChatSessionNavigationOptions {
  refreshProjects: () => Promise<AgentChatProject[] | null>
  openOrActivateSession: (project: AgentChatProject, session: AgentChatSession) => void
}

/**
 * Consumes cross-page conversation navigation and exposes a per-session sync signal.
 *
 * The signal lets an already-mounted conversation reload history or reconnect when Automation
 * admits a turn outside that tab's local chat hook.
 */
export function useAgentChatSessionNavigation({
  refreshProjects,
  openOrActivateSession,
}: AgentChatSessionNavigationOptions): ReadonlyMap<string, number> {
  const [syncSignals, setSyncSignals] = useState<ReadonlyMap<string, number>>(() => new Map())
  const openOrActivateSessionRef = useRef(openOrActivateSession)
  openOrActivateSessionRef.current = openOrActivateSession

  const requestConversationSync = useCallback((projectID: string, sessionID: string) => {
    const key = agentChatSessionBindingKey(projectID, sessionID)
    setSyncSignals((current) => {
      const next = new Map(current)
      next.set(key, (next.get(key) ?? 0) + 1)
      return next
    })
  }, [])

  useEffect(() => {
    let cancelled = false
    const openTarget = async (target: AgentChatSessionNavigationTarget | null) => {
      if (!target?.projectId || !target.sessionId) return
      const snapshot = await refreshProjects()
      if (cancelled || !snapshot) return
      const project = snapshot.find((candidate) => candidate.id === target.projectId)
      const session = project?.sessions.find((candidate) => candidate.id === target.sessionId)
      if (!project || !session) {
        console.warn('[features/agent-chat/use-agent-chat-session-navigation.ts] requested conversation is missing from Project snapshot', target)
        return
      }
      // Refreshing Projects also reconciles the workbench. The ref prevents that state change from
      // cancelling navigation through a freshly-created navigator callback.
      openOrActivateSessionRef.current(project, session)
      requestConversationSync(project.id, session.id)
    }
    const receiveNavigation = (event: Event) => {
      const queued = consumeAgentChatSessionNavigation()
      const detail = (event as CustomEvent<AgentChatSessionNavigationTarget>).detail
      void openTarget(queued || detail)
    }
    window.addEventListener(AGENT_CHAT_SESSION_NAVIGATION_EVENT, receiveNavigation)
    void openTarget(consumeAgentChatSessionNavigation())
    return () => {
      cancelled = true
      window.removeEventListener(AGENT_CHAT_SESSION_NAVIGATION_EVENT, receiveNavigation)
    }
  }, [refreshProjects, requestConversationSync])

  return syncSignals
}
