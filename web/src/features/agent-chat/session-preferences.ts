const ACTIVE_SESSION_STORAGE_PREFIX = 'nova.writingAgent.activeSession.v1:'

export function readAgentChatActiveSession(projectId: string): string {
  if (typeof window === 'undefined') return ''
  return window.localStorage.getItem(`${ACTIVE_SESSION_STORAGE_PREFIX}${projectId}`) || ''
}

export function writeAgentChatActiveSession(projectId: string, sessionId: string): void {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(`${ACTIVE_SESSION_STORAGE_PREFIX}${projectId}`, sessionId)
}
