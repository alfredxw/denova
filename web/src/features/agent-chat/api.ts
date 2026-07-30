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

export type AgentChatProjectType = 'book' | 'general'
export type AgentChatProjectStatus = 'available' | 'missing' | 'archived'

/** One user-managed Project with its conversations. */
export interface AgentChatProject {
  id: string
  type: AgentChatProjectType
  path: string
  name: string
  status: AgentChatProjectStatus
  /** Marks the workspace the backend currently has open. */
  current: boolean
  /** Conversation count before the backend truncated the list. */
  total: number
  sessions: AgentChatSession[]
  /** Per-project read failure; one broken book must not blank the tree. */
  error?: string
}

export interface AgentChatHistoryItem {
  project_id: string
  project_name: string
  session: AgentChatSession
}

export interface AgentChatHistoryPage {
  items: AgentChatHistoryItem[]
  total: number
  offset: number
  has_more: boolean
}

export interface HostDirectorySelection {
  path: string
  canceled: boolean
}

let projectsReadInFlight: Promise<AgentChatProject[]> | null = null

/** Read every project with its conversations. This never switches the open workspace. */
export function getAgentChatProjects(): Promise<AgentChatProject[]> {
  if (projectsReadInFlight) return projectsReadInFlight
  projectsReadInFlight = requestJSON<{ projects?: AgentChatProject[] }>('/api/agent-chat/projects')
    .then((data) =>
      (data.projects ?? []).map((project) => ({
        ...project,
        sessions: project.sessions ?? [],
      })),
    )
    .finally(() => {
      projectsReadInFlight = null
    })
  return projectsReadInFlight
}

/** Search complete durable conversation metadata without switching the foreground workspace. */
export function getAgentChatHistory(
  options: {
    query?: string
    projectId?: string
    offset?: number
    limit?: number
    signal?: AbortSignal
  } = {},
): Promise<AgentChatHistoryPage> {
  const params = new URLSearchParams()
  const query = options.query?.trim()
  const projectId = options.projectId?.trim()
  if (query) params.set('query', query)
  if (projectId) params.set('project_id', projectId)
  if (options.offset) params.set('offset', String(options.offset))
  if (options.limit) params.set('limit', String(options.limit))
  const suffix = params.size > 0 ? `?${params.toString()}` : ''
  return requestJSON<AgentChatHistoryPage>(`/api/agent-chat/history${suffix}`, {
    signal: options.signal,
  }).then((page) => ({ ...page, items: page.items ?? [] }))
}

/** Create a conversation inside any project, open or not. */
export async function createAgentChatSession(projectId: string, title = ''): Promise<AgentChatSession> {
  return requestJSON<AgentChatSession>('/api/agent-chat/sessions', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ project_id: projectId, title }),
  })
}

export async function renameAgentChatSession(projectId: string, sessionId: string, title: string): Promise<void> {
  await requestJSON('/api/agent-chat/sessions/rename', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({
      project_id: projectId,
      session_id: sessionId,
      title,
    }),
  })
}

export async function deleteAgentChatSession(projectId: string, sessionId: string): Promise<void> {
  await requestJSON('/api/agent-chat/sessions/delete', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ project_id: projectId, session_id: sessionId }),
  })
}

/** Open the folder chooser on the machine running Denova. */
export async function selectAgentChatProjectDirectory(initialPath = ''): Promise<HostDirectorySelection> {
  return requestJSON<HostDirectorySelection>('/api/host/dialogs/directory', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ initial_path: initialPath }),
  })
}

/** Register a folder; the backend derives its name and Book/General behavior. */
export async function addAgentChatProject(path: string): Promise<AgentChatProject> {
  return requestJSON<AgentChatProject>('/api/agent-chat/projects', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ path }),
  })
}

export async function renameAgentChatProject(projectId: string, name: string): Promise<AgentChatProject> {
  return requestJSON<AgentChatProject>(`/api/agent-chat/projects/${encodeURIComponent(projectId)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify({ name }),
  })
}

export async function relinkAgentChatProject(projectId: string, path: string): Promise<AgentChatProject> {
  return requestJSON<AgentChatProject>(`/api/agent-chat/projects/${encodeURIComponent(projectId)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify({ path }),
  })
}

export async function archiveAgentChatProject(projectId: string): Promise<void> {
  await requestJSON(`/api/agent-chat/projects/${encodeURIComponent(projectId)}`, { method: 'DELETE' })
}

export async function reorderAgentChatProjects(projectIds: string[]): Promise<void> {
  await requestJSON('/api/agent-chat/projects/reorder', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ project_ids: projectIds }),
  })
}
