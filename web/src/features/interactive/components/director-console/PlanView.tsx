import { useEffect, useState } from 'react'
import { Edit3, Eye, FileText, Loader2, RefreshCw, Save, ShieldAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { MarkdownRenderer, type MarkdownRendererComponents } from '@/components/common/MarkdownRenderer'
import { Button } from '@/components/ui/button'
import type { DirectorPlan, DirectorPlanDocs, DirectorPlanMetadata, RuleResolution, TerminalOutcome } from '../../types'
import { RuleAuditCard } from './RuleAuditCard'
import type { DirectorStatusLike } from './types'
import { directorPlanTotals, directorStatusLabel, formatBytes } from './utils'

export function PlanView({
  storyId,
  directorRevealed,
  onRevealDirector,
  directorPlan,
  draftDocs,
  onDraftDocsChange,
  directorStatus,
  directorMetadata,
  loading,
  rebuilding,
  saving,
  onSave,
  onRebuild,
  hasRuleAudit,
  ruleResolution,
  terminalOutcome,
  ruleError,
  rerolling,
  onReroll,
}: {
  storyId?: string
  directorRevealed: boolean
  onRevealDirector: () => void
  directorPlan: DirectorPlan | null
  draftDocs: DirectorPlanDocs | null
  onDraftDocsChange: (docs: DirectorPlanDocs) => void
  directorStatus?: DirectorStatusLike
  directorMetadata?: DirectorPlanMetadata
  loading: boolean
  rebuilding: boolean
  saving: boolean
  onSave: () => void
  onRebuild: () => void
  hasRuleAudit: boolean
  ruleResolution: RuleResolution | undefined
  terminalOutcome: TerminalOutcome | undefined
  ruleError: string
  rerolling: boolean
  onReroll: () => void
}) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState(false)

  useEffect(() => {
    setEditing(false)
  }, [directorPlan?.metadata?.revision, directorRevealed])

  if (!directorRevealed) {
    return (
      <div className="space-y-3">
        <PlanPublicSummary
          storyId={storyId}
          directorStatus={directorStatus}
          directorMetadata={directorMetadata}
          loading={loading}
          rebuilding={rebuilding}
          onRebuild={onRebuild}
        />
        <DirectorSpoilerGate onReveal={onRevealDirector} />
        {hasRuleAudit ? (
          <RuleAuditCard ruleResolution={ruleResolution} terminalOutcome={terminalOutcome} error={ruleError} rerolling={rerolling} onReroll={onReroll} />
        ) : null}
      </div>
    )
  }
  return (
    <div className="space-y-3">
      <section className="overflow-hidden rounded-[12px] border border-[var(--nova-border)] bg-[var(--director-panel)]">
        <div className="flex min-w-0 items-start justify-between gap-3 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-3">
          <div className="flex min-w-0 items-start gap-2.5">
            <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-[10px] border border-[var(--nova-border)] bg-[var(--director-panel)] text-[var(--director-brass)]">
              <FileText className="h-3.5 w-3.5" />
            </span>
            <div className="min-w-0">
              <h3 className="director-console__display truncate text-base font-semibold leading-5 text-[var(--nova-text)]">{t('memoryPanel.planTitle')}</h3>
              <p className="mt-1 truncate text-[9px] uppercase tracking-[0.12em] text-[var(--nova-text-faint)]">{directorStatusLabel(directorStatus, loading, t)}</p>
            </div>
          </div>
          <div className="flex shrink-0 flex-wrap justify-end gap-1">
            <Button type="button" variant="outline" size="xs" aria-label={editing ? t('memoryPanel.plan.preview') : t('memoryPanel.plan.edit')} title={editing ? t('memoryPanel.plan.preview') : t('memoryPanel.plan.edit')} className="h-7 gap-1.5 rounded-[8px] border-[var(--nova-border)] bg-[var(--director-panel)] px-2 text-[var(--nova-text-muted)] hover:border-[var(--director-brass)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" disabled={!draftDocs} onClick={() => setEditing((value) => !value)}>
              {editing ? <Eye className="h-3 w-3" /> : <Edit3 className="h-3 w-3" />}
              <span className="director-plan-action-label">{editing ? t('memoryPanel.plan.preview') : t('memoryPanel.plan.edit')}</span>
            </Button>
            {editing ? (
              <Button type="button" variant="outline" size="xs" aria-label={saving ? t('common.saving') : t('common.save')} title={saving ? t('common.saving') : t('common.save')} className="h-7 gap-1.5 rounded-[8px] border-[var(--nova-border)] bg-[var(--director-panel)] px-2 text-[var(--nova-text-muted)] hover:border-[var(--director-brass)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" disabled={!storyId || !draftDocs || !directorPlan || saving} onClick={onSave}>
                {saving ? <Loader2 className="h-3 w-3 animate-spin" /> : <Save className="h-3 w-3" />}
                <span className="director-plan-action-label">{saving ? t('common.saving') : t('common.save')}</span>
              </Button>
            ) : null}
            <Button type="button" variant="outline" size="xs" aria-label={rebuilding ? t('snapshot.director.rebuilding') : t('snapshot.director.rebuild')} title={rebuilding ? t('snapshot.director.rebuilding') : t('snapshot.director.rebuild')} className="h-7 gap-1.5 rounded-[8px] border-[var(--nova-border)] bg-[var(--director-panel)] px-2 text-[var(--nova-text-muted)] hover:border-[var(--director-brass)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" disabled={!storyId || rebuilding} onClick={onRebuild}>
              {rebuilding ? <Loader2 className="h-3 w-3 animate-spin" /> : <RefreshCw className="h-3 w-3" />}
              <span className="director-plan-action-label">{rebuilding ? t('snapshot.director.rebuilding') : t('snapshot.director.rebuild')}</span>
            </Button>
          </div>
        </div>
        <div className="p-3">
          {draftDocs ? (
            editing ? (
              <DirectorPlanTextarea label={t('snapshot.director.plan')} value={draftDocs.plan} onChange={(value) => onDraftDocsChange({ ...draftDocs, plan: value })} />
            ) : (
              <DirectorPlanMarkdown content={draftDocs.plan} />
            )
          ) : (
            <div className="flex min-h-[220px] items-center justify-center rounded-[10px] border border-dashed border-[var(--nova-border)] px-4 text-center text-xs text-[var(--nova-text-muted)]">{t('memoryPanel.directorEmpty')}</div>
          )}
        </div>
      </section>
      {hasRuleAudit ? (
        <RuleAuditCard ruleResolution={ruleResolution} terminalOutcome={terminalOutcome} error={ruleError} rerolling={rerolling} onReroll={onReroll} />
      ) : null}
    </div>
  )
}

function PlanPublicSummary({ storyId, directorStatus, directorMetadata, loading, rebuilding, onRebuild }: { storyId?: string; directorStatus?: DirectorStatusLike; directorMetadata?: DirectorPlanMetadata; loading: boolean; rebuilding: boolean; onRebuild: () => void }) {
  const { t } = useTranslation()
  const totals = directorPlanTotals(directorStatus, directorMetadata)
  return (
    <section className="overflow-hidden rounded-[12px] border border-[var(--nova-border)] bg-[var(--director-panel)]">
      <div className="flex min-w-0 items-start justify-between gap-3 px-3 py-3.5">
        <div className="flex min-w-0 items-start gap-2.5">
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--director-brass)]">
            <FileText className="h-3.5 w-3.5" />
          </span>
          <div className="min-w-0">
            <h3 className="director-console__display truncate text-base font-semibold leading-5 text-[var(--nova-text)]">{t('memoryPanel.planTitle')}</h3>
            <p className="mt-1 text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('memoryPanel.planPublicHint')}</p>
          </div>
        </div>
        <Button type="button" variant="outline" size="xs" aria-label={rebuilding ? t('snapshot.director.rebuilding') : t('snapshot.director.rebuild')} title={rebuilding ? t('snapshot.director.rebuilding') : t('snapshot.director.rebuild')} className="h-7 shrink-0 gap-1.5 rounded-[8px] border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[var(--nova-text-muted)] hover:border-[var(--director-brass)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" disabled={!storyId || rebuilding} onClick={onRebuild}>
          {rebuilding ? <Loader2 className="h-3 w-3 animate-spin" /> : <RefreshCw className="h-3 w-3" />}
          <span className="director-plan-action-label">{rebuilding ? t('snapshot.director.rebuilding') : t('snapshot.director.rebuild')}</span>
        </Button>
      </div>
      <div className="grid grid-cols-3 border-t border-[var(--nova-border)] bg-[var(--nova-surface)]">
        <PlanMetric label={t('memoryPanel.planStatus')} value={directorStatusLabel(directorStatus, loading, t)} />
        <PlanMetric label={t('snapshot.director.docs')} value={`${totals.completed}/${totals.planned}`} />
        <PlanMetric label={t('memoryPanel.run.visibleBytes')} value={formatBytes(totals.visibleBytes)} />
      </div>
    </section>
  )
}

function DirectorSpoilerGate({ onReveal }: { onReveal: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-[300px] items-center justify-center">
      <section className="relative w-full overflow-hidden rounded-[12px] border border-[var(--nova-border)] bg-[var(--director-panel)] px-5 py-7 text-center">
        <div className="absolute inset-x-0 top-0 h-px bg-[linear-gradient(90deg,transparent,var(--director-brass),transparent)]" />
        <div className="mx-auto flex h-10 w-10 items-center justify-center rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--director-brass)]">
          <ShieldAlert className="h-5 w-5" />
        </div>
        <h3 className="director-console__display mt-3 text-base font-semibold text-[var(--nova-text)]">{t('memoryPanel.directorSpoilerTitle')}</h3>
        <p className="mt-2 text-xs leading-5 text-[var(--nova-text-muted)]">{t('memoryPanel.directorSpoilerDescription')}</p>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="mt-4 gap-2 rounded-[9px] border-[var(--director-brass)] bg-[color-mix(in_srgb,var(--director-brass)_10%,var(--nova-surface))] text-[var(--nova-text)] hover:bg-[color-mix(in_srgb,var(--director-brass)_16%,var(--nova-surface))]"
          onClick={onReveal}
        >
          <Eye className="h-3.5 w-3.5" />
          {t('memoryPanel.directorReveal')}
        </Button>
      </section>
    </div>
  )
}

function DirectorPlanMarkdown({ content }: { content: string }) {
  const { t } = useTranslation()
  if (!content.trim()) {
    return (
      <div className="flex min-h-[180px] items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] px-4 text-center text-xs text-[var(--nova-text-muted)]">{t('memoryPanel.plan.empty')}</div>
    )
  }
  return (
    <div data-testid="director-plan-markdown" className="director-plan-sheet max-h-[min(62vh,640px)] overflow-y-auto rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-4 text-xs leading-5 text-[var(--nova-text)]">
      <MarkdownRenderer content={content} components={directorMarkdownComponents} />
    </div>
  )
}

const directorMarkdownComponents: MarkdownRendererComponents = {
  h1: ({ children }) => <h1 className="director-console__display mb-4 break-words text-lg font-semibold leading-7 text-[var(--nova-text)] [overflow-wrap:anywhere]">{children}</h1>,
  h2: ({ children }) => <h2 className="director-console__display mb-2 mt-5 break-words border-l-2 border-[var(--director-brass)] pl-3 text-[15px] font-semibold leading-5 text-[var(--nova-text)] [overflow-wrap:anywhere]">{children}</h2>,
  h3: ({ children }) => <h3 className="mb-1.5 mt-3 break-words text-xs font-semibold leading-5 text-[var(--nova-text)] [overflow-wrap:anywhere]">{children}</h3>,
  h4: ({ children }) => <h4 className="mb-1 mt-3 break-words text-xs font-semibold leading-5 text-[var(--nova-text-muted)] [overflow-wrap:anywhere]">{children}</h4>,
  p: ({ children }) => <p className="my-2 break-words text-xs leading-5 text-[var(--nova-text)] [overflow-wrap:anywhere]">{children}</p>,
  ul: ({ children }) => <ul className="my-2 list-disc space-y-1 pl-5 text-xs leading-5 text-[var(--nova-text)]">{children}</ul>,
  ol: ({ children }) => <ol className="my-2 list-decimal space-y-1 pl-5 text-xs leading-5 text-[var(--nova-text)]">{children}</ol>,
  li: ({ children }) => <li className="break-words pl-1 [overflow-wrap:anywhere]">{children}</li>,
  blockquote: ({ children }) => <blockquote className="my-3 border-l-2 border-[var(--nova-warning)]/70 pl-3 text-[var(--nova-text-muted)]">{children}</blockquote>,
  code: ({ children }) => <code className="rounded-[5px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1 py-0.5 font-mono text-[11px] text-[var(--nova-text)]">{children}</code>,
  pre: ({ children }) => <pre className="my-3 overflow-x-auto rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3 text-[11px] leading-5 text-[var(--nova-text)]">{children}</pre>,
  hr: () => <hr className="my-4 border-[var(--nova-border)]" />,
}

function DirectorPlanTextarea({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  return (
    <label className="block">
      <span className="mb-1 block text-[11px] font-medium text-[var(--nova-text-faint)]">{label}</span>
      <textarea
        className="min-h-[320px] w-full resize-y rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-3 font-mono text-[11px] leading-5 text-[var(--nova-text)] outline-none transition-colors focus:border-[var(--director-brass)] focus:ring-2 focus:ring-[color-mix(in_srgb,var(--director-brass)_16%,transparent)]"
        value={value}
        spellCheck={false}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  )
}

function PlanMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 border-r border-[var(--nova-border)] px-2.5 py-2 last:border-r-0">
      <div className="truncate text-[10px] font-medium text-[var(--nova-text)]" title={value}>{value}</div>
      <div className="mt-0.5 truncate text-[8px] uppercase tracking-[0.1em] text-[var(--nova-text-faint)]">{label}</div>
    </div>
  )
}
