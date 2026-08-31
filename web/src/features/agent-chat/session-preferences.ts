const ACTIVE_SESSION_STORAGE_PREFIX = 'nova.writingAgent.activeSession.v1:'

function activeSessionStorageKey(projectId: string, preferenceScope: string): string {
  const normalizedScope = preferenceScope.trim()
  return normalizedScope
    ? `${ACTIVE_SESSION_STORAGE_PREFIX}${encodeURIComponent(normalizedScope)}:${projectId}`
    : `${ACTIVE_SESSION_STORAGE_PREFIX}${projectId}`
}

export function readAgentChatActiveSession(projectId: string, preferenceScope = ''): string {
  if (typeof window === 'undefined') return ''
  return window.localStorage.getItem(activeSessionStorageKey(projectId, preferenceScope)) || ''
}

export function writeAgentChatActiveSession(
  projectId: string,
  sessionId: string,
  preferenceScope = '',
): void {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(activeSessionStorageKey(projectId, preferenceScope), sessionId)
}
