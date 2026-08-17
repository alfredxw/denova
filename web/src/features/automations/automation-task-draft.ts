import type { AutomationTask, AutomationTaskTemplate, AutomationTaskUpdate } from '@/lib/api'
import { automationTaskKey, normalizeAutomationTask } from './automation-catalog'
import { defaultScheduleTrigger } from './automation-trigger'

const defaultAutomationActionPolicy = 'auto_run' as const

/** Strips server-owned run and trigger state before a configuration PATCH. */
export function automationTaskUpdate(task: AutomationTask): AutomationTaskUpdate {
  return {
    enabled: task.enabled,
    name: task.name,
    template: task.template,
    prompt: task.prompt,
    model_profile_id: task.model_profile_id,
    schedule: task.schedule,
    triggers: task.triggers,
    default_action_policy: task.default_action_policy,
    session_strategy: task.session_strategy,
  }
}

export function automationTaskDraftSignature(task: Partial<AutomationTask> | AutomationTaskUpdate): string {
  return JSON.stringify(automationTaskUpdate(task as AutomationTask))
}

export function newAutomationTask(target: NonNullable<AutomationTask['target']>, name: string): AutomationTask {
  const schedule = { kind: 'manual', hour: 9, minute: 0, weekday: 1, day_of_month: 1, every_hours: 6 } satisfies AutomationTask['schedule']
  return {
    scope: target.kind === 'user' ? 'user' : 'workspace',
    target,
    enabled: false,
    name,
    template: 'custom_prompt',
    prompt: '',
    model_profile_id: '',
    schedule,
    triggers: [defaultScheduleTrigger(schedule)],
    default_action_policy: defaultAutomationActionPolicy,
    session_strategy: 'per_run',
    recent_runs: [],
  }
}

export function newAutomationTaskFromTemplate(
  template: AutomationTaskTemplate,
  target: NonNullable<AutomationTask['target']>,
): AutomationTask {
  if (!template.target_kinds.includes(target.kind)) {
    throw new Error(`Automation template ${template.id} does not support target ${target.kind}`)
  }
  const defaults = JSON.parse(JSON.stringify(template.defaults)) as AutomationTaskTemplate['defaults']
  return normalizeAutomationTaskShape({
    ...newAutomationTask(target, defaults.name),
    ...defaults,
    scope: target.kind === 'user' ? 'user' : 'workspace',
    target,
    recent_runs: [],
  }, target)
}

export function cloneAutomationTask(
  task: AutomationTask,
  fallbackTarget: NonNullable<AutomationTask['target']>,
): AutomationTask {
  return normalizeAutomationTaskShape(JSON.parse(JSON.stringify(task)) as AutomationTask, fallbackTarget)
}

// normalizeAutomationTaskShape canonicalizes server-owned defaults without
// exposing execution policy that belongs to the Project Agent and task Prompt.
export function normalizeAutomationTaskShape(
  task: AutomationTask,
  fallbackTarget: NonNullable<AutomationTask['target']>,
): AutomationTask {
  task = normalizeAutomationTask(task, fallbackTarget)
  return {
    ...task,
    default_action_policy: defaultAutomationActionPolicy,
    session_strategy: task.session_strategy === 'per_task' ? 'per_task' : 'per_run',
  }
}

export function upsertAutomationTask(tasks: AutomationTask[], task: AutomationTask) {
  const index = tasks.findIndex((item) => automationTaskKey(item) === automationTaskKey(task))
  if (index < 0) return [task, ...tasks]
  const next = tasks.slice()
  next[index] = task
  return next
}

export function defaultAutomationTarget(project: { projectId: string; workspace: string }): NonNullable<AutomationTask['target']> {
  const projectId = project.projectId.trim()
  const workspace = project.workspace.trim()
  return { kind: 'workspace', project_id: projectId, ...(workspace ? { workspace } : {}) }
}
