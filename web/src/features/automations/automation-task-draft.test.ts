import { describe, expect, it } from 'vitest'
import { applyAutomationTarget, newAutomationTask } from './automation-task-draft'

describe('automation task target draft', () => {
  it('removes workspace-only capabilities when a new task becomes global', () => {
    const workspaceTask = newAutomationTask({ kind: 'workspace', workspace: '/books/a' }, 'Review')
    workspaceTask.write_mode = 'auto_write'
    workspaceTask.write_scope = 'file'
    workspaceTask.output_policy = 'optional_file'
    workspaceTask.output_path = 'review.md'
    workspaceTask.triggers.push({ id: 'semantic', type: 'semantic', enabled: true, semantic_condition: 'ready' })

    const globalTask = applyAutomationTarget(workspaceTask, 'user')

    expect(globalTask.target).toEqual({ kind: 'user' })
    expect(globalTask.triggers.map((trigger) => trigger.type)).toEqual(['schedule'])
    expect(globalTask).toMatchObject({
      scope: 'user',
      template: 'custom_prompt',
      write_mode: 'read_only',
      write_scope: 'none',
      output_policy: 'run_record_only',
      output_path: '',
    })
  })
})
