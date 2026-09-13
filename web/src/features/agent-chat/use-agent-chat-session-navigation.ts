import { useCallback, useEffect, useRef, useState } from 'react'
import type { AgentChatProject, AgentChatSession } from './api'
import {
  AGENT_CHAT_SESSION_NAVIGATION_EVENT,
  pendingAgentChatSessionNavigation,
  completeAgentChatSessionNavigation,
  type AgentChatSessionNavigationTarget,
} from './session-navigation'
import { agentChatSessionBindingKey } from './sidebar-activity'
import type { AgentChatPendingAction } from './AgentChatConversationTab'

interface AgentChatSessionNavigationOptions {
  refreshProjects: () => Promise<AgentChatProject[] | null>
  openOrActivateSession: (project: AgentChatProject, session: AgentChatSession) => void
  onOpenFile?: (project: AgentChatProject, path: string) => void
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
  onOpenFile,
}: AgentChatSessionNavigationOptions) {
  const [syncSignals, setSyncSignals] = useState<ReadonlyMap<string, number>>(() => new Map())
  const [pendingActions, setPendingActions] = useState<ReadonlyMap<string, AgentChatPendingAction>>(() => new Map())
  const consumePendingAction = useCallback((key: string) => {
    setPendingActions(current => {
      const next = new Map(current)
      next.delete(key)
      return next
    })
  }, [])
  const openOrActivateSessionRef = useRef(openOrActivateSession)
  openOrActivateSessionRef.current = openOrActivateSession
  const openFileRef = useRef(onOpenFile)
  openFileRef.current = onOpenFile

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
      if (cancelled || !snapshot || pendingAgentChatSessionNavigation() !== target) return
      const project = snapshot.find((candidate) => candidate.id === target.projectId)
      const session = project?.sessions.find((candidate) => candidate.id === target.sessionId)
      if (!project || !session) {
        console.warn('[features/agent-chat/use-agent-chat-session-navigation.ts] requested conversation is missing from Project snapshot', target)
        return
      }
      // Refreshing Projects also reconciles the workbench. The ref prevents that state change from
      // cancelling navigation through a freshly-created navigator callback.
      openOrActivateSessionRef.current(project, session)
      const message = target.initialInstruction?.trim()
      if (message) {
        const key = agentChatSessionBindingKey(project.id, session.id)
        setPendingActions(current => new Map(current).set(key, { id: session.id, message, displayMessage: message }))
      }
      if (target.sourcePath) openFileRef.current?.(project, target.sourcePath)
      requestConversationSync(project.id, session.id)
      completeAgentChatSessionNavigation(target)
    }
    const receiveNavigation = () => { void openTarget(pendingAgentChatSessionNavigation()) }
    window.addEventListener(AGENT_CHAT_SESSION_NAVIGATION_EVENT, receiveNavigation)
    receiveNavigation()
    return () => {
      cancelled = true
      window.removeEventListener(AGENT_CHAT_SESSION_NAVIGATION_EVENT, receiveNavigation)
    }
  }, [refreshProjects, requestConversationSync])

  return { syncSignals, pendingActions, consumePendingAction }
}
