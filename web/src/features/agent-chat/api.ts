import { jsonHeaders, requestJSON } from '@/lib/api-client/client'

/** One conversation in the AgentChat project tree. */
export interface AgentChatSession {
  id: string
  title: string
  created_at: string
  updated_at: string
  message_count: number
  /** This exact conversation has a task running; other conversations remain available. */
  running: boolean
  active: boolean
}

/** One book with its conversations. */
export interface AgentChatProject {
  path: string
  name: string
  /** Marks the workspace the backend currently has open. */
  current: boolean
  /** Conversation count before the backend truncated the list. */
  total: number
  sessions: AgentChatSession[]
  /** Per-project read failure; one broken book must not blank the tree. */
  error?: string
}

let projectsReadInFlight: Promise<AgentChatProject[]> | null = null

/** Read every project with its conversations. This never switches the open workspace. */
export function getAgentChatProjects(): Promise<AgentChatProject[]> {
  if (projectsReadInFlight) return projectsReadInFlight
  projectsReadInFlight = requestJSON<{ projects?: AgentChatProject[] }>('/api/agent-chat/projects')
    .then((data) => (data.projects ?? []).map((project) => ({ ...project, sessions: project.sessions ?? [] })))
    .finally(() => { projectsReadInFlight = null })
  return projectsReadInFlight
}

/** Create a conversation inside any project, open or not. */
export async function createAgentChatSession(workspace: string, title = ''): Promise<AgentChatSession> {
  return requestJSON<AgentChatSession>('/api/agent-chat/sessions', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ workspace, title }),
  })
}

export async function renameAgentChatSession(workspace: string, sessionId: string, title: string): Promise<void> {
  await requestJSON('/api/agent-chat/sessions/rename', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ workspace, session_id: sessionId, title }),
  })
}

export async function deleteAgentChatSession(workspace: string, sessionId: string): Promise<void> {
  await requestJSON('/api/agent-chat/sessions/delete', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ workspace, session_id: sessionId }),
  })
}
