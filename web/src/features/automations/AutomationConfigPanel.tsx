import { Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/common/EmptyState'
import { FormField } from '@/components/forms/form-field'
import { FormSectionHeader } from '@/components/forms/form-section-header'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import type {
  AutomationRunRecord,
  AutomationTask,
  AutomationTriggerDefinition,
  BookRecord,
} from '@/lib/api'
import {
  automationTargetLabel,
  automationTargetOptions,
  automationTargetValue,
  nextAutomationWriteModePatch,
  nextAutomationWriteScopePatch,
} from './automation-task-draft'
import { TriggerEditor } from './AutomationTriggerEditor'

const controlClassName = 'nova-field min-h-7 w-full min-w-0 rounded-[var(--nova-radius)] border text-xs'

interface AutomationConfigPanelProps {
  activeId: string
  activeRunId: string
  books: BookRecord[]
  draft: AutomationTask
  inheritedModelProfile: string
  modelProfileOptions: Array<{ id: string; label: string }>
  running: boolean
  saving: boolean
  onChange: (patch: Partial<AutomationTask>) => void
  onOpenRun: (run: AutomationRunRecord) => void
  onRemove: () => void
  onTriggersChange: (triggers: AutomationTriggerDefinition[]) => void
}

/** Pure automation-definition editor; persistence and navigation stay in AutomationsView. */
export function AutomationConfigPanel({
  activeId,
  activeRunId,
  books,
  draft,
  inheritedModelProfile,
  modelProfileOptions,
  running,
  saving,
  onChange,
  onOpenRun,
  onRemove,
  onTriggersChange,
}: AutomationConfigPanelProps) {
  const { t } = useTranslation()
  const globalTask = draft.target?.kind === 'user'

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <div className="mx-auto flex w-full min-w-0 max-w-5xl flex-col gap-5 px-4 py-5 sm:px-6">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--nova-border)] pb-4">
          <div className="min-w-0">
            <div className="truncate text-sm font-medium text-[var(--nova-text)]">{draft.name || t('automations.newTask')}</div>
            <div className="mt-1 truncate text-[11px] text-[var(--nova-text-faint)]">
              {automationTargetLabel(draft, books, t)} · {draft.enabled ? t('automations.enabled') : t('automations.disabled')}
            </div>
          </div>
          {activeId && (
            <Button
              type="button"
              size="sm"
              variant="destructive"
              onClick={onRemove}
              disabled={saving || running}
              className="nova-nav-item h-8 shrink-0 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-3"
              aria-label={t('automations.deleteTask')}
              title={t('automations.deleteTask')}
            >
              <Trash2 data-icon="inline-start" />
              {t('automations.deleteTask')}
            </Button>
          )}
        </div>

        <section className="grid gap-3 border-b border-[var(--nova-border)] pb-5 md:grid-cols-2">
          <FormField htmlFor="automation-name" label={t('automations.field.name')}>
            <Input id="automation-name" value={draft.name} onChange={(event) => onChange({ name: event.target.value })} className={controlClassName} />
          </FormField>
          <FormField label={t('automations.field.enabled')}>
            <div className="flex h-8 items-center gap-2">
              <Switch
                checked={draft.enabled}
                onCheckedChange={(enabled) => onChange({ enabled })}
                aria-label={t('automations.field.enabled')}
              />
              <span className="text-[11px] text-muted-foreground">{draft.enabled ? t('automations.enabled') : t('automations.disabled')}</span>
            </div>
          </FormField>
          <FormField label={t('automations.field.target')}>
            <Select value={automationTargetValue(draft)} disabled>
              <SelectTrigger className={controlClassName} aria-label={t('automations.field.target')}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="user">{t('automations.target.global')}</SelectItem>
                  {automationTargetOptions(books, draft).map((book) => <SelectItem key={book.path} value={`workspace:${book.path}`}>{t('automations.target.workspace', { name: book.name })}</SelectItem>)}
                </SelectGroup>
              </SelectContent>
            </Select>
          </FormField>
          <FormField label={t('automations.field.modelProfile')}>
            <Select value={draft.model_profile_id || '__inherit__'} onValueChange={(profileId) => onChange({ model_profile_id: profileId === '__inherit__' ? '' : profileId })}>
              <SelectTrigger className={controlClassName} aria-label={t('automations.field.modelProfile')}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="__inherit__">{t('automations.model.inherit', { label: inheritedModelProfile })}</SelectItem>
                  {modelProfileOptions.map((profile) => <SelectItem key={profile.id} value={profile.id}>{profile.label}</SelectItem>)}
                </SelectGroup>
              </SelectContent>
            </Select>
          </FormField>
          <div className="md:col-span-2">
            <FormField label={t('automations.field.prompt')}>
              <Textarea autoResize value={draft.prompt} onChange={(event) => onChange({ prompt: event.target.value })} aria-label={t('automations.field.prompt')} placeholder={t('automations.prompt.placeholder')} className={`${controlClassName} min-h-32 resize-y leading-5 shadow-none focus-visible:ring-0`} />
            </FormField>
          </div>
        </section>

        <section className="grid gap-3 border-b border-[var(--nova-border)] pb-5 md:grid-cols-2">
          <FormField label={t('automations.field.writeMode')}>
            <Select value={draft.write_mode} disabled={globalTask} onValueChange={(mode) => onChange(nextAutomationWriteModePatch(draft, mode as AutomationTask['write_mode']))}>
              <SelectTrigger className={controlClassName} aria-label={t('automations.field.writeMode')}><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="read_only">{t('automations.writeMode.readOnly')}</SelectItem>
                  <SelectItem value="confirm_write">{t('automations.writeMode.confirmWrite')}</SelectItem>
                  <SelectItem value="auto_write">{t('automations.writeMode.autoWrite')}</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </FormField>
          <FormField label={t('automations.field.writeScope')}>
            <Select value={draft.write_scope} disabled={globalTask || draft.write_mode === 'read_only'} onValueChange={(scope) => onChange(nextAutomationWriteScopePatch(draft, scope as AutomationTask['write_scope']))}>
              <SelectTrigger className={controlClassName} aria-label={t('automations.field.writeScope')}><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="none">{t('automations.writeScope.none')}</SelectItem>
                  <SelectItem value="lore">{t('automations.writeScope.lore')}</SelectItem>
                  <SelectItem value="file">{t('automations.writeScope.file')}</SelectItem>
                  <SelectItem value="lore_and_file">{t('automations.writeScope.loreFile')}</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </FormField>
          <FormField label={t('automations.field.outputPolicy')}>
            <Select value={draft.output_policy} disabled={globalTask} onValueChange={(policy) => onChange({ output_policy: policy as AutomationTask['output_policy'] })}>
              <SelectTrigger className={controlClassName} aria-label={t('automations.field.outputPolicy')}><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="run_record_only">{t('automations.output.record')}</SelectItem>
                  <SelectItem value="optional_file">{t('automations.output.file')}</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </FormField>
          <div className="md:col-span-2">
            <FormField htmlFor="automation-output-path" label={t('automations.field.outputPath')}>
              <Input id="automation-output-path" value={draft.output_path} disabled={globalTask} onChange={(event) => onChange({ output_path: event.target.value })} placeholder="reports/automation-review.md" className={controlClassName} />
            </FormField>
          </div>
          {globalTask && <div className="md:col-span-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('automations.target.globalHelp')}</div>}
        </section>

        <section className="flex flex-col gap-3 border-b border-[var(--nova-border)] pb-5">
          <FormSectionHeader title={t('automations.section.triggers')} />
          <TriggerEditor task={draft} onChange={onTriggersChange} />
        </section>

        <section className="flex flex-col gap-3 pb-5">
          <FormSectionHeader title={t('automations.section.runs')} />
          <AutomationRunList task={draft} activeRunId={activeRunId} onOpenRun={onOpenRun} />
        </section>
      </div>
    </div>
  )
}

function AutomationRunList({ task, activeRunId, onOpenRun }: { task: AutomationTask; activeRunId: string; onOpenRun: (run: AutomationRunRecord) => void }) {
  const { t } = useTranslation()
  const runs = task.recent_runs || []
  if (runs.length === 0) {
    return <EmptyState variant="compact" title={t('automations.runs.empty')} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-faint)]" />
  }
  return (
    <div className="flex flex-col gap-2">
      {runs.slice(0, 5).map((run) => (
        <div key={run.id} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
          <div className="flex items-center gap-2">
            <span className="font-medium">{run.status}</span>
            <span className="text-[11px] text-[var(--nova-text-faint)]">{new Date(run.started_at).toLocaleString()}</span>
            {run.output_path && <span className="ml-auto truncate text-[11px] text-[var(--nova-text-faint)]">{run.output_path}</span>}
            {run.session_id && (
              <button
                type="button"
                onClick={() => onOpenRun(run)}
                className={`nova-nav-item ml-auto rounded-[var(--nova-radius)] px-2 py-0.5 text-[11px] ${activeRunId === run.id ? 'is-active' : 'text-[var(--nova-text-muted)]'}`}
              >
                {t('automations.runs.viewTimeline')}
              </button>
            )}
          </div>
          <div className="mt-1 line-clamp-3 whitespace-pre-wrap text-[11px] leading-5 text-[var(--nova-text-muted)]">{run.error || run.summary}</div>
        </div>
      ))}
    </div>
  )
}
