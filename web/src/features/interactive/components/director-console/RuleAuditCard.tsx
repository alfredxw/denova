import { Activity, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import type { RuleResolution, TerminalOutcome } from '../../types'
import { InfoLine, MemoryChip } from './shared'
import { ruleOutcomeClass } from './utils'

export function RuleAuditCard({ ruleResolution, terminalOutcome, error, rerolling, onReroll }: { ruleResolution?: RuleResolution; terminalOutcome?: TerminalOutcome; error?: string; rerolling?: boolean; onReroll: () => void }) {
  const { t } = useTranslation()
  const ruleRequest = ruleResolution?.request
  const ruleResult = ruleResolution?.result
  const terminalCandidate = ruleResolution?.terminal_candidate
  return (
    <section className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
      <div className="mb-2 flex items-center justify-between gap-2 text-xs font-semibold text-[var(--nova-text)]">
        <div className="flex min-w-0 items-center gap-2">
          <Activity className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
          <span className="truncate">{t('snapshot.ruleAudit.title')}</span>
        </div>
        {ruleResolution?.id ? (
          <TooltipIconButton label={rerolling ? t('snapshot.ruleAudit.rerolling') : t('snapshot.ruleAudit.reroll')} className="border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:opacity-45" variant="ghost" size="icon-xs" onClick={onReroll} disabled={rerolling}>
            <RefreshCw className={`h-3.5 w-3.5 ${rerolling ? 'animate-spin' : ''}`} />
          </TooltipIconButton>
        ) : null}
      </div>
      {error ? <div className="mb-2 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-2 py-1.5 text-xs text-[var(--nova-danger)]">{error}</div> : null}
      <div className="flex flex-wrap gap-1.5">
        <MemoryChip>{ruleRequest?.intent || t('snapshot.noRecord')}</MemoryChip>
        <MemoryChip>{`${t('snapshot.ruleAudit.difficulty')}: ${ruleRequest?.difficulty || t('snapshot.noRecord')}`}</MemoryChip>
        <MemoryChip>{`${t('snapshot.ruleAudit.outcome')}: ${ruleResult?.outcome || t('snapshot.noRecord')}`}</MemoryChip>
      </div>
      {ruleRequest?.challenge || ruleRequest?.cost || ruleRequest?.state ? (
        <div className="mt-2 space-y-1 text-xs leading-5 text-[var(--nova-text-muted)]">
          {ruleRequest.challenge ? <InfoLine label={t('snapshot.field.challenge')} value={ruleRequest.challenge} /> : null}
          {ruleRequest.cost ? <InfoLine label={t('snapshot.field.cost')} value={ruleRequest.cost} /> : null}
          {ruleRequest.state ? <InfoLine label={t('snapshot.field.state')} value={ruleRequest.state} /> : null}
        </div>
      ) : null}
      {ruleResult ? (
        <div className="mt-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5 text-xs">
          <div className="flex items-center justify-between gap-2">
            <span className="min-w-0 truncate text-[var(--nova-text)]">{ruleResult.label || ruleRequest?.challenge || t('snapshot.ruleAudit.result')}</span>
            <span className={ruleOutcomeClass(ruleResult.outcome)}>{ruleResult.outcome}</span>
          </div>
          <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">
            {[ruleResult.dice, ruleResult.roll_mode, ruleResult.rolls?.length ? `${t('snapshot.field.rolls')}: ${ruleResult.rolls.join(', ')}` : '', Number.isFinite(ruleResult.kept_roll) ? `${t('snapshot.field.kept_roll')}: ${ruleResult.kept_roll}` : '', Number.isFinite(ruleResult.bonus_total) ? `${t('snapshot.field.bonus_total')}: ${ruleResult.bonus_total}` : '', Number.isFinite(ruleResult.total) ? `${t('snapshot.field.total')}: ${ruleResult.total}` : ''].filter(Boolean).join(' · ')}
          </div>
          {ruleResult.result ? <div className="mt-1 text-[var(--nova-text-muted)]">{ruleResult.result}</div> : null}
        </div>
      ) : null}
      {terminalCandidate || terminalOutcome ? (
        <div className="mt-2 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-2 py-1.5 text-xs text-[var(--nova-danger)]">
          {terminalOutcome?.reason || terminalCandidate?.reason || terminalOutcome?.type || terminalCandidate?.type}
        </div>
      ) : null}
    </section>
  )
}
