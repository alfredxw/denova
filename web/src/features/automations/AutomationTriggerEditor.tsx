import { useState } from 'react'
import { Bell, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { FormField } from '@/components/forms/form-field'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import type {
  AutomationNotifyPolicy,
  AutomationTask,
  AutomationTriggerDefinition,
  AutomationTriggerType,
} from '@/lib/api'
import { defaultScheduleTrigger } from './automation-trigger'

const controlClassName = 'nova-field w-full min-w-0 rounded-[var(--nova-radius)] border text-xs'
const workspaceTriggerTypes: AutomationTriggerType[] = ['schedule', 'chapter_batch', 'semantic']

export function TriggerEditor({ task, onChange }: { task: AutomationTask; onChange: (triggers: AutomationTriggerDefinition[]) => void }) {
  const { t } = useTranslation()
  const triggers = task.triggers?.length ? task.triggers : [defaultScheduleTrigger(task.schedule)]
  const triggerTypes = task.target?.kind === 'user' ? ['schedule'] as AutomationTriggerType[] : workspaceTriggerTypes
  const [newType, setNewType] = useState<AutomationTriggerType>('schedule')
  const selectedNewType = triggerTypes.includes(newType) ? newType : triggerTypes[0]
  const updateTrigger = (id: string, patch: Partial<AutomationTriggerDefinition>) => {
    onChange(triggers.map((trigger) => trigger.id === id ? normalizeDraftTrigger({ ...trigger, ...patch }, task.schedule) : trigger))
  }
  const removeTrigger = (id: string) => onChange(triggers.filter((trigger) => trigger.id !== id))
  const addTrigger = () => onChange([...triggers, newTrigger(selectedNewType, task.schedule)])

  return (
    <div className="flex flex-col gap-3">
      {triggers.map((trigger) => {
        const notifyPolicy = trigger.notify_policy || defaultNotifyPolicy(trigger.type)
        return (
          <div key={trigger.id} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3">
            <div className="grid gap-3 md:grid-cols-3">
              <FormField label={t('automations.trigger.enabled')}>
                <div className="flex h-8 items-center gap-2">
                  <Switch
                    checked={trigger.enabled}
                    onCheckedChange={(enabled) => updateTrigger(trigger.id, { enabled })}
                    aria-label={t('automations.trigger.enabled')}
                  />
                  <span className="text-[11px] text-muted-foreground">
                    {trigger.enabled ? t('automations.enabled') : t('automations.disabled')}
                  </span>
                </div>
              </FormField>
              <FormField label={t('automations.trigger.type')}>
                <Select value={trigger.type} onValueChange={(type) => updateTrigger(trigger.id, { type: type as AutomationTriggerType })}>
                  <SelectTrigger className={controlClassName} aria-label={t('automations.trigger.type')}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {triggerTypes.map((type) => <SelectItem key={type} value={type}>{triggerTypeLabel(type, t)}</SelectItem>)}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </FormField>
              <FormField label={trigger.type === 'schedule' ? t('automations.trigger.notifyOnSchedule') : t('automations.trigger.notify')}>
                <Select value={notifyPolicy} onValueChange={(policy) => updateTrigger(trigger.id, { notify_policy: policy as AutomationNotifyPolicy })}>
                  <SelectTrigger className={controlClassName} aria-label={trigger.type === 'schedule' ? t('automations.trigger.notifyOnSchedule') : t('automations.trigger.notify')}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {trigger.type === 'schedule' ? (
                        <>
                          <SelectItem value="silent">{t('automations.notify.silent')}</SelectItem>
                          <SelectItem value="inbox">{t('automations.notify.inbox')}</SelectItem>
                        </>
                      ) : (
                        <>
                          <SelectItem value="inbox">{t('automations.notify.inbox')}</SelectItem>
                          <SelectItem value="silent">{t('automations.notify.silent')}</SelectItem>
                        </>
                      )}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </FormField>
            </div>
            {trigger.type === 'schedule' && (
              <div className="mt-3">
                <ScheduleEditor schedule={trigger.schedule || task.schedule} onChange={(schedule) => updateTrigger(trigger.id, { schedule })} />
              </div>
            )}
            {trigger.type === 'semantic' && (
              <div className="mt-3 flex flex-col gap-3">
                <FormField label={t('automations.trigger.semanticCondition')}>
                  <Textarea
                    autoResize
                    value={trigger.semantic_condition || ''}
                    onChange={(event) => updateTrigger(trigger.id, { semantic_condition: event.target.value })}
                    placeholder={t('automations.trigger.semanticPlaceholder')}
                    aria-label={t('automations.trigger.semanticCondition')}
                    className={`${controlClassName} min-h-20 resize-y leading-5 shadow-none focus-visible:ring-0`}
                  />
                </FormField>
                <div className="grid gap-3 md:grid-cols-4">
                  <NumberInput label={t('automations.trigger.semanticBatchSize')} value={trigger.chapter_batch_size ?? 5} min={1} max={100} onChange={(value) => updateTrigger(trigger.id, { chapter_batch_size: value })} />
                </div>
              </div>
            )}
            {trigger.type === 'chapter_batch' && (
              <div className="mt-3 grid gap-3 md:grid-cols-4">
                <NumberInput label={t('automations.trigger.chapterBatchSize')} value={trigger.chapter_batch_size ?? 5} min={1} max={100} onChange={(value) => updateTrigger(trigger.id, { chapter_batch_size: value })} />
              </div>
            )}
            <div className="mt-3 flex justify-end">
              <Button type="button" size="sm" variant="ghost" onClick={() => removeTrigger(trigger.id)} className="nova-nav-item text-[var(--nova-text-muted)]">
                <Trash2 data-icon="inline-start" />
                {t('common.delete')}
              </Button>
            </div>
          </div>
        )
      })}
      <div className="flex items-center gap-2">
        <Select value={selectedNewType} onValueChange={(type) => setNewType(type as AutomationTriggerType)}>
          <SelectTrigger className={`${controlClassName} w-auto min-w-40`} aria-label={t('automations.trigger.type')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {triggerTypes.map((type) => <SelectItem key={type} value={type}>{triggerTypeLabel(type, t)}</SelectItem>)}
            </SelectGroup>
          </SelectContent>
        </Select>
        <Button type="button" size="sm" variant="secondary" onClick={addTrigger} className="nova-nav-item border border-[var(--nova-border)] bg-[var(--nova-active)]">
          <Bell data-icon="inline-start" />
          {t('automations.trigger.add')}
        </Button>
      </div>
    </div>
  )
}

function ScheduleEditor({ schedule, onChange }: { schedule: AutomationTask['schedule']; onChange: (schedule: AutomationTask['schedule']) => void }) {
  const { t } = useTranslation()
  const patch = (next: Partial<AutomationTask['schedule']>) => onChange({ ...schedule, ...next })
  return (
    <div className="grid gap-3 md:grid-cols-5">
      <FormField label={t('automations.schedule.kind')}>
        <Select value={schedule.kind} onValueChange={(kind) => patch({ kind: kind as AutomationTask['schedule']['kind'] })}>
          <SelectTrigger className={controlClassName} aria-label={t('automations.schedule.kind')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="manual">{t('automations.schedule.manual')}</SelectItem>
              <SelectItem value="daily">{t('automations.schedule.daily')}</SelectItem>
              <SelectItem value="weekly">{t('automations.schedule.weekly')}</SelectItem>
              <SelectItem value="monthly">{t('automations.schedule.monthly')}</SelectItem>
              <SelectItem value="every_hours">{t('automations.schedule.everyHours')}</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      </FormField>
      {schedule.kind === 'weekly' && <NumberInput label={t('automations.schedule.weekday')} value={schedule.weekday ?? 1} min={0} max={6} onChange={(value) => patch({ weekday: value })} />}
      {schedule.kind === 'monthly' && <NumberInput label={t('automations.schedule.day')} value={schedule.day_of_month ?? 1} min={1} max={31} onChange={(value) => patch({ day_of_month: value })} />}
      {schedule.kind === 'every_hours' && <NumberInput label={t('automations.schedule.hours')} value={schedule.every_hours ?? 6} min={1} max={168} onChange={(value) => patch({ every_hours: value })} />}
      {schedule.kind !== 'manual' && schedule.kind !== 'every_hours' && <NumberInput label={t('automations.schedule.hour')} value={schedule.hour} min={0} max={23} onChange={(value) => patch({ hour: value })} />}
      {schedule.kind !== 'manual' && <NumberInput label={t('automations.schedule.minute')} value={schedule.minute} min={0} max={59} onChange={(value) => patch({ minute: value })} />}
    </div>
  )
}

function NumberInput({ label, value, min, max, onChange }: { label: string; value: number; min: number; max: number; onChange: (value: number) => void }) {
  return (
    <FormField label={label}>
      <Input
        type="number"
        min={min}
        max={max}
        value={value}
        onChange={(event) => onChange(Number(event.target.value))}
        aria-label={label}
        className={controlClassName}
      />
    </FormField>
  )
}

function newTrigger(type: AutomationTriggerType, schedule: AutomationTask['schedule']): AutomationTriggerDefinition {
  return normalizeDraftTrigger({
    id: `${type}_${Date.now().toString(36)}`,
    type,
    enabled: true,
    notify_policy: defaultNotifyPolicy(type),
    schedule: type === 'schedule' ? schedule : undefined,
    chapter_batch_size: type === 'chapter_batch' || type === 'semantic' ? 5 : undefined,
  }, schedule)
}

function normalizeDraftTrigger(trigger: AutomationTriggerDefinition, fallbackSchedule: AutomationTask['schedule']): AutomationTriggerDefinition {
  const next = { ...trigger }
  if (next.type === 'schedule') {
    next.schedule = next.schedule || fallbackSchedule
    next.notify_policy = next.notify_policy || 'silent'
    next.chapter_batch_size = undefined
  } else {
    next.schedule = undefined
    next.notify_policy = next.notify_policy || 'inbox'
    if (next.type === 'chapter_batch' || next.type === 'semantic') {
      next.chapter_batch_size = next.chapter_batch_size || 5
    } else {
      next.chapter_batch_size = undefined
    }
  }
  next.action_policy = undefined
  if (next.notify_policy !== 'silent' && next.notify_policy !== 'inbox') {
    next.notify_policy = 'inbox'
  }
  return next
}

function defaultNotifyPolicy(type: AutomationTriggerType): AutomationNotifyPolicy {
  return type === 'schedule' ? 'silent' : 'inbox'
}

function triggerTypeLabel(type: AutomationTriggerType, t: (key: string) => string) {
  return t(`automations.trigger.type.${type}`)
}
