import { Sparkle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { classifyStateChange, type ClassifiedStateChange } from './changes'
import { changeFieldLabel, type ActorStateEntry, type StoryStateChange } from './model'
import type { ActorStateSchemaSnapshot } from '../../types'

const MAX_SUMMARY_ITEMS = 4

interface SummaryItem {
  id: string
  label: string
  change: ClassifiedStateChange
}

/**
 * ChangesSummary is the single-row digest of the current turn's state delta.
 * It replaces the old per-field "updated this turn" notes; field-level detail
 * remains available through each field's change chip and reason tooltip.
 */
export function ChangesSummary({ changes, actors, schema }: { changes: StoryStateChange[]; actors: ActorStateEntry[]; schema?: ActorStateSchemaSnapshot }) {
  const { t } = useTranslation()
  if (changes.length === 0) return null

  const items: SummaryItem[] = changes.map((change) => ({
    id: change.id,
    label: changeFieldLabel(change, actors, schema),
    // Delta detection is safe to enable globally: it only fires for numeric
    // inc/decrement values, which non-numeric fields never produce.
    change: classifyStateChange(change, true),
  }))
  const visible = items.slice(0, MAX_SUMMARY_ITEMS)
  const hiddenCount = items.length - visible.length

  return (
    <div className="story-state-ledger__summary" aria-label={t('storyStage.state.changesTitle', { count: changes.length })}>
      <span className="story-state-ledger__summary-title">
        <Sparkle aria-hidden="true" className="size-3" />
        {t('storyStage.state.changesTitle', { count: changes.length })}
      </span>
      <span className="min-w-0 truncate">
        {visible.map((item, index) => (
          <span key={item.id} title={item.change.reason}>
            {index > 0 ? <span className="text-[var(--nova-text-faint)]"> · </span> : null}
            <span className="text-[var(--nova-text-muted)]">{item.label}</span>{' '}
            <SummaryItemValue change={item.change} />
          </span>
        ))}
        {hiddenCount > 0 ? (
          <span className="text-[var(--nova-text-faint)]"> · {t('storyStage.state.changesMore', { count: hiddenCount })}</span>
        ) : null}
      </span>
    </div>
  )
}

function SummaryItemValue({ change }: { change: ClassifiedStateChange }) {
  const { t } = useTranslation()
  if (change.kind === 'delta' && change.delta !== null) {
    return (
      <span className={cn('font-mono tabular-nums', change.tone === 'positive' ? 'text-[var(--story-state-positive)]' : 'text-[var(--story-state-negative)]')}>
        {change.delta >= 0 ? `+${change.delta}` : change.delta}
      </span>
    )
  }
  if (change.kind === 'added') {
    return <span className="text-[var(--story-state-positive)]">{change.text ? `+${truncateEnd(change.text, 10)}` : t('storyStage.state.change.added')}</span>
  }
  if (change.kind === 'removed') {
    return <span className="text-[var(--story-state-negative)]">{change.text ? `−${truncateEnd(change.text, 10)}` : t('storyStage.state.change.removed')}</span>
  }
  return (
    <span className="text-[var(--nova-text-faint)]">
      {change.kind === 'cleared' ? t('storyStage.state.change.cleared') : t('storyStage.state.change.updated')}
    </span>
  )
}

function truncateEnd(text: string, max: number) {
  return text.length > max ? `${text.slice(0, max)}…` : text
}
