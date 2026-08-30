const ACTIVE_SESSION_STORAGE_PREFIX = 'nova.writingAgent.activeSession.v1:'

function activeSessionStorageKey(projectId: string, channel: 'agent' | 'configuration') {
  return channel === 'agent'
    ? `${ACTIVE_SESSION_STORAGE_PREFIX}${projectId}`
    : `${ACTIVE_SESSION_STORAGE_PREFIX}${channel}:${projectId}`
}

export function readAgentChatActiveSession(projectId: string, channel: 'agent' | 'configuration' = 'agent'): string {
  if (typeof window === 'undefined') return ''
  return window.localStorage.getItem(activeSessionStorageKey(projectId, channel)) || ''
}

export function writeAgentChatActiveSession(
  projectId: string,
  sessionId: string,
  channel: 'agent' | 'configuration' = 'agent',
): void {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(activeSessionStorageKey(projectId, channel), sessionId)
}
