import { useState, type ReactNode } from 'react'
import { Bell, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Textarea } from '@/components/ui/textarea'
import type {
  AutomationNotifyPolicy,
  AutomationTask,
  AutomationTriggerDefinition,
  AutomationTriggerType,
} from '@/lib/api'

const fieldCls = 'nova-field min-h-7 rounded-[var(--nova-radius)] border px-2.5 py-1.5 outline-none placeholder:text-[var(--nova-text-faint)] focus:border-[var(--nova-field-focus-border)] focus:bg-[var(--nova-surface-3)]'

const triggerTypes: AutomationTriggerType[] = ['schedule', 'chapter_batch', 'semantic']

export function TriggerEditor({ task, onChange }: { task: AutomationTask; onChange: (triggers: AutomationTriggerDefinition[]) => void }) {
  const { t } = useTranslation()
  const triggers = task.triggers?.length ? task.triggers : [defaultScheduleTrigger(task.schedule)]
  const [newType, setNewType] = useState<AutomationTriggerType>('semantic')
  const updateTrigger = (id: string, patch: Partial<AutomationTriggerDefinition>) => {
    onChange(triggers.map((trigger) => trigger.id === id ? normalizeDraftTrigger({ ...trigger, ...patch }, task.schedule) : trigger))
  }
  const removeTrigger = (id: string) => onChange(triggers.filter((trigger) => trigger.id !== id))
  const addTrigger = () => onChange([...triggers, newTrigger(newType, task.schedule)])
  return (
    <div className="space-y-3">
      {triggers.map((trigger) => {
        const notifyPolicy = trigger.notify_policy || defaultNotifyPolicy(trigger.type)
        return (
          <div key={trigger.id} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3">
            <div className="grid gap-3 md:grid-cols-3">
              <Field label={t('automations.trigger.enabled')}>
                <select value={String(trigger.enabled)} onChange={(e) => updateTrigger(trigger.id, { enabled: e.target.value === 'true' })} className={fieldCls}>
                  <option value="true">{t('automations.enabled')}</option>
                  <option value="false">{t('automations.disabled')}</option>
                </select>
              </Field>
              <Field label={t('automations.trigger.type')}>
                <select value={trigger.type} onChange={(e) => updateTrigger(trigger.id, { type: e.target.value as AutomationTriggerType })} className={fieldCls}>
                  {triggerTypes.map((type) => <option key={type} value={type}>{triggerTypeLabel(type, t)}</option>)}
                </select>
              </Field>
              <Field label={trigger.type === 'schedule' ? t('automations.trigger.notifyOnSchedule') : t('automations.trigger.notify')}>
                <select value={notifyPolicy} onChange={(e) => updateTrigger(trigger.id, { notify_policy: e.target.value as AutomationNotifyPolicy })} className={fieldCls}>
                  {trigger.type === 'schedule' ? (
                    <>
                      <option value="silent">{t('automations.notify.silent')}</option>
                      <option value="inbox">{t('automations.notify.inbox')}</option>
                    </>
                  ) : (
                    <>
                      <option value="inbox">{t('automations.notify.inbox')}</option>
                      <option value="silent">{t('automations.notify.silent')}</option>
                    </>
                  )}
                </select>
              </Field>
            </div>
            {trigger.type === 'schedule' && (
              <div className="mt-3">
                <ScheduleEditor schedule={trigger.schedule || task.schedule} onChange={(schedule) => updateTrigger(trigger.id, { schedule })} />
              </div>
            )}
            {trigger.type === 'semantic' && (
              <div className="mt-3 space-y-3">
                <Field label={t('automations.trigger.semanticCondition')}>
                  <Textarea autoResize value={trigger.semantic_condition || ''} onChange={(e) => updateTrigger(trigger.id, { semantic_condition: e.target.value })} placeholder={t('automations.trigger.semanticPlaceholder')} className={`${fieldCls} min-h-20 resize-y leading-5 shadow-none focus-visible:ring-0`} />
                </Field>
                <div className="grid gap-3 md:grid-cols-4">
                  <NumberInput label={t('automations.trigger.semanticBatchSize')} value={trigger.chapter_batch_size ?? 5} min={1} max={100} onChange={(v) => updateTrigger(trigger.id, { chapter_batch_size: v })} />
                </div>
              </div>
            )}
            {trigger.type === 'chapter_batch' && (
              <div className="mt-3 grid gap-3 md:grid-cols-4">
                <NumberInput label={t('automations.trigger.chapterBatchSize')} value={trigger.chapter_batch_size ?? 5} min={1} max={100} onChange={(v) => updateTrigger(trigger.id, { chapter_batch_size: v })} />
              </div>
            )}
            <div className="mt-3 flex justify-end">
              <button type="button" onClick={() => removeTrigger(trigger.id)} className="nova-nav-item inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] px-2 py-1 text-[var(--nova-text-muted)]">
                <Trash2 className="h-3.5 w-3.5" />
                {t('common.delete')}
              </button>
            </div>
          </div>
        )
      })}
      <div className="flex items-center gap-2">
        <select value={newType} onChange={(e) => setNewType(e.target.value as AutomationTriggerType)} className={fieldCls}>
          {triggerTypes.map((type) => <option key={type} value={type}>{triggerTypeLabel(type, t)}</option>)}
        </select>
        <button type="button" onClick={addTrigger} className="nova-nav-item inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-3 py-1.5 text-[var(--nova-text)]">
          <Bell className="h-3.5 w-3.5" />
          {t('automations.trigger.add')}
        </button>
      </div>
    </div>
  )
}

function ScheduleEditor({ schedule, onChange }: { schedule: AutomationTask['schedule']; onChange: (schedule: AutomationTask['schedule']) => void }) {
  const { t } = useTranslation()
  const patch = (next: Partial<AutomationTask['schedule']>) => onChange({ ...schedule, ...next })
  return (
    <div className="grid gap-3 md:grid-cols-5">
      <Field label={t('automations.schedule.kind')}>
        <select value={schedule.kind} onChange={(e) => patch({ kind: e.target.value as AutomationTask['schedule']['kind'] })} className={fieldCls}>
          <option value="manual">{t('automations.schedule.manual')}</option>
          <option value="daily">{t('automations.schedule.daily')}</option>
          <option value="weekly">{t('automations.schedule.weekly')}</option>
          <option value="monthly">{t('automations.schedule.monthly')}</option>
          <option value="every_hours">{t('automations.schedule.everyHours')}</option>
        </select>
      </Field>
      {schedule.kind === 'weekly' && <NumberInput label={t('automations.schedule.weekday')} value={schedule.weekday ?? 1} min={0} max={6} onChange={(v) => patch({ weekday: v })} />}
      {schedule.kind === 'monthly' && <NumberInput label={t('automations.schedule.day')} value={schedule.day_of_month ?? 1} min={1} max={31} onChange={(v) => patch({ day_of_month: v })} />}
      {schedule.kind === 'every_hours' && <NumberInput label={t('automations.schedule.hours')} value={schedule.every_hours ?? 6} min={1} max={168} onChange={(v) => patch({ every_hours: v })} />}
      {schedule.kind !== 'manual' && schedule.kind !== 'every_hours' && <NumberInput label={t('automations.schedule.hour')} value={schedule.hour} min={0} max={23} onChange={(v) => patch({ hour: v })} />}
      {schedule.kind !== 'manual' && <NumberInput label={t('automations.schedule.minute')} value={schedule.minute} min={0} max={59} onChange={(v) => patch({ minute: v })} />}
    </div>
  )
}

export function defaultScheduleTrigger(schedule: AutomationTask['schedule']): AutomationTriggerDefinition {
  return {
    id: 'schedule',
    type: 'schedule',
    enabled: schedule.kind !== 'manual',
    notify_policy: 'silent',
    schedule,
  }
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="flex flex-col gap-1.5 text-xs"><span className="text-[var(--nova-text-muted)]">{label}</span>{children}</label>
}

function NumberInput({ label, value, min, max, onChange }: { label: string; value: number; min: number; max: number; onChange: (value: number) => void }) {
  return (
    <Field label={label}>
      <input type="number" min={min} max={max} value={value} onChange={(e) => onChange(Number(e.target.value))} className={fieldCls} />
    </Field>
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
