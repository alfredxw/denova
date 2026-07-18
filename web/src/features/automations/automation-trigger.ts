import type { AutomationTask, AutomationTriggerDefinition } from '@/lib/api'

export function defaultScheduleTrigger(schedule: AutomationTask['schedule']): AutomationTriggerDefinition {
  return {
    id: 'schedule',
    type: 'schedule',
    enabled: schedule.kind !== 'manual',
    notify_policy: 'silent',
    schedule,
  }
}
