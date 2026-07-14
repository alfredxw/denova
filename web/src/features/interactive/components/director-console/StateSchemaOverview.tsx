import { useState } from 'react'
import { AlertTriangle, CheckCircle2, Clock3, Database, ListChecks, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { retryInteractiveStateSchema, reviewInteractiveStateSchema, skipInteractiveStateSchema } from '../../api'
import type { ActorStateField, ActorStateSchemaRequirementReview, ActorStateSchemaSnapshot, StateSchemaInitializationStatus } from '../../types'

export function StateSchemaOverview({ storyId, schema, initialization, canReview = true, onRefresh }: {
  storyId?: string
  schema?: ActorStateSchemaSnapshot
  initialization?: StateSchemaInitializationStatus
  canReview?: boolean
  onRefresh?: () => void | Promise<unknown>
}) {
  const { t } = useTranslation()
  const [action, setAction] = useState<'retry' | 'review' | 'skip' | ''>('')
  const [actionError, setActionError] = useState('')

  const runAction = async (kind: 'retry' | 'review' | 'skip') => {
    if (!storyId || action) return
    setAction(kind)
    setActionError('')
    try {
      if (kind === 'retry') await retryInteractiveStateSchema(storyId)
      else if (kind === 'review') await reviewInteractiveStateSchema(storyId)
      else await skipInteractiveStateSchema(storyId)
      await onRefresh?.()
    } catch (error) {
      setActionError(error instanceof Error ? error.message : t('memoryPanel.stateSchema.actionFailed'))
    } finally {
      setAction('')
    }
  }

  const requirements = initialization?.requirements?.length
    ? initialization.requirements
    : schema?.adaptation?.requirements || []
  const reviewedLoreIDs = initialization?.reviewed_lore_ids?.length
    ? initialization.reviewed_lore_ids
    : schema?.adaptation?.reviewed_lore_ids || []
  const loreRevision = initialization?.lore_revision || schema?.adaptation?.lore_revision

  return (
    <div className="space-y-3">
      {initialization ? (
        <section className={`rounded-[10px] border px-3 py-2.5 ${statusClass(initialization.status)}`}>
          <div className="flex items-start gap-2">
            <StatusIcon status={initialization.status} />
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h3 className="text-xs font-semibold">{t('memoryPanel.stateSchema.title')}</h3>
                <span className="font-mono text-[9px] uppercase tracking-[0.1em] opacity-70">
                  {t(`memoryPanel.stateSchema.status.${initialization.status}`, { defaultValue: initialization.status })}
                </span>
              </div>
              <p className="mt-1 text-[11px] leading-4 opacity-80">{initializationDescription(initialization, t)}</p>
              {initialization.error ? <p className="mt-1.5 break-words text-[10px] leading-4">{initialization.error}</p> : null}
              {initialization.status === 'failed' ? (
                <div className="mt-2 flex flex-wrap gap-2">
                  <Button size="xs" variant="outline" disabled={!storyId || Boolean(action)} onClick={() => void runAction('retry')}>
                    <RefreshCw className={`mr-1 h-3 w-3 ${action === 'retry' ? 'animate-spin' : ''}`} />
                    {t('memoryPanel.stateSchema.retry')}
                  </Button>
                  <Button size="xs" variant="ghost" disabled={!storyId || Boolean(action)} onClick={() => void runAction('skip')}>
                    {t('memoryPanel.stateSchema.usePreset')}
                  </Button>
                </div>
              ) : null}
              {initialization.status === 'ready' ? (
                <div className="mt-2 flex flex-wrap gap-2">
                  <Button size="xs" variant="outline" disabled={!storyId || !canReview || Boolean(action)} onClick={() => void runAction('review')}>
                    <RefreshCw className={`mr-1 h-3 w-3 ${action === 'review' ? 'animate-spin' : ''}`} />
                    {t('memoryPanel.stateSchema.review')}
                  </Button>
                </div>
              ) : null}
              {initialization.status === 'ready' && !canReview ? <p className="mt-1.5 text-[10px] leading-4 opacity-70">{t('memoryPanel.stateSchema.reviewMultiBranchUnavailable')}</p> : null}
              {actionError ? <p className="mt-2 text-[10px] text-[var(--nova-danger)]">{actionError}</p> : null}
            </div>
          </div>
        </section>
      ) : null}

      {requirements.length || reviewedLoreIDs.length || loreRevision ? (
        <section className="min-w-0 rounded-[10px] border border-[var(--nova-border)] bg-[var(--director-panel)] px-3 py-2.5">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h3 className="flex items-center gap-1.5 text-xs font-semibold"><ListChecks className="h-3.5 w-3.5 text-[var(--director-brass)]" />{t('memoryPanel.stateSchema.coverageReview')}</h3>
            {initialization?.outcome ? (
              <span className="rounded bg-[var(--nova-surface-3)] px-1.5 py-0.5 text-[9px] text-[var(--nova-text-faint)]">
                {t(`memoryPanel.stateSchema.outcome.${initialization.outcome}`, { defaultValue: initialization.outcome })}
              </span>
            ) : null}
          </div>
          {loreRevision || reviewedLoreIDs.length ? (
            <div className="mt-2 space-y-1 text-[9px] leading-4 text-[var(--nova-text-faint)]">
              {loreRevision ? <p>{t('memoryPanel.stateSchema.loreRevision')} <span className="font-mono">{loreRevision}</span></p> : null}
              {reviewedLoreIDs.length ? (
                <div className="flex flex-wrap items-center gap-1">
                  <span>{t('memoryPanel.stateSchema.reviewedLore')}</span>
                  {reviewedLoreIDs.map((id) => <span key={id} className="max-w-full break-all rounded bg-[var(--nova-surface-3)] px-1.5 py-0.5 font-mono">{id}</span>)}
                </div>
              ) : null}
            </div>
          ) : null}
          {requirements.length ? (
            <ul className="mt-2 space-y-2">
              {requirements.map((requirement, index) => (
                <li key={`${requirement.source.kind}:${requirement.source.id}:${requirement.template_id}:${requirement.field_id}:${index}`} className="min-w-0 rounded-md bg-[var(--nova-surface-2)] px-2.5 py-2 text-[10px] leading-4">
                  <div className="flex flex-wrap items-center gap-1.5 text-[9px] text-[var(--nova-text-faint)]">
                    <span className="rounded bg-[var(--nova-surface-3)] px-1.5 py-0.5 font-medium text-[var(--nova-text-muted)]">{t(`memoryPanel.stateSchema.decision.${requirement.decision}`, { defaultValue: requirement.decision })}</span>
                    {requirement.evidence_kind ? <span className="rounded border border-[var(--nova-border)] px-1.5 py-0.5">{t(`memoryPanel.stateSchema.evidence.${requirement.evidence_kind}`, { defaultValue: requirement.evidence_kind })}</span> : null}
                    <span className="min-w-0 break-words">{t(`memoryPanel.stateSchema.source.${requirement.source.kind}`, { defaultValue: requirement.source.kind })} · <span className="break-all font-mono">{requirement.source.id}</span></span>
                  </div>
                  <p className="mt-1 break-words text-[var(--nova-text-muted)]">{requirement.requirement}</p>
                  {requirementTarget(requirement) ? <p className="mt-0.5 break-all font-mono text-[9px] text-[var(--nova-text-faint)]">{requirementTarget(requirement)}{requirementExpected(requirement) ? ` · ${requirementExpected(requirement)}` : ''}</p> : null}
                  {requirement.reason ? <p className="mt-0.5 break-words text-[var(--nova-text-faint)]">{requirement.reason}</p> : null}
                </li>
              ))}
            </ul>
          ) : null}
        </section>
      ) : null}

      {schema ? (
        <section className="rounded-[10px] border border-[var(--nova-border)] bg-[var(--director-panel)]">
          <div className="flex items-center justify-between gap-2 border-b border-[var(--nova-border)] px-3 py-2.5">
            <div className="flex items-center gap-2 text-xs font-semibold"><Database className="h-3.5 w-3.5 text-[var(--director-brass)]" />{t('memoryPanel.stateSchema.structure')}</div>
            <span className="font-mono text-[9px] text-[var(--nova-text-faint)]">rev {schema.revision || 1}</span>
          </div>
          <div className="divide-y divide-[var(--nova-border)]">
            {(schema.system.templates || []).map((template) => (
              <details key={template.id} className="group px-3 py-2" open={(schema.system.templates || []).length <= 3}>
                <summary className="cursor-pointer list-none text-[11px] font-medium text-[var(--nova-text-muted)]">
                  <span>{template.name || template.id}</span>
                  <span className="ml-2 font-mono text-[9px] text-[var(--nova-text-faint)]">{template.id} · {(template.fields || []).length}</span>
                </summary>
                <div className="mt-2 space-y-1.5 pl-2">
                  {(template.fields || []).filter((field) => field.visibility !== 'hidden').map((field) => (
                    <div key={field.name} className="grid grid-cols-[minmax(0,1fr)_auto] gap-2 text-[10px] leading-4">
                      <div className="min-w-0"><span className="break-words text-[var(--nova-text-muted)]">{field.name}</span>{field.description ? <span className="ml-1 text-[var(--nova-text-faint)]">— {field.description}</span> : null}</div>
                      <span className="font-mono text-[var(--nova-text-faint)]">{fieldMeta(field)}</span>
                    </div>
                  ))}
                  {(template.trait_rules || []).length ? (
                    <div className="flex flex-wrap gap-1 pt-1 text-[9px] text-[var(--nova-text-faint)]">
                      <span>{t('memoryPanel.stateSchema.traitRules', { count: template.trait_rules?.length || 0 })}</span>
                      {template.trait_rules?.map((rule) => <span key={rule.pool_id} className="rounded bg-[var(--nova-surface-3)] px-1.5 py-0.5 font-mono">{rule.pool_id} × {rule.draw_count}</span>)}
                    </div>
                  ) : null}
                </div>
              </details>
            ))}
          </div>
          {(schema.system.initial_actors || []).length ? (
            <div className="border-t border-[var(--nova-border)] px-3 py-2 text-[10px] text-[var(--nova-text-faint)]">
              {t('memoryPanel.stateSchema.initialActors', { actors: schema.system.initial_actors?.map((actor) => actor.name || actor.id).join('、') })}
            </div>
          ) : null}
        </section>
      ) : null}

      {(initialization?.changes || []).length ? (
        <section className="rounded-[10px] border border-[var(--nova-border)] bg-[var(--director-panel)] px-3 py-2.5">
          <h3 className="text-xs font-semibold">{t('memoryPanel.stateSchema.changes')}</h3>
          <ul className="mt-2 space-y-1.5">
            {initialization?.changes?.map((change, index) => (
              <li key={`${change.kind}:${change.template_id}:${change.field_id}:${change.actor_id}:${index}`} className="text-[10px] leading-4 text-[var(--nova-text-muted)]">
                <span className="mr-1 rounded bg-[var(--nova-surface-3)] px-1.5 py-0.5 font-mono text-[8px] uppercase text-[var(--nova-text-faint)]">{change.op}</span>
                {changeLabel(change)}
                {change.reason ? <span className="text-[var(--nova-text-faint)]"> — {change.reason}</span> : null}
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {(initialization?.warnings || []).length ? (
        <section className="rounded-[10px] border border-[var(--nova-warning)]/25 bg-[var(--nova-warning-bg)] px-3 py-2.5 text-[var(--nova-warning)]">
          <h3 className="flex items-center gap-1.5 text-xs font-semibold"><AlertTriangle className="h-3.5 w-3.5" />{t('memoryPanel.stateSchema.warnings')}</h3>
          <ul className="mt-1.5 list-disc space-y-1 pl-4 text-[10px] leading-4">{initialization?.warnings?.map((warning) => <li key={warning}>{warning}</li>)}</ul>
        </section>
      ) : null}
    </div>
  )
}

function StatusIcon({ status }: { status: string }) {
  if (status === 'ready' || status === 'skipped') return <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" />
  if (status === 'failed') return <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
  return <Clock3 className={`mt-0.5 h-4 w-4 shrink-0 ${status === 'running' ? 'animate-pulse' : ''}`} />
}

function statusClass(status: string) {
  if (status === 'failed') return 'border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] text-[var(--nova-danger)]'
  if (status === 'ready') return 'border-[var(--nova-success)]/25 bg-[var(--nova-success-bg)] text-[var(--nova-success)]'
  return 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]'
}

function initializationDescription(status: StateSchemaInitializationStatus, t: ReturnType<typeof useTranslation>['t']) {
  if (status.summary) return status.summary
  return t(`memoryPanel.stateSchema.description.${status.status}`, { defaultValue: status.status })
}

function changeLabel(change: NonNullable<StateSchemaInitializationStatus['changes']>[number]) {
  const source = change.field_id || change.actor_id || change.template_id || change.kind
  return change.target_id && change.target_id !== source ? `${source} → ${change.target_id}` : source
}

function requirementTarget(requirement: ActorStateSchemaRequirementReview) {
  if (!requirement.template_id || !requirement.field_id) return ''
  return `${requirement.template_id}.${requirement.field_id}`
}

function requirementExpected(requirement: ActorStateSchemaRequirementReview) {
  const range = requirement.min !== undefined || requirement.max !== undefined
    ? `${requirement.min ?? '−∞'}…${requirement.max ?? '+∞'}`
    : ''
  return [requirement.expected_type, range].filter(Boolean).join(' · ')
}

function fieldMeta(field: ActorStateField) {
  const defaultValue = field.default === undefined ? '' : ` = ${compactValue(field.default)}`
  return `${field.type}${defaultValue}${field.visibility ? ` · ${field.visibility}` : ''}`
}

function compactValue(value: unknown) {
  try {
    const serialized = JSON.stringify(value)
    if (!serialized) return String(value)
    return serialized.length > 32 ? `${serialized.slice(0, 29)}…` : serialized
  } catch {
    return String(value)
  }
}
