import type { AutomationTask, BookRecord } from '@/lib/api'
import { automationTaskKey, normalizeAutomationTask } from './automation-catalog'
import { defaultScheduleTrigger } from './automation-trigger'

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
    default_action_policy: 'auto_run',
    write_policy: 'read_only',
    write_mode: 'read_only',
    write_scope: 'none',
    output_policy: 'run_record_only',
    output_path: '',
    recent_runs: [],
  }
}

export function cloneAutomationTask(task: AutomationTask, workspace: string): AutomationTask {
  return normalizeAutomationTaskShape(JSON.parse(JSON.stringify(task)) as AutomationTask, workspace)
}

export function normalizeAutomationTaskShape(task: AutomationTask, workspace: string): AutomationTask {
  task = normalizeAutomationTask(task, workspace)
  if (task.write_mode && task.write_scope) {
    return { ...task, default_action_policy: actionPolicyForWriteMode() }
  }
  const legacy = task.write_policy || 'read_only'
  if (legacy === 'allow_lore_write') return { ...task, default_action_policy: 'auto_run', write_mode: 'auto_write', write_scope: 'lore' }
  if (legacy === 'allow_file_write') return { ...task, default_action_policy: 'auto_run', write_mode: 'auto_write', write_scope: 'file' }
  if (legacy === 'allow_lore_and_file_write') return { ...task, default_action_policy: 'auto_run', write_mode: 'auto_write', write_scope: 'lore_and_file' }
  return { ...task, default_action_policy: 'auto_run', write_policy: 'read_only', write_mode: 'read_only', write_scope: 'none' }
}

export function nextAutomationWriteModePatch(task: AutomationTask, writeMode: AutomationTask['write_mode']): Partial<AutomationTask> {
  if (writeMode === 'read_only') {
    return { default_action_policy: actionPolicyForWriteMode(), write_mode: 'read_only', write_scope: 'none', write_policy: 'read_only' }
  }
  const scope = task.write_scope === 'none' ? 'file' : task.write_scope
  return { default_action_policy: actionPolicyForWriteMode(), write_mode: writeMode, write_scope: scope, write_policy: legacyWritePolicyForScope(scope) }
}

export function nextAutomationWriteScopePatch(task: AutomationTask, writeScope: AutomationTask['write_scope']): Partial<AutomationTask> {
  if (task.write_mode === 'read_only' || writeScope === 'none') {
    return { write_mode: 'read_only', write_scope: 'none', write_policy: 'read_only' }
  }
  return { write_scope: writeScope, write_policy: legacyWritePolicyForScope(writeScope) }
}

export function upsertAutomationTask(tasks: AutomationTask[], task: AutomationTask) {
  const index = tasks.findIndex((item) => automationTaskKey(item) === automationTaskKey(task))
  if (index < 0) return [task, ...tasks]
  const next = tasks.slice()
  next[index] = task
  return next
}

export function defaultAutomationTarget(workspace: string): NonNullable<AutomationTask['target']> {
  return workspace ? { kind: 'workspace', workspace } : { kind: 'user' }
}

export function automationTargetValue(task: AutomationTask): string {
  return task.target?.kind === 'workspace' ? `workspace:${task.target.workspace || ''}` : 'user'
}

export function applyAutomationTarget(task: AutomationTask, value: string): AutomationTask {
  if (value === 'user') {
    const scheduleTrigger = task.triggers.find((trigger) => trigger.type === 'schedule') || defaultScheduleTrigger(task.schedule)
    return {
      ...task,
      scope: 'user',
      target: { kind: 'user' },
      template: 'custom_prompt',
      triggers: [scheduleTrigger],
      write_policy: 'read_only',
      write_mode: 'read_only',
      write_scope: 'none',
      output_policy: 'run_record_only',
      output_path: '',
    }
  }
  return {
    ...task,
    scope: 'workspace',
    target: { kind: 'workspace', workspace: value.slice('workspace:'.length) },
  }
}

export function automationTargetOptions(books: BookRecord[], task: AutomationTask): BookRecord[] {
  const workspace = task.target?.kind === 'workspace' ? task.target.workspace?.trim() : ''
  if (!workspace || books.some((book) => book.path === workspace)) return books
  return [{ name: workspace.split('/').filter(Boolean).at(-1) || workspace, path: workspace, author: '', last_opened_at: '' }, ...books]
}

export function automationTargetLabel(task: AutomationTask, books: BookRecord[], t: (key: string, options?: Record<string, unknown>) => string) {
  if (task.target?.kind !== 'workspace') return t('automations.target.global')
  const workspace = task.target.workspace || ''
  const name = books.find((book) => book.path === workspace)?.name || workspace.split('/').filter(Boolean).at(-1) || workspace
  return t('automations.target.workspace', { name })
}

function legacyWritePolicyForScope(writeScope: AutomationTask['write_scope']): AutomationTask['write_policy'] {
  if (writeScope === 'lore') return 'allow_lore_write'
  if (writeScope === 'file') return 'allow_file_write'
  if (writeScope === 'lore_and_file') return 'allow_lore_and_file_write'
  return 'read_only'
}

function actionPolicyForWriteMode(): AutomationTask['default_action_policy'] {
  return 'auto_run'
}
