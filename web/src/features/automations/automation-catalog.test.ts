import { describe, expect, it } from 'vitest'
import type { AutomationActiveRun, AutomationTask } from '@/lib/api'
import {
  automationTaskKey,
  findAutomationTaskForRun,
  normalizeAutomationTask,
} from './automation-catalog'

const baseTask: AutomationTask = {
  scope: 'workspace',
  enabled: true,
  name: 'Review',
  template: 'review',
  prompt: '',
  schedule: { kind: 'manual', hour: 9, minute: 0 },
  triggers: [],
  default_action_policy: 'auto_run',
  write_mode: 'read_only',
  write_scope: 'none',
  session_strategy: 'per_run',
  output_policy: 'run_record_only',
  output_path: '',
  recent_runs: [],
}

describe('automation catalog', () => {
  it('keeps project-qualified task identities and resolves their runs', () => {
    const fallback = { kind: 'workspace' as const, project_id: 'workspace-a', workspace: '/books/a' }
    const tasks = [
      normalizeAutomationTask({ ...baseTask, id: 'same', catalog_id: 'workspace-a:same', name: 'A', target: { kind: 'workspace', workspace: '/books/a', project_id: 'workspace-a' } }, fallback),
      normalizeAutomationTask({ ...baseTask, id: 'same', catalog_id: 'workspace-b:same', name: 'B', target: { kind: 'workspace', workspace: '/books/b', project_id: 'workspace-b' } }, fallback),
    ]
    const activeRuns: AutomationActiveRun[] = [{
      task_id: 'same',
      run: { id: 'run-b', task_id: 'same', scope: 'workspace', workspace: '/books/b', trigger: 'schedule', status: 'running', started_at: '2026-07-18T12:00:00Z', summary: '', tool_manifest: [] },
    }]

    expect(tasks.map(automationTaskKey)).toEqual(['workspace-a:same', 'workspace-b:same'])
    expect(findAutomationTaskForRun(tasks, activeRuns[0].run)?.name).toBe('B')
  })

  it('upgrades legacy scope data without depending on the active tab at runtime', () => {
    const task = normalizeAutomationTask({ ...baseTask, id: 'legacy' }, { kind: 'workspace', project_id: 'workspace-current', workspace: '/books/current' })
    expect(task.target).toEqual({ kind: 'workspace', project_id: 'workspace-current', workspace: '/books/current' })
    expect(automationTaskKey(task)).toBe('legacy')
  })
})
