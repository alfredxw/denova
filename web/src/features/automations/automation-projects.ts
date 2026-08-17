import type { AgentChatProject, AgentChatProjectStatus, AgentChatProjectType } from '@/features/agent-chat/api'
import type { AutomationExecutionTarget, AutomationTask } from '@/lib/api'

/** Minimal Project identity needed by Automation configuration and catalogs. */
export interface AutomationProjectOption {
  id: string
  name: string
  path: string
  type: AgentChatProjectType
  status: AgentChatProjectStatus
  current: boolean
}

export function automationProjectOptions(projects: AgentChatProject[]): AutomationProjectOption[] {
  return projects
    .filter((project) => project.status !== 'archived')
    .map(({ id, name, path, type, status, current }) => ({ id, name: name || path, path, type, status, current }))
}

/** Resolves the safest initial owner for a new Automation draft. */
export function defaultAutomationProject(
  projects: AutomationProjectOption[],
  activeProjectId: string,
): AutomationProjectOption | undefined {
  const available = projects.filter((project) => project.status === 'available')
  return available.find((project) => project.id === activeProjectId)
    ?? available.find((project) => project.current)
    ?? available[0]
}

export function automationProjectTarget(project: AutomationProjectOption): AutomationExecutionTarget {
  return { kind: 'workspace', project_id: project.id, workspace: project.path }
}

export function automationTaskProjectID(task: Pick<AutomationTask, 'target'>): string {
  return task.target?.kind === 'workspace' ? task.target.project_id?.trim() || '' : ''
}
