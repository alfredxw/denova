import type { AutomationActiveRun, AutomationRunRecord, AutomationTask, BookRecord } from '@/lib/api'

export interface AutomationTaskGroup {
  kind: 'user' | 'workspace'
  projectId: string
  workspace: string
  label: string
  tasks: AutomationTask[]
  runningCount: number
}

export function normalizeAutomationTask(
  task: AutomationTask,
  fallbackTarget: NonNullable<AutomationTask['target']>,
): AutomationTask {
  const target = task.target?.kind
    ? normalizeWorkspaceTarget(task.target, fallbackTarget)
    : task.scope === 'user'
      ? { kind: 'user' as const }
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
    return task.target?.kind === 'user'
  })
}

export function groupAutomationTasks(
  tasks: AutomationTask[],
  books: BookRecord[],
  activeRuns: AutomationActiveRun[],
): AutomationTaskGroup[] {
  const groups = new Map<string, AutomationTaskGroup>()
  const bookLabels = new Map(books.flatMap((book) => [
    [`project:${book.project_id}`, book.name] as const,
    [`workspace:${canonicalWorkspace(book.path)}`, book.name] as const,
  ]))
  for (const task of tasks) {
    const kind = task.target?.kind || (task.scope === 'user' ? 'user' : 'workspace')
    const projectId = kind === 'workspace' ? task.target?.project_id?.trim() || '' : ''
    const workspace = kind === 'workspace' ? canonicalWorkspace(task.target?.workspace) : ''
    const key = kind === 'user' ? 'user' : projectId ? `project:${projectId}` : `workspace:${workspace}`
    const existing = groups.get(key)
    if (existing) {
      existing.tasks.push(task)
      continue
    }
    groups.set(key, {
      kind,
      projectId,
      workspace,
      label: kind === 'user'
        ? ''
        : bookLabels.get(projectId ? `project:${projectId}` : `workspace:${workspace}`) || workspaceLabel(workspace) || projectId,
      tasks: [task],
      runningCount: 0,
    })
  }
  const runningTasks = activeRuns
    .map((active) => findAutomationTaskForRun(tasks, active.run))
    .filter((task): task is AutomationTask => Boolean(task))
  const runningKeys = new Set(runningTasks.map(automationTaskKey).filter(Boolean))
  for (const group of groups.values()) {
    group.tasks.sort((a, b) => Number(runningKeys.has(automationTaskKey(b))) - Number(runningKeys.has(automationTaskKey(a))))
    group.runningCount = group.tasks.filter((task) => runningKeys.has(automationTaskKey(task))).length
  }
  const ordered = Array.from(groups.values())
  ordered.sort((a, b) => {
    if (a.kind !== b.kind) return a.kind === 'user' ? -1 : 1
    const aBookIndex = books.findIndex((book) => a.projectId ? book.project_id === a.projectId : canonicalWorkspace(book.path) === a.workspace)
    const bBookIndex = books.findIndex((book) => b.projectId ? book.project_id === b.projectId : canonicalWorkspace(book.path) === b.workspace)
    if (aBookIndex >= 0 || bBookIndex >= 0) {
      if (aBookIndex < 0) return 1
      if (bBookIndex < 0) return -1
      return aBookIndex - bBookIndex
    }
    return a.label.localeCompare(b.label)
  })
  return ordered
}

export function isAutomationTaskRunning(task: AutomationTask, activeRuns: AutomationActiveRun[]): boolean {
  return activeRuns.some((active) => findAutomationTaskForRun([task], active.run) === task)
}

function canonicalWorkspace(value: string | undefined): string {
  return (value || '').trim().replace(/\/+$/, '')
}

function workspaceLabel(workspace: string): string {
  const parts = workspace.split('/').filter(Boolean)
  return parts.at(-1) || workspace
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
