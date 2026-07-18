import { describe, expect, it } from 'vitest'
import type { AutomationActiveRun, AutomationTask, BookRecord } from '@/lib/api'
import {
  automationTaskKey,
  findAutomationTaskForRun,
  groupAutomationTasks,
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
  output_policy: 'run_record_only',
  output_path: '',
  recent_runs: [],
}

describe('automation catalog', () => {
  it('groups user-owned tasks by explicit execution target and keeps IDs unique', () => {
    const books: BookRecord[] = [
      { name: 'Book A', path: '/books/a', author: '', last_opened_at: '' },
      { name: 'Book B', path: '/books/b', author: '', last_opened_at: '' },
    ]
    const tasks = [
      normalizeAutomationTask({ ...baseTask, id: 'same', catalog_id: 'workspace-a:same', name: 'A', target: { kind: 'workspace', workspace: '/books/a', workspace_id: 'workspace-a' } }, ''),
      normalizeAutomationTask({ ...baseTask, id: 'same', catalog_id: 'workspace-b:same', name: 'B', target: { kind: 'workspace', workspace: '/books/b', workspace_id: 'workspace-b' } }, ''),
      normalizeAutomationTask({ ...baseTask, id: 'global', scope: 'user', name: 'Global', target: { kind: 'user' } }, ''),
    ]
    const activeRuns: AutomationActiveRun[] = [{
      task_id: 'same',
      run: { id: 'run-b', task_id: 'same', scope: 'workspace', workspace: '/books/b', trigger: 'schedule', status: 'running', started_at: '2026-07-18T12:00:00Z', summary: '', tool_manifest: [] },
    }]

    expect(tasks.map(automationTaskKey)).toEqual(['workspace-a:same', 'workspace-b:same', 'global'])
    const groups = groupAutomationTasks(tasks, books, activeRuns)
    expect(groups.map((group) => [group.kind, group.label, group.tasks.map((task) => task.name), group.runningCount])).toEqual([
      ['user', '', ['Global'], 0],
      ['workspace', 'Book A', ['A'], 0],
      ['workspace', 'Book B', ['B'], 1],
    ])
    expect(findAutomationTaskForRun(tasks, activeRuns[0].run)?.name).toBe('B')
  })

  it('upgrades legacy scope data without depending on the active tab at runtime', () => {
    const task = normalizeAutomationTask({ ...baseTask, id: 'legacy' }, '/books/current')
    expect(task.target).toEqual({ kind: 'workspace', workspace: '/books/current' })
    expect(automationTaskKey(task)).toBe('legacy')
  })
})
