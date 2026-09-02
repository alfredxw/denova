import type { ReactNode } from 'react'
import type { TFunction } from 'i18next'
import { cn } from '@/lib/utils'
import {
  DetailPre,
  formatMaybeJSON,
  formatValue,
  inlinePreview,
  numberValue,
  parseRecord,
  recordArray,
  recordValue,
  stringArray,
  stringValue,
  type ToolDetailAdapter,
  type ToolDetailRenderProps,
} from './shared'

const OUTCOMES = ['critical_success', 'success', 'failure', 'critical_failure'] as const
const KNOWN_DIAGNOSTICS = new Set([
  'invalid_json', 'invalid_top_level', 'invalid_module', 'choice_count_mismatch', 'duplicate_choice',
  'empty_choice', 'story_context_required', 'initial_state_incomplete', 'invalid_plan_update_mode',
  'invalid_choice_count_config', 'choice_too_large', 'too_many_choices',
])

export const interactiveTurnToolDetailAdapters: Record<string, ToolDetailAdapter> = {
  prepare_interactive_turn: {
    layout: 'unified',
    render: renderPreparedTurn,
    summarize: summarizePreparedTurn,
  },
  submit_interactive_turn: {
    layout: 'unified',
    render: renderSubmittedTurn,
    summarize: summarizeSubmittedTurn,
  },
}

function summarizePreparedTurn({ input, result, t }: ToolDetailRenderProps): string {
  const response = parseRecord(result) || {}
  const rule = recordValue(input.rule)
  const label = stringValue(response.label) || stringValue(rule.label) || t('chat.tool.detail.ruleCheck')
  const outcome = stringValue(response.outcome)
  const consequence = stringValue(response.result)
  const inputDifficulty = stringValue(input.difficulty)
  const total = numberValue(response.total)
  const target = numberValue(response.target)
  if (outcome && consequence) return `${outcomeLabel(outcome, t)} · ${inlinePreview(consequence, 96)}`
  if (outcome && total !== undefined && target !== undefined) return `${outcomeLabel(outcome, t)} · ${formatNumber(total)} / ${formatNumber(target)}`
  return [label, inputDifficulty ? difficultyLabel(inputDifficulty, t) : ''].filter(Boolean).join(' · ')
}

function summarizeSubmittedTurn({ input, result, t }: ToolDetailRenderProps): string {
  const receipt = parseRecord(result)
  const stateChanges = recordArray(input.state_changes)
  const choices = stringArray(input.choices)
  const planUpdate = recordValue(input.plan_update)
  let status = t('chat.tool.detail.turnSubmission')
  if (receipt) {
    status = receipt.ready === true ? t('chat.tool.detail.turnSubmitted') : t('chat.tool.detail.notReady')
  }
  return [
    status,
    input.state_changes === undefined ? '' : t('chat.tool.detail.stateChangeCount', { count: stateChanges.length }),
    input.choices === undefined ? '' : t('chat.tool.detail.choiceCount', { count: choices.length }),
    Object.keys(planUpdate).length ? t('chat.tool.detail.planUpdated') : '',
  ].filter(Boolean).join(' · ')
}

function renderPreparedTurn({ input, result, t }: ToolDetailRenderProps): ReactNode {
  const response = parseRecord(result)
  const adjudication = recordValue(input.adjudication)
  const rule = recordValue(input.rule)
  const outcomes = recordValue(input.outcomes)
  const resolvedBonuses = response ? recordArray(response.bonus_details) : []
  const bonuses = resolvedBonuses.length ? resolvedBonuses : recordArray(input.bonuses)
  const stateRefs = recordArray(adjudication.state_refs)
  const selectedOutcome = stringValue(response?.outcome)
  const resultChanges = recordArray(response?.state_changes)
  const baseTarget = numberValue(response?.base_target)
  const formula = response ? rollFormula(response) : ''
  const resolvedDifficulty = response ? difficultySummary(response, input, t) : ''
  const stakes = stringValue(adjudication.stakes) || stringValue(response?.stakes)
  const ruleLabel = stringValue(rule.label) || stringValue(response?.label)

  return (
    <>
      <TurnDetailSection title={t('snapshot.ruleAudit.request')}>
        <p className="whitespace-pre-wrap font-medium text-[var(--nova-text)]">{stringValue(input.action)}</p>
        {stringValue(input.intent) ? <InfoRow label={t('snapshot.ruleAudit.intent')}>{stringValue(input.intent)}</InfoRow> : null}
        {stringValue(input.state) ? <InfoRow label={t('chat.tool.detail.relevantState')}>{stringValue(input.state)}</InfoRow> : null}
        {stringValue(input.challenge) ? <InfoRow label={t('chat.tool.detail.challenge')}>{stringValue(input.challenge)}</InfoRow> : null}
        {stringValue(input.cost) ? <InfoRow label={t('chat.tool.detail.cost')}>{stringValue(input.cost)}</InfoRow> : null}
      </TurnDetailSection>

      {response ? (
        <TurnDetailSection title={t('snapshot.ruleAudit.result')}>
          <div className="flex min-w-0 flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
            <strong className={cn('text-sm font-semibold', outcomeTone(selectedOutcome))}>{outcomeLabel(selectedOutcome, t)}</strong>
            {formula ? <code className="text-[11px] tabular-nums text-[var(--nova-text)]">{formula}</code> : null}
          </div>
          {stringValue(response.result) ? <p className="whitespace-pre-wrap text-[var(--nova-text)]">{stringValue(response.result)}</p> : null}
          <div className="flex flex-col gap-1">
            {resolvedDifficulty ? <InfoRow label={t('snapshot.ruleAudit.difficulty')}>{resolvedDifficulty}</InfoRow> : null}
            {baseTarget !== undefined ? <InfoRow label={t('snapshot.field.base_target')}><TechnicalValue>{formatNumber(baseTarget)}</TechnicalValue></InfoRow> : null}
            {stringValue(response.roll_mode) ? <InfoRow label={t('chat.tool.detail.rollMode')}>{rollModeLabel(stringValue(response.roll_mode), t)}</InfoRow> : null}
          </div>
        </TurnDetailSection>
      ) : null}

      <TurnDetailSection title={t('chat.tool.detail.rulingBasis')}>
        {stringValue(adjudication.reason) ? <InfoRow label={t('snapshot.ruleAudit.reason')}>{stringValue(adjudication.reason)}</InfoRow> : null}
        {stakes ? <InfoRow label={t('snapshot.ruleAudit.stakes')}>{stakes}</InfoRow> : null}
        {stringValue(adjudication.difficulty_reason) ? <InfoRow label={t('snapshot.ruleAudit.difficultyReason')}>{stringValue(adjudication.difficulty_reason)}</InfoRow> : null}
        {stringValue(adjudication.roll_mode_reason) ? <InfoRow label={t('snapshot.ruleAudit.rollModeReason')}>{stringValue(adjudication.roll_mode_reason)}</InfoRow> : null}
        {ruleLabel || stringValue(rule.template_id) ? (
          <InfoRow label={t('chat.tool.detail.rule')}>
            <span>{ruleLabel || t('chat.tool.detail.ruleCheck')}</span>
            {stringValue(rule.template_id) ? <TechnicalValue className="ml-1.5 text-[var(--nova-text-faint)]">{stringValue(rule.template_id)}</TechnicalValue> : null}
          </InfoRow>
        ) : null}
        {stringValue(rule.failure_policy) ? <InfoRow label={t('chat.tool.detail.failurePolicy')}>{failurePolicyLabel(stringValue(rule.failure_policy), t)}</InfoRow> : null}
        {stateRefs.length ? (
          <InfoRow label={t('snapshot.ruleAudit.statePaths')}>
            <div className="flex min-w-0 flex-wrap gap-x-2 gap-y-0.5">
              {stateRefs.map((stateRef, index) => (
                <TechnicalValue key={`${stringValue(stateRef.actor_id)}-${stringValue(stateRef.field_id)}-${index}`}>
                  {[stringValue(stateRef.actor_id), stringValue(stateRef.field_id)].filter(Boolean).join('.')}
                </TechnicalValue>
              ))}
            </div>
          </InfoRow>
        ) : null}
        {bonuses.length ? <InfoRow label={t('snapshot.ruleAudit.bonuses')}><BonusList values={bonuses} t={t} /></InfoRow> : null}
      </TurnDetailSection>

      {Object.keys(outcomes).length ? (
        <TurnDetailSection title={t('chat.tool.detail.presetOutcomes')}>
          <div className="flex flex-col gap-1">
            {OUTCOMES.map((outcome) => {
              const consequence = recordValue(outcomes[outcome])
              const selected = selectedOutcome === outcome
              return (
                <div
                  key={outcome}
                  className={cn(
                    'grid min-w-0 grid-cols-1 gap-x-3 gap-y-0.5 border-l-2 py-1 pl-2 sm:grid-cols-[7rem_minmax(0,1fr)]',
                    selected ? selectedOutcomeBorder(outcome) : 'border-transparent',
                  )}
                >
                  <span className={cn('font-medium', selected ? outcomeTone(outcome) : 'text-[var(--nova-text-faint)]')}>
                    {outcomeLabel(outcome, t)}{selected ? ` · ${t('chat.tool.detail.selected')}` : ''}
                  </span>
                  <span className="min-w-0 whitespace-pre-wrap text-[var(--nova-text-muted)]">{stringValue(consequence.result) || t('chat.tool.detail.none')}</span>
                </div>
              )
            })}
          </div>
        </TurnDetailSection>
      ) : null}

      {resultChanges.length ? (
        <TurnDetailSection title={t('chat.tool.detail.stateChanges')}>
          <StateChangeList values={resultChanges} t={t} />
        </TurnDetailSection>
      ) : null}
      {result.trim() && !response ? (
        <TurnDetailSection title={t('chat.tool.detail.resultDetails')} tone="warning">
          <DetailPre className="text-[var(--nova-danger)]">{formatMaybeJSON(result)}</DetailPre>
        </TurnDetailSection>
      ) : null}
    </>
  )
}

function renderSubmittedTurn({ input, result, t }: ToolDetailRenderProps): ReactNode {
  const receipt = parseRecord(result)
  const changes = recordArray(input.state_changes)
  const choices = stringArray(input.choices)
  const planUpdate = recordValue(input.plan_update)
  const hasSubmittedModules = input.state_changes !== undefined || input.choices !== undefined || Object.keys(planUpdate).length > 0

  return (
    <>
      {receipt && receipt.ready !== true ? <SubmissionIssues receipt={receipt} t={t} /> : null}
      {receipt?.ready === true && !hasSubmittedModules ? (
        <p className="font-medium text-[var(--nova-success)]">{t('chat.tool.detail.ready')}</p>
      ) : null}
      {input.state_changes !== undefined ? (
        <TurnDetailSection title={t('chat.tool.detail.stateChanges')} count={changes.length}>
          <StateChangeList values={changes} t={t} empty={t('chat.tool.detail.none')} />
        </TurnDetailSection>
      ) : null}
      {input.choices !== undefined ? (
        <TurnDetailSection title={t('chat.tool.detail.choices')} count={choices.length}>
          {choices.length ? (
            <ol className="m-0 flex list-decimal flex-col gap-1 pl-5 marker:text-[var(--nova-text-faint)]">
              {choices.map((choice, index) => <li key={`${choice}-${index}`} className="pl-1 text-[var(--nova-text)]">{choice}</li>)}
            </ol>
          ) : <span className="text-[var(--nova-text-faint)]">{t('chat.tool.detail.none')}</span>}
        </TurnDetailSection>
      ) : null}
      {Object.keys(planUpdate).length ? (
        <TurnDetailSection title={t('chat.tool.detail.planUpdate')}>
          <PlanUpdateDetail value={planUpdate} t={t} />
        </TurnDetailSection>
      ) : null}
      {result.trim() && !receipt ? (
        <TurnDetailSection title={t('chat.tool.detail.resultDetails')} tone="warning">
          <DetailPre className="text-[var(--nova-danger)]">{formatMaybeJSON(result)}</DetailPre>
        </TurnDetailSection>
      ) : null}
    </>
  )
}

function SubmissionIssues({ receipt, t }: { receipt: Record<string, unknown>; t: TFunction }) {
  const moduleStatuses = recordValue(receipt.module_status)
  const diagnostics = recordArray(receipt.diagnostics)
  const retryModules = stringArray(receipt.retry_modules)
  const missingModules = stringArray(receipt.missing_modules)
  const planDetail = recordValue(receipt.plan_update_detail)
  return (
    <TurnDetailSection title={t('chat.tool.detail.needsAttention')} tone="warning">
      {Object.entries(moduleStatuses).map(([module, status]) => (
        <InfoRow key={module} label={moduleLabel(module, t)}>{moduleStatusLabel(String(status), t)}</InfoRow>
      ))}
      {retryModules.length ? <InfoRow label={t('chat.tool.detail.retry')}>{retryModules.map(value => moduleLabel(value, t)).join(t('chat.tool.detail.listSeparator'))}</InfoRow> : null}
      {missingModules.length ? <InfoRow label={t('chat.tool.detail.missingModules')}>{missingModules.map(value => moduleLabel(value, t)).join(t('chat.tool.detail.listSeparator'))}</InfoRow> : null}
      {diagnostics.map((diagnostic, index) => (
        <div key={`${stringValue(diagnostic.code)}-${index}`} className="flex min-w-0 flex-col gap-0.5 text-[var(--nova-danger)]">
          <span>{diagnosticLabel(stringValue(diagnostic.code), t)}</span>
          {stringValue(diagnostic.path) ? <TechnicalValue className="text-[var(--nova-text-faint)]">{stringValue(diagnostic.path)}</TechnicalValue> : null}
        </div>
      ))}
      {receipt.diagnostics_truncated === true ? <p className="text-[var(--nova-warning)]">{t('chat.tool.detail.diagnosticsTruncated')}</p> : null}
      {Object.keys(planDetail).length ? <PlanReceiptDetail value={planDetail} t={t} /> : null}
    </TurnDetailSection>
  )
}

function PlanUpdateDetail({ value, t }: { value: Record<string, unknown>; t: TFunction }) {
  const mode = stringValue(value.mode)
  const markdown = stringValue(value.markdown)
  const sections = recordArray(value.sections)
  return (
    <div className="flex min-w-0 flex-col gap-2">
      {mode ? <InfoRow label={t('chat.tool.detail.updateMode')}>{planModeLabel(mode, t)}</InfoRow> : null}
      {markdown ? <p className="whitespace-pre-wrap text-[var(--nova-text)]">{markdown}</p> : null}
      {sections.map((section, index) => (
        <div key={`${stringValue(section.heading)}-${index}`} className="flex min-w-0 flex-col gap-0.5">
          <TechnicalValue className="font-medium text-[var(--nova-text)]">{stringValue(section.heading) || `#${index + 1}`}</TechnicalValue>
          <p className="whitespace-pre-wrap">{stringValue(section.markdown)}</p>
        </div>
      ))}
      {!mode && !markdown && !sections.length ? <DetailPre>{formatValue(value)}</DetailPre> : null}
    </div>
  )
}

function PlanReceiptDetail({ value, t }: { value: Record<string, unknown>; t: TFunction }) {
  const rows: Array<[string, string[]]> = [
    [t('chat.tool.detail.acceptedSections'), stringArray(value.accepted_sections)],
    [t('chat.tool.detail.rejectedSections'), stringArray(value.rejected_sections)],
    [t('chat.tool.detail.retrySections'), stringArray(value.retry_sections)],
  ]
  return (
    <div className="flex min-w-0 flex-col gap-1 border-l-2 border-[var(--nova-border)] pl-2">
      {rows.map(([label, values]) => values.length ? <InfoRow key={label} label={label}>{values.join(t('chat.tool.detail.listSeparator'))}</InfoRow> : null)}
      {value.retained_draft === true ? <span>{t('chat.tool.detail.planDraftRetained')}</span> : null}
    </div>
  )
}

function StateChangeList({ values, t, empty }: { values: Record<string, unknown>[]; t: TFunction; empty?: string }) {
  if (!values.length) return empty ? <span className="text-[var(--nova-text-faint)]">{empty}</span> : null
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      {values.map((change, index) => {
        const op = stringValue(change.op) || (change.change !== undefined ? 'delta' : 'replace')
        const target = [stringValue(change.actor_id), stringValue(change.field_id), ...stringArray(change.subpath)].filter(Boolean).join('.')
          || stringValue(change.name)
        let value = ''
        if (change.value !== undefined) {
          value = op === 'delta' ? signedValue(change.value) : formatValue(change.value)
        } else if (change.change !== undefined) {
          value = signedValue(change.change)
        }
        return (
          <div key={`${target}-${index}`} className="grid min-w-0 grid-cols-[4.5rem_minmax(0,1fr)] gap-x-2 gap-y-0.5">
            <span className="text-[11px] text-[var(--nova-text-faint)]">{stateOperationLabel(op, t)}</span>
            <div className="min-w-0">
              <DetailPre><TechnicalValue className="text-[var(--nova-text)]">{target || `#${index + 1}`}</TechnicalValue>{value ? ` = ${value}` : ''}</DetailPre>
              {stringValue(change.template_id) ? <div>{t('chat.tool.detail.template')}: <TechnicalValue>{stringValue(change.template_id)}</TechnicalValue></div> : null}
              {stringValue(change.role) ? <div>{t('chat.tool.detail.role')}: {stringValue(change.role)}</div> : null}
              {stringValue(change.description) ? <p className="whitespace-pre-wrap">{stringValue(change.description)}</p> : null}
              {change.initial_state !== undefined ? <DetailPre className="text-[var(--nova-text-muted)]">{formatValue(change.initial_state)}</DetailPre> : null}
              {stringValue(change.reason) ? <p className="whitespace-pre-wrap text-[var(--nova-text-faint)]">{stringValue(change.reason)}</p> : null}
            </div>
          </div>
        )
      })}
    </div>
  )
}

function BonusList({ values, t }: { values: Record<string, unknown>[]; t: TFunction }) {
  return (
    <div className="flex min-w-0 flex-col gap-1">
      {values.map((bonus, index) => {
        const source = [stringValue(bonus.actor_id), stringValue(bonus.field_id)].filter(Boolean).join('.')
        const isStoryRollModifier = stringValue(bonus.kind) === 'story'
          && stringValue(bonus.reason) === 'Story-wide roll modifier.'
        const reason = isStoryRollModifier ? t('chat.tool.detail.storyRollModifier') : stringValue(bonus.reason)
        return (
          <div key={`${stringValue(bonus.reason)}-${index}`} className="grid min-w-0 grid-cols-[2.5rem_minmax(0,1fr)] gap-x-2">
            <TechnicalValue className="text-right text-[var(--nova-text)]">{signedValue(bonus.value)}</TechnicalValue>
            <span className="min-w-0 break-words">
              {reason}
              {source ? <TechnicalValue className="ml-1.5 text-[var(--nova-text-faint)]">{source}</TechnicalValue> : null}
            </span>
          </div>
        )
      })}
    </div>
  )
}

function TurnDetailSection({ title, count, tone = 'normal', children }: { title: string; count?: number; tone?: 'normal' | 'warning'; children: ReactNode }) {
  return (
    <section className="flex min-w-0 flex-col gap-1.5">
      <h4 className={cn('flex items-baseline gap-1.5 text-[11px] font-medium tracking-wide', tone === 'warning' ? 'text-[var(--nova-warning)]' : 'text-[var(--nova-text-faint)]')}>
        <span>{title}</span>
        {count !== undefined ? <span className="font-mono font-normal tabular-nums">{count}</span> : null}
      </h4>
      <div className="flex min-w-0 flex-col gap-1.5">{children}</div>
    </section>
  )
}

function InfoRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid min-w-0 grid-cols-1 gap-x-3 gap-y-0.5 sm:grid-cols-[7rem_minmax(0,1fr)]">
      <span className="text-[11px] text-[var(--nova-text-faint)]">{label}</span>
      <div className="min-w-0 whitespace-pre-wrap">{children}</div>
    </div>
  )
}

function TechnicalValue({ children, className }: { children: ReactNode; className?: string }) {
  return <code className={cn('font-mono text-[11px] [overflow-wrap:anywhere]', className)}>{children}</code>
}

function rollFormula(response: Record<string, unknown>) {
  const rolls = Array.isArray(response.rolls) ? response.rolls.map(value => String(value)) : []
  const total = numberValue(response.total)
  const target = numberValue(response.target)
  if (!rolls.length || total === undefined || target === undefined) return ''
  const dice = stringValue(response.dice) || '1d20'
  const kept = numberValue(response.kept_roll)
  const selection = rolls.length > 1 && kept !== undefined ? ` → ${formatNumber(kept)}` : ''
  const bonus = numberValue(response.bonus_total)
  return `${dice} [${rolls.join(', ')}]${selection}${bonus === undefined ? '' : ` ${signedValue(bonus)}`} = ${formatNumber(total)} / ${formatNumber(target)}`
}

function difficultySummary(response: Record<string, unknown>, input: Record<string, unknown>, t: TFunction) {
  const requested = stringValue(response.requested_difficulty) || stringValue(input.difficulty)
  const effective = stringValue(response.difficulty) || requested
  if (!effective) return ''
  if (!requested || requested === effective) return difficultyLabel(effective, t)
  const shift = numberValue(response.difficulty_shift)
  return `${difficultyLabel(requested, t)} → ${difficultyLabel(effective, t)}${shift ? ` · ${t('chat.tool.detail.difficultyShift', { value: signedValue(shift) })}` : ''}`
}

function outcomeLabel(value: string, t: TFunction) {
  return localizedEnum(value, 'outcome', t)
}

function difficultyLabel(value: string, t: TFunction) {
  return localizedEnum(value, 'difficulty', t)
}

function rollModeLabel(value: string, t: TFunction) {
  return localizedEnum(value, 'rollModeValue', t)
}

function failurePolicyLabel(value: string, t: TFunction) {
  return localizedEnum(value, 'failurePolicyValue', t)
}

function stateOperationLabel(value: string, t: TFunction) {
  return localizedEnum(value, 'stateOperation', t)
}

function moduleLabel(value: string, t: TFunction) {
  return localizedEnum(value, 'module', t)
}

function moduleStatusLabel(value: string, t: TFunction) {
  return localizedEnum(value, 'moduleStatus', t)
}

function planModeLabel(value: string, t: TFunction) {
  return localizedEnum(value, 'planMode', t)
}

function diagnosticLabel(value: string, t: TFunction) {
  return KNOWN_DIAGNOSTICS.has(value) ? t(`chat.tool.detail.diagnostic.${value}`) : t('chat.tool.detail.diagnostic.generic')
}

function localizedEnum(value: string, group: string, t: TFunction) {
  return value ? t(`chat.tool.detail.${group}.${value}`, { defaultValue: value }) : ''
}

function outcomeTone(value: string) {
  if (value === 'success' || value === 'critical_success') return 'text-[var(--nova-success)]'
  if (value === 'failure' || value === 'critical_failure') return 'text-[var(--nova-danger)]'
  return 'text-[var(--nova-text)]'
}

function selectedOutcomeBorder(value: string) {
  return value === 'success' || value === 'critical_success'
    ? 'border-[var(--nova-success)]'
    : 'border-[var(--nova-danger)]'
}

function signedValue(value: unknown) {
  const number = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(number)) return String(value ?? '')
  return number > 0 ? `+${formatNumber(number)}` : formatNumber(number)
}

function formatNumber(value: number) {
  return Number.isInteger(value) ? String(value) : String(Number(value.toFixed(2)))
}
