import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Progress } from '@/components/ui/progress'
import { cn } from '@/lib/utils'
import type { ClassifiedStateChange } from './changes'
import type { LedgerFieldItem } from './model'
import { humanizeStateKey } from './model'

/** Long text beyond this length is clamped with an inline expand toggle. */
const BLOCK_CLAMP_LENGTH = 300
const OBJECT_ENTRY_LIMIT = 20
const OBJECT_ARRAY_LIMIT = 8
const MAX_OBJECT_DEPTH = 3

/**
 * LedgerFieldView renders one state field with the renderer resolved by
 * field-layout. All renderers share the same label typography and the
 * left change accent so a group grid stays visually aligned.
 */
export function LedgerFieldView({ item }: { item: LedgerFieldItem }) {
  const { label, value, renderer, change } = item
  return (
    <section
      aria-label={label}
      data-ledger-field
      data-renderer={renderer}
      data-change-tone={change ? change.tone : undefined}
      className="story-state-ledger__field"
      title={change?.reason}
    >
      {renderer === 'stat' ? (
        <StatFieldBody item={item} />
      ) : (
        <>
          <div className="mb-0.5 flex min-w-0 items-baseline justify-between gap-2">
            <FieldLabel label={label} />
            <FieldChangeChip change={change} />
          </div>
          {renderer === 'inline' ? <InlineFieldBody value={value} /> : null}
          {renderer === 'block' ? <BlockFieldBody value={value} /> : null}
          {renderer === 'list' ? <ListFieldBody value={value} /> : null}
          {renderer === 'object' ? <ObjectFieldBody value={value} /> : null}
        </>
      )}
    </section>
  )
}

function FieldLabel({ label }: { label: string }) {
  return <h4 className="story-state-ledger__field-label" title={label}>{label}</h4>
}

function StatFieldBody({ item }: { item: LedgerFieldItem }) {
  const { t } = useTranslation()
  const field = item.field
  const value = typeof item.value === 'number' ? item.value : 0
  const min = field?.min ?? 0
  const max = field?.max ?? 100
  const progress = Math.min(100, Math.max(0, ((value - min) / (max - min)) * 100))
  const valueLabel = `${formatLedgerNumber(value)} / ${formatLedgerNumber(max)}`
  return (
    <div>
      <div className="mb-1 flex min-w-0 items-baseline justify-between gap-2">
        <FieldLabel label={item.label} />
        <span className="flex shrink-0 items-baseline gap-1.5">
          <FieldChangeChip change={item.change} />
          <span className="font-mono text-[11px] font-semibold tabular-nums text-[var(--nova-text)]">{valueLabel}</span>
        </span>
      </div>
      <Progress
        value={progress}
        aria-label={t('storyStage.state.metricProgress', {
          label: item.label,
          value: formatLedgerNumber(value),
          min: formatLedgerNumber(min),
          max: formatLedgerNumber(max),
        })}
        aria-valuetext={valueLabel}
        className="story-state-ledger__metric-progress h-1.5"
      />
    </div>
  )
}

function InlineFieldBody({ value }: { value: unknown }) {
  const { t } = useTranslation()
  if (value === null || value === undefined || value === '') {
    return <span className="text-xs text-[var(--nova-text-faint)]">—</span>
  }
  if (typeof value === 'boolean') {
    return (
      <span className="inline-flex items-center gap-1.5 text-xs text-[var(--nova-text)]">
        <span className={cn('size-1.5 rounded-full', value ? 'bg-[var(--nova-success)]' : 'bg-[var(--nova-text-faint)]')} />
        {value ? t('directorPanel.stateValue.yes') : t('directorPanel.stateValue.no')}
      </span>
    )
  }
  if (typeof value === 'number') {
    return <span className="font-mono text-xs tabular-nums text-[var(--nova-text)]">{formatLedgerNumber(value)}</span>
  }
  return <span className="break-words text-xs leading-5 text-[var(--nova-text)] [overflow-wrap:anywhere]">{String(value)}</span>
}

function BlockFieldBody({ value }: { value: unknown }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const text = typeof value === 'string' ? value : value == null ? '' : String(value)
  if (!text.trim()) return <span className="text-xs text-[var(--nova-text-faint)]">—</span>
  const clamped = text.length > BLOCK_CLAMP_LENGTH && !expanded
  return (
    <div>
      <p className={cn('story-state-ledger__block-text', clamped && 'story-state-ledger__block-text--clamped')}>{text}</p>
      {text.length > BLOCK_CLAMP_LENGTH ? (
        <button
          type="button"
          className="mt-0.5 text-[11px] text-[var(--nova-text-faint)] transition-colors hover:text-[var(--nova-text)]"
          onClick={() => setExpanded((next) => !next)}
        >
          {expanded ? t('storyStage.state.collapseField') : t('storyStage.state.expandField')}
        </button>
      ) : null}
    </div>
  )
}

function ListFieldBody({ value }: { value: unknown }) {
  const { t } = useTranslation()
  if (!Array.isArray(value) || value.length === 0) {
    return <span className="text-xs text-[var(--nova-text-faint)]">{t('directorPanel.stateValue.empty')}</span>
  }
  return (
    <ul className="story-state-ledger__item-list">
      {value.map((item, index) => (
        <li key={index} className="story-state-ledger__item-row">
          {item === null ? '—' : typeof item === 'object' ? JSON.stringify(item) : String(item)}
        </li>
      ))}
    </ul>
  )
}

function ObjectFieldBody({ value }: { value: unknown }) {
  const { t } = useTranslation()
  if (Array.isArray(value)) {
    const items = value.filter((item) => isRecord(item))
    if (items.length === 0) return <span className="text-xs text-[var(--nova-text-faint)]">{t('directorPanel.stateValue.empty')}</span>
    return (
      <div className="flex flex-col gap-2">
        {items.slice(0, 8).map((item, index) => (
          <ObjectEntryList key={index} value={item} />
        ))}
      </div>
    )
  }
  if (!isRecord(value) || Object.keys(value).length === 0) {
    return <span className="text-xs text-[var(--nova-text-faint)]">{t('directorPanel.stateValue.empty')}</span>
  }
  return <ObjectEntryList value={value} />
}

function ObjectEntryList({ value }: { value: Record<string, unknown> }) {
  return <NestedObjectList value={value} depth={0} />
}

function ObjectValue({ value, depth = 0 }: { value: unknown; depth?: number }) {
  if (value === null || value === undefined || value === '') return <span className="text-[var(--nova-text-faint)]">—</span>
  if (typeof value === 'boolean') return <span>{value ? '✓' : '—'}</span>
  if (typeof value === 'number') return <span className="font-mono tabular-nums">{formatLedgerNumber(value)}</span>
  if (typeof value === 'string') return <span className="break-words [overflow-wrap:anywhere]">{value}</span>
  if (Array.isArray(value)) {
    if (value.length === 0) return <span className="text-[var(--nova-text-faint)]">—</span>
    return (
      <ul className="story-state-ledger__object-value-list">
        {value.slice(0, OBJECT_ARRAY_LIMIT).map((item, index) => (
          <li key={index} className="story-state-ledger__object-value-item">
            <ObjectValue value={item} depth={depth + 1} />
          </li>
        ))}
      </ul>
    )
  }
  if (isRecord(value)) {
    if (depth >= MAX_OBJECT_DEPTH) return <span className="text-[var(--nova-text-faint)]">…</span>
    return <NestedObjectList value={value} depth={depth} />
  }
  return <span>{String(value)}</span>
}

function NestedObjectList({ value, depth }: { value: Record<string, unknown>; depth: number }) {
  const entries = Object.entries(value).filter(([, item]) => item !== undefined)
  if (entries.length === 0) return <span className="text-[var(--nova-text-faint)]">—</span>
  return (
    <ul className={cn('story-state-ledger__nested-object-list', depth > 0 && 'story-state-ledger__nested-object-list--nested')} data-depth={depth}>
      {entries.slice(0, OBJECT_ENTRY_LIMIT).map(([key, item]) => {
        const structured = Array.isArray(item) || isRecord(item)
        const label = humanizeStateKey(key)
        return (
          <li key={key} className={cn('story-state-ledger__nested-object-item', structured && 'story-state-ledger__nested-object-item--branch')}>
            <span className="story-state-ledger__nested-object-key" title={label}>{label}:</span>
            <div className="story-state-ledger__nested-object-value">
              <ObjectValue value={item} depth={depth + 1} />
            </div>
          </li>
        )
      })}
    </ul>
  )
}

/** FieldChangeChip renders the compact per-field turn-change marker. */
function FieldChangeChip({ change }: { change: ClassifiedStateChange | null }) {
  const { t } = useTranslation()
  if (!change) return null
  if (change.kind === 'delta' && change.delta !== null) {
    return (
      <span className={cn('story-state-ledger__change-chip', `story-state-ledger__change-chip--${change.tone}`)}>
        {change.delta >= 0 ? `+${formatLedgerNumber(change.delta)}` : formatLedgerNumber(change.delta)}
      </span>
    )
  }
  if (change.kind === 'added' || change.kind === 'removed') {
    const sign = change.kind === 'added' ? '+' : '−'
    const text = change.text ? truncateEnd(change.text, 16) : t('storyStage.state.change.oneItem')
    return (
      <span className={cn('story-state-ledger__change-chip', `story-state-ledger__change-chip--${change.tone}`)}>
        {sign}{text}
      </span>
    )
  }
  return (
    <span className="story-state-ledger__change-chip story-state-ledger__change-chip--neutral">
      {change.kind === 'cleared' ? t('storyStage.state.change.cleared') : t('storyStage.state.change.updated')}
    </span>
  )
}

function formatLedgerNumber(value: number) {
  return Number.isInteger(value) ? String(value) : String(Number(value.toFixed(2)))
}

function truncateEnd(text: string, max: number) {
  return text.length > max ? `${text.slice(0, max)}…` : text
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}
