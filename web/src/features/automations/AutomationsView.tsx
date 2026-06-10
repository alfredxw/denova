import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { Clock3, FileText, Play, Save, Trash2, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { Textarea } from '@/components/ui/textarea'
import {
  createAutomation,
  deleteAutomation,
  getAutomations,
  runAutomation,
  updateAutomation,
  type AutomationScheduleKind,
  type AutomationTask,
} from '@/lib/api'

const fieldCls = 'nova-field min-h-7 rounded-[var(--nova-radius)] border px-2.5 py-1.5 outline-none placeholder:text-[var(--nova-text-faint)] focus:border-[#3a3a3a] focus:bg-[var(--nova-surface-3)]'
const tabCls = 'nova-nav-item rounded-[var(--nova-radius)] px-2.5 py-1 text-xs'

export function AutomationsView({ onClose }: { workspace: string; onClose?: () => void }) {
  const { t } = useTranslation()
  const [tasks, setTasks] = useState<AutomationTask[]>([])
  const [activeId, setActiveId] = useState<string>('')
  const [draft, setDraft] = useState<AutomationTask>(() => newTask('workspace'))
  const [scopeFilter, setScopeFilter] = useState<'workspace' | 'user'>('workspace')
  const [saving, setSaving] = useState(false)
  const [running, setRunning] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      const data = await getAutomations()
      setTasks(data)
      const first = data.find((task) => task.scope === scopeFilter) ?? data[0]
      if (first) {
        setActiveId(first.id || '')
        setDraft(cloneTask(first))
      } else {
        setActiveId('')
        setDraft(newTask(scopeFilter))
      }
    } catch (e) {
      setError((e as Error).message)
    }
  }, [scopeFilter])

  useEffect(() => { void load() }, [load])

  const filteredTasks = useMemo(() => tasks.filter((task) => task.scope === scopeFilter), [scopeFilter, tasks])
  const schedulePreview = scheduleToText(draft.schedule, t)

  const selectTask = (task: AutomationTask) => {
    setActiveId(task.id || '')
    setDraft(cloneTask(task))
  }

  const createNew = (scope: 'workspace' | 'user') => {
    setActiveId('')
    setScopeFilter(scope)
    setDraft(newTask(scope))
  }

  const switchScope = (scope: 'workspace' | 'user') => {
    setScopeFilter(scope)
    const first = tasks.find((task) => task.scope === scope)
    if (first) {
      setActiveId(first.id || '')
      setDraft(cloneTask(first))
      return
    }
    setActiveId('')
    setDraft(newTask(scope))
  }

  const save = async () => {
    setSaving(true)
    setError(null)
    try {
      const saved = activeId ? await updateAutomation(activeId, draft) : await createAutomation(draft)
      setActiveId(saved.id || '')
      setDraft(cloneTask(saved))
      setTasks((current) => upsertTask(current, saved))
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const remove = async () => {
    if (!activeId) return
    setSaving(true)
    setError(null)
    try {
      await deleteAutomation(activeId)
      const next = tasks.filter((task) => task.id !== activeId)
      setTasks(next)
      const fallback = next.find((task) => task.scope === scopeFilter)
      setActiveId(fallback?.id || '')
      setDraft(fallback ? cloneTask(fallback) : newTask(scopeFilter))
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const runNow = async () => {
    if (!activeId) return
    setRunning(true)
    setError(null)
    try {
      const result = await runAutomation(activeId)
      setDraft(cloneTask(result.task))
      setTasks((current) => upsertTask(current, result.task))
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setRunning(false)
    }
  }

  const setDraftField = (patch: Partial<AutomationTask>) => setDraft((current) => ({ ...current, ...patch }))
  const setTemplate = (template: AutomationTask['template']) => {
    setDraft((current) => ({
      ...current,
      template,
      write_policy: template === 'continue_writing' && current.write_policy === 'read_only' ? 'allow_file_write' : current.write_policy,
    }))
  }

  return (
    <div className="flex h-full min-h-0 w-full flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <div className="nova-topbar flex min-h-10 shrink-0 flex-wrap items-center gap-2 border-b px-4 py-1.5 text-xs">
        <Clock3 className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
        <span className="font-medium">{t('automations.title')}</span>
        <div className="ml-3 flex gap-1 border-l border-[var(--nova-border)] pl-3">
          {(['workspace', 'user'] as const).map((scope) => (
            <button key={scope} type="button" onClick={() => switchScope(scope)} className={`${tabCls} ${scopeFilter === scope ? 'is-active' : 'bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]'}`}>
              {scope === 'workspace' ? t('automations.scope.workspace') : t('automations.scope.user')}
            </button>
          ))}
        </div>
        <button type="button" onClick={runNow} disabled={!activeId || running || saving} className="nova-nav-item ml-auto inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-3 py-1 text-[var(--nova-text)] disabled:opacity-50">
          <Play className="h-3.5 w-3.5" />
          {running ? t('automations.running') : t('automations.runNow')}
        </button>
        <button type="button" onClick={save} disabled={saving || running} className="nova-nav-item inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-3 py-1 text-[var(--nova-text)] disabled:opacity-50">
          <Save className="h-3.5 w-3.5" />
          {saving ? t('common.saving') : t('common.save')}
        </button>
        {onClose && (
          <button type="button" onClick={onClose} className="nova-nav-item rounded p-1" aria-label={t('automations.close')} title={t('automations.close')}>
            <X className="h-3.5 w-3.5" />
          </button>
        )}
      </div>

      {error && <InlineErrorNotice className="mx-3 mt-2" message={error} title={t('automations.error')} />}

      <div className="grid min-h-0 flex-1 grid-cols-[18rem_minmax(0,1fr)] text-xs">
        <aside className="min-h-0 overflow-y-auto border-r border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
          <button type="button" onClick={() => createNew(scopeFilter)} className="nova-nav-item mb-3 w-full rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-3 py-2 text-left">
            {t('automations.newTask')}
          </button>
          <div className="space-y-1">
            {filteredTasks.length === 0 ? (
              <div className="px-2 py-8 text-center text-[var(--nova-text-faint)]">{t('automations.empty')}</div>
            ) : filteredTasks.map((task) => (
              <button key={task.id} type="button" onClick={() => selectTask(task)} className={`nova-nav-item flex w-full items-start gap-2 rounded-[var(--nova-radius)] px-2.5 py-2 text-left ${activeId === task.id ? 'is-active' : ''}`}>
                <FileText className="mt-0.5 h-4 w-4 shrink-0 text-[var(--nova-text-muted)]" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium text-[var(--nova-text)]">{task.name}</span>
                  <span className="mt-0.5 block truncate text-[11px] text-[var(--nova-text-faint)]">{templateLabel(task.template, t)} · {task.enabled ? t('automations.enabled') : t('automations.disabled')}</span>
                </span>
              </button>
            ))}
          </div>
        </aside>

        <main className="min-h-0 overflow-y-auto">
          <div className="mx-auto flex max-w-5xl flex-col gap-5 px-6 py-5">
            <section className="grid gap-3 border-b border-[var(--nova-border)] pb-5 md:grid-cols-2">
              <Field label={t('automations.field.name')}>
                <input value={draft.name} onChange={(e) => setDraftField({ name: e.target.value })} className={fieldCls} />
              </Field>
              <Field label={t('automations.field.enabled')}>
                <select value={String(draft.enabled)} onChange={(e) => setDraftField({ enabled: e.target.value === 'true' })} className={fieldCls}>
                  <option value="true">{t('automations.enabled')}</option>
                  <option value="false">{t('automations.disabled')}</option>
                </select>
              </Field>
              <Field label={t('automations.field.template')}>
                <select value={draft.template} onChange={(e) => setTemplate(e.target.value as AutomationTask['template'])} className={fieldCls}>
                  <option value="memory_consolidation">{t('automations.template.memory')}</option>
                  <option value="review">{t('automations.template.review')}</option>
                  <option value="continue_writing">{t('automations.template.continueWriting')}</option>
                  <option value="custom_prompt">{t('automations.template.custom')}</option>
                </select>
              </Field>
              <Field label={t('automations.field.scope')}>
                <select value={draft.scope} disabled={Boolean(activeId)} onChange={(e) => setDraftField({ scope: e.target.value as AutomationTask['scope'] })} className={fieldCls}>
                  <option value="workspace">{t('automations.scope.workspace')}</option>
                  <option value="user">{t('automations.scope.user')}</option>
                </select>
              </Field>
              <div className="md:col-span-2">
                <Field label={t('automations.field.prompt')}>
                  <Textarea autoResize value={draft.prompt} onChange={(e) => setDraftField({ prompt: e.target.value })} placeholder={t('automations.prompt.placeholder')} className={`${fieldCls} min-h-32 resize-y leading-5 shadow-none focus-visible:ring-0`} />
                </Field>
              </div>
            </section>

            <section className="grid gap-3 border-b border-[var(--nova-border)] pb-5 md:grid-cols-2">
              <Field label={t('automations.field.writePolicy')}>
                <select value={draft.write_policy} onChange={(e) => setDraftField({ write_policy: e.target.value as AutomationTask['write_policy'] })} className={fieldCls}>
                  <option value="read_only">{t('automations.write.readOnly')}</option>
                  <option value="allow_lore_write">{t('automations.write.lore')}</option>
                  <option value="allow_file_write">{t('automations.write.file')}</option>
                  <option value="allow_lore_and_file_write">{t('automations.write.loreFile')}</option>
                </select>
              </Field>
              <Field label={t('automations.field.outputPolicy')}>
                <select value={draft.output_policy} onChange={(e) => setDraftField({ output_policy: e.target.value as AutomationTask['output_policy'] })} className={fieldCls}>
                  <option value="run_record_only">{t('automations.output.record')}</option>
                  <option value="optional_file">{t('automations.output.file')}</option>
                </select>
              </Field>
              <div className="md:col-span-2">
                <Field label={t('automations.field.outputPath')}>
                  <input value={draft.output_path} onChange={(e) => setDraftField({ output_path: e.target.value })} placeholder="reports/automation-review.md" className={fieldCls} />
                </Field>
              </div>
            </section>

            <section className="space-y-3 border-b border-[var(--nova-border)] pb-5">
              <SectionTitle title={t('automations.section.schedule')} />
              <ScheduleEditor task={draft} onChange={(schedule) => setDraftField({ schedule })} />
              <div className="text-[11px] text-[var(--nova-text-faint)]">{schedulePreview}</div>
            </section>

            <section className="space-y-3 pb-5">
              <div className="flex items-center justify-between">
                <SectionTitle title={t('automations.section.runs')} />
                {activeId && <button type="button" onClick={remove} disabled={saving || running} className="nova-nav-item inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] px-2 py-1 text-[var(--nova-text-muted)] disabled:opacity-50"><Trash2 className="h-3.5 w-3.5" />{t('common.delete')}</button>}
              </div>
              <RunList task={draft} />
            </section>
          </div>
        </main>
      </div>
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="flex flex-col gap-1.5 text-xs"><span className="text-[var(--nova-text-muted)]">{label}</span>{children}</label>
}

function SectionTitle({ title }: { title: string }) {
  return <div className="text-xs font-medium text-[var(--nova-text)]">{title}</div>
}

function NumberInput({ label, value, min, max, onChange }: { label: string; value: number; min: number; max: number; onChange: (value: number) => void }) {
  return (
    <Field label={label}>
      <input type="number" min={min} max={max} value={value} onChange={(e) => onChange(Number(e.target.value))} className={fieldCls} />
    </Field>
  )
}

function ScheduleEditor({ task, onChange }: { task: AutomationTask; onChange: (schedule: AutomationTask['schedule']) => void }) {
  const { t } = useTranslation()
  const schedule = task.schedule
  const patch = (next: Partial<AutomationTask['schedule']>) => onChange({ ...schedule, ...next })
  return (
    <div className="grid gap-3 md:grid-cols-5">
      <Field label={t('automations.schedule.kind')}>
        <select value={schedule.kind} onChange={(e) => patch({ kind: e.target.value as AutomationScheduleKind })} className={fieldCls}>
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

function RunList({ task }: { task: AutomationTask }) {
  const { t } = useTranslation()
  const runs = task.recent_runs || []
  if (runs.length === 0) return <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-8 text-center text-[var(--nova-text-faint)]">{t('automations.runs.empty')}</div>
  return (
    <div className="space-y-2">
      {runs.slice(0, 5).map((run) => (
        <div key={run.id} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
          <div className="flex items-center gap-2">
            <span className="font-medium">{run.status}</span>
            <span className="text-[11px] text-[var(--nova-text-faint)]">{new Date(run.started_at).toLocaleString()}</span>
            {run.output_path && <span className="ml-auto truncate text-[11px] text-[var(--nova-text-faint)]">{run.output_path}</span>}
          </div>
          <div className="mt-1 line-clamp-3 whitespace-pre-wrap text-[11px] leading-5 text-[var(--nova-text-muted)]">{run.error || run.summary}</div>
        </div>
      ))}
    </div>
  )
}

function newTask(scope: 'workspace' | 'user'): AutomationTask {
  return {
    scope,
    enabled: false,
    name: scope === 'workspace' ? 'Workspace review' : 'User memory consolidation',
    template: scope === 'workspace' ? 'review' : 'memory_consolidation',
    prompt: '',
    schedule: { kind: 'manual', hour: 9, minute: 0, weekday: 1, day_of_month: 1, every_hours: 6 },
    write_policy: 'read_only',
    output_policy: 'run_record_only',
    output_path: '',
    recent_runs: [],
  }
}

function cloneTask(task: AutomationTask): AutomationTask {
  return JSON.parse(JSON.stringify(task)) as AutomationTask
}

function upsertTask(tasks: AutomationTask[], task: AutomationTask) {
  const index = tasks.findIndex((item) => item.id === task.id)
  if (index < 0) return [task, ...tasks]
  const next = tasks.slice()
  next[index] = task
  return next
}

function templateLabel(template: AutomationTask['template'], t: (key: string) => string) {
  if (template === 'memory_consolidation') return t('automations.template.memory')
  if (template === 'review') return t('automations.template.review')
  if (template === 'continue_writing') return t('automations.template.continueWriting')
  return t('automations.template.custom')
}

function scheduleToText(schedule: AutomationTask['schedule'], t: (key: string, options?: Record<string, unknown>) => string) {
  if (schedule.kind === 'manual') return t('automations.schedule.previewManual')
  if (schedule.kind === 'daily') return t('automations.schedule.previewDaily', { hour: schedule.hour, minute: pad2(schedule.minute) })
  if (schedule.kind === 'weekly') return t('automations.schedule.previewWeekly', { weekday: schedule.weekday ?? 1, hour: schedule.hour, minute: pad2(schedule.minute) })
  if (schedule.kind === 'monthly') return t('automations.schedule.previewMonthly', { day: schedule.day_of_month ?? 1, hour: schedule.hour, minute: pad2(schedule.minute) })
  return t('automations.schedule.previewEveryHours', { hours: schedule.every_hours ?? 6, minute: pad2(schedule.minute) })
}

function pad2(value: number) {
  return String(value).padStart(2, '0')
}
