import { describe, expect, it } from 'vitest'
import type { AutomationTaskTemplate } from '@/lib/api'
import { newAutomationTaskFromTemplate } from './automation-task-draft'

describe('automation task target draft', () => {
  it('copies a template into an independent disabled workspace draft', () => {
    const template: AutomationTaskTemplate = {
      id: 'review',
      version: 1,
      description: 'Review every five chapters',
      target_kinds: ['workspace'],
      defaults: {
        enabled: false,
        name: 'Automatic Review',
        template: 'review',
        prompt: 'Review new chapters',
        schedule: { kind: 'manual', hour: 9, minute: 0 },
        triggers: [{ id: 'chapter_batch_review', type: 'chapter_batch', enabled: true, chapter_batch_size: 5 }],
        default_action_policy: 'auto_run',
        write_mode: 'read_only',
        write_scope: 'none',
        output_policy: 'run_record_only',
        output_path: '',
      },
    }

    const draft = newAutomationTaskFromTemplate(template, { kind: 'workspace', workspace: '/books/a' })
    draft.triggers[0].chapter_batch_size = 8

    expect(draft).toMatchObject({
      scope: 'workspace',
      target: { kind: 'workspace', workspace: '/books/a' },
      enabled: false,
      name: 'Automatic Review',
      prompt: 'Review new chapters',
    })
    expect(template.defaults.triggers[0].chapter_batch_size).toBe(5)
  })
})
