import type { AutomationActiveRun, AutomationRunRecord, AutomationTask, BookRecord } from '@/lib/api'

export interface AutomationTaskGroup {
  kind: 'user' | 'workspace'
  workspace: string
  label: string
  tasks: AutomationTask[]
  runningCount: number
}

export function normalizeAutomationTask(task: AutomationTask, currentWorkspace: string): AutomationTask {
  const target = task.target?.kind
    ? task.target
    : task.scope === 'user'
      ? { kind: 'user' as const }
      : { kind: 'workspace' as const, workspace: currentWorkspace }
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
  return findAutomationTaskByTarget(tasks, run.task_id, run.workspace)
}

export function findAutomationTaskByTarget(tasks: AutomationTask[], taskID: string, workspace?: string): AutomationTask | undefined {
  const targetWorkspace = canonicalWorkspace(workspace)
  return tasks.find((task) => {
    if (task.id !== taskID && task.catalog_id !== taskID) return false
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
  const bookLabels = new Map(books.map((book) => [canonicalWorkspace(book.path), book.name]))
  for (const task of tasks) {
    const kind = task.target?.kind || (task.scope === 'user' ? 'user' : 'workspace')
    const workspace = kind === 'workspace' ? canonicalWorkspace(task.target?.workspace) : ''
    const key = kind === 'user' ? 'user' : `workspace:${workspace}`
    const existing = groups.get(key)
    if (existing) {
      existing.tasks.push(task)
      continue
    }
    groups.set(key, {
      kind,
      workspace,
      label: kind === 'user' ? '' : bookLabels.get(workspace) || workspaceLabel(workspace),
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
    const aBookIndex = books.findIndex((book) => canonicalWorkspace(book.path) === a.workspace)
    const bBookIndex = books.findIndex((book) => canonicalWorkspace(book.path) === b.workspace)
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
