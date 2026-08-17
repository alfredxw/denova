import type { AutomationActiveRun, AutomationRunRecord, AutomationTask } from '@/lib/api'

export function normalizeAutomationTask(
  task: AutomationTask,
  fallbackTarget: NonNullable<AutomationTask['target']>,
): AutomationTask {
  const target = task.target?.kind
    ? normalizeWorkspaceTarget(task.target, fallbackTarget)
    : fallbackTarget
  return {
    ...task,
    scope: target.kind === 'user' ? 'user' : 'workspace',
    target,
  }
}

export function automationTaskKey(task: AutomationTask): string {
  return task.catalog_id?.trim() || task.id?.trim() || ''
}

export function findAutomationTaskForRun(tasks: AutomationTask[], run: AutomationRunRecord): AutomationTask | undefined {
  return findAutomationTaskByTarget(tasks, run.task_id, run.workspace, run.project_id)
}

export function findAutomationTaskByTarget(
  tasks: AutomationTask[],
  taskID: string,
  workspace?: string,
  projectId?: string,
): AutomationTask | undefined {
  const targetProjectID = projectId?.trim() || ''
  const targetWorkspace = canonicalWorkspace(workspace)
  return tasks.find((task) => {
    if (task.id !== taskID && task.catalog_id !== taskID) return false
    if (targetProjectID) {
      return task.target?.kind === 'workspace' && task.target.project_id === targetProjectID
    }
    if (targetWorkspace) return task.target?.kind === 'workspace' && canonicalWorkspace(task.target.workspace) === targetWorkspace
    return false
  })
}

export function isAutomationTaskRunning(task: AutomationTask, activeRuns: AutomationActiveRun[]): boolean {
  return activeRuns.some((active) => findAutomationTaskForRun([task], active.run) === task)
}

function canonicalWorkspace(value: string | undefined): string {
  return (value || '').trim().replace(/\/+$/, '')
}

function normalizeWorkspaceTarget(
  target: NonNullable<AutomationTask['target']>,
  fallback: NonNullable<AutomationTask['target']>,
): NonNullable<AutomationTask['target']> {
  if (target.kind !== 'workspace' || fallback.kind !== 'workspace') return target
  const sameProject = Boolean(target.project_id && target.project_id === fallback.project_id)
  const sameWorkspace = Boolean(
    target.workspace && fallback.workspace && canonicalWorkspace(target.workspace) === canonicalWorkspace(fallback.workspace),
  )
  if (!sameProject && !sameWorkspace && (target.project_id || target.workspace)) return target
  return {
    kind: 'workspace',
    project_id: target.project_id || fallback.project_id,
    workspace: target.workspace || fallback.workspace,
  }
}
