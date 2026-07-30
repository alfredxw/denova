import { useCallback, useEffect, useRef, type Dispatch, type SetStateAction } from 'react'
import type { AgentChatTab } from '../types'
import { readStoredWorkbenchState, type AgentChatWorkbenchState } from '../tab-state'
import { closeTerminalSession, getTerminalRuntimeStatus, type TerminalSessionInfo } from './api'

/** Map every persisted terminal owner to the backend session it has already established. */
function terminalTabSessions(workbench: AgentChatWorkbenchState): Map<string, string | undefined> {
  const sessions = new Map<string, string | undefined>()
  for (const state of Object.values(workbench.projects)) {
    for (const tab of state.tabs) {
      if (tab.kind === 'terminal') sessions.set(tab.id, tab.terminalSessionId)
    }
  }
  return sessions
}

/**
 * Keeps backend pty processes aligned with persisted workbench tabs.
 *
 * WebSocket detach is deliberately not ownership loss: page reloads and mode switches must be
 * able to re-attach. A session is released only after its stable tab owner disappears.
 */
export function useTerminalSessionLifecycle(
  workbench: AgentChatWorkbenchState,
  projectsLoading: boolean,
  setWorkbench: Dispatch<SetStateAction<AgentChatWorkbenchState>>,
) {
  const workbenchRef = useRef(workbench)
  const viewMountedRef = useRef(false)
  const closedTabIdsRef = useRef(new Set<string>())
  const previousSessionsRef = useRef(new Map<string, string | undefined>())
  const releasingSessionIdsRef = useRef(new Set<string>())
  const reconciledRef = useRef(false)
  workbenchRef.current = workbench

  const releaseSession = useCallback((sessionId: string, context: Record<string, unknown>) => {
    if (releasingSessionIdsRef.current.has(sessionId)) return
    releasingSessionIdsRef.current.add(sessionId)
    void closeTerminalSession(sessionId)
      .catch((error) => {
        console.warn('[features/agent-chat/terminal/use-terminal-session-lifecycle.ts] releasing session failed', {
          ...context,
          sessionId,
          error,
        })
      })
      .finally(() => {
        releasingSessionIdsRef.current.delete(sessionId)
      })
  }, [])

  useEffect(() => {
    viewMountedRef.current = true
    return () => {
      viewMountedRef.current = false
    }
  }, [])

  // Also catches implicit removals such as the open-tab cap and projects disappearing from the
  // live project list; those paths do not pass through the tab bar's explicit close callback.
  useEffect(() => {
    const current = terminalTabSessions(workbench)
    for (const [tabId, sessionId] of previousSessionsRef.current) {
      if (current.has(tabId)) continue
      closedTabIdsRef.current.add(tabId)
      if (sessionId) releaseSession(sessionId, { tabId, reason: 'owner_removed' })
    }
    previousSessionsRef.current = current
  }, [releaseSession, workbench])

  // Reconcile once after persisted tabs have been filtered against the live project list. This
  // closes sessions left behind by old storage versions, crashes or requests that lost their tab.
  useEffect(() => {
    if (projectsLoading || reconciledRef.current) return
    reconciledRef.current = true
    void getTerminalRuntimeStatus()
      .then((runtime) => {
        // Read storage at response time so another same-origin window's newer tab state is respected.
        const tabs = terminalTabSessions(readStoredWorkbenchState())
        const ownedSessionIds = new Set([...tabs.values()].filter((id): id is string => Boolean(id)))
        const orphaned = runtime.sessions.filter(
          (session) => session.attached === 0 && !ownedSessionIds.has(session.id) && (!session.owner_tab_id || !tabs.has(session.owner_tab_id)),
        )
        if (orphaned.length > 0) {
          console.info('[features/agent-chat/terminal/use-terminal-session-lifecycle.ts] releasing orphaned sessions', {
            count: orphaned.length,
            sessionIds: orphaned.map((session) => session.id),
          })
        }
        for (const session of orphaned) {
          releaseSession(session.id, {
            ownerTabId: session.owner_tab_id,
            reason: 'startup_reconciliation',
          })
        }
      })
      .catch((error) => {
        console.warn('[features/agent-chat/terminal/use-terminal-session-lifecycle.ts] reconciliation failed', { error })
      })
  }, [projectsLoading, releaseSession])

  const markTerminalTabsClosing = useCallback(
    (tabs: readonly AgentChatTab[]) => {
      for (const tab of tabs) {
        if (tab.kind !== 'terminal') continue
        closedTabIdsRef.current.add(tab.id)
        if (tab.terminalSessionId) {
          releaseSession(tab.terminalSessionId, {
            tabId: tab.id,
            reason: 'tab_closed',
          })
        }
      }
    },
    [releaseSession],
  )

  const bindTerminalSession = useCallback(
    (projectID: string, tabId: string, session: TerminalSessionInfo): boolean => {
      const state = workbenchRef.current.projects[projectID]
      const owned = !closedTabIdsRef.current.has(tabId) && state?.tabs.some((tab) => tab.kind === 'terminal' && tab.id === tabId)
      if (!owned) return false
      // Mode switches may unmount AgentChat while a creation request is resolving. The persisted
      // tab still owns the pty, but there is no mounted component whose state needs updating.
      if (!viewMountedRef.current) return true
      setWorkbench((current) => {
        const currentState = current.projects[projectID]
        if (!currentState) return current
        return {
          ...current,
          projects: {
            ...current.projects,
            [projectID]: {
              ...currentState,
              tabs: currentState.tabs.map((tab) =>
                tab.id === tabId && tab.kind === 'terminal'
                  ? {
                      ...tab,
                      terminalSessionId: session.id,
                      title: tab.title || session.title,
                    }
                  : tab,
              ),
            },
          },
        }
      })
      return true
    },
    [setWorkbench],
  )

  return { bindTerminalSession, markTerminalTabsClosing }
}
