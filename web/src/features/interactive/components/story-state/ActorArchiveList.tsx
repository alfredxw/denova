import { Archive, ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { cn } from '@/lib/utils'
import type { ActorArchiveEntry } from './model'

export function ActorArchiveList({ entries, variant = 'stage' }: { entries: ActorArchiveEntry[]; variant?: 'stage' | 'director' }) {
  const { t } = useTranslation()
  if (entries.length === 0) return null
  const title = t('storyStage.state.archive.title', { count: entries.length })

  return (
    <Collapsible className={cn('group/archive min-w-0', variant === 'stage' ? 'border-t border-[var(--nova-border-soft)]' : 'mt-4')}>
      <CollapsibleTrigger asChild>
        <button
          type="button"
          aria-label={title}
          className={cn(
            'flex w-full min-w-0 items-center gap-2 text-left text-[var(--nova-text-muted)] transition-colors hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]',
            variant === 'stage' ? 'px-3 py-2' : 'rounded-[10px] border border-[var(--nova-border)] bg-[var(--director-panel)] px-3 py-2.5',
          )}
        >
          <Archive aria-hidden="true" className="size-3.5 shrink-0" />
          <span className="min-w-0 flex-1 truncate text-[11px] font-medium">{title}</span>
          <span className="hidden truncate text-[10px] text-[var(--nova-text-faint)] sm:block">{t('storyStage.state.archive.hint')}</span>
          <ChevronDown aria-hidden="true" className="size-3.5 shrink-0 transition-transform group-data-[state=open]/archive:rotate-180" />
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className={cn('grid grid-cols-1 gap-2', variant === 'stage' ? 'px-3 pb-3' : 'pt-2')}>
          {entries.map((entry) => (
            <article key={entry.actorId} aria-label={entry.name} className="min-w-0 rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2.5">
              <div className="flex min-w-0 items-baseline justify-between gap-2">
                <h4 className="min-w-0 truncate text-xs font-medium text-[var(--nova-text)]">{entry.name}</h4>
                <code className="shrink-0 text-[9px] text-[var(--nova-text-faint)]">{entry.actorId}</code>
              </div>
              {entry.reason ? <p className="mt-1 text-[10px] leading-4 text-[var(--nova-text-muted)]">{entry.reason}</p> : null}
              {entry.sourceTurnId ? (
                <p className="mt-1 truncate text-[9px] text-[var(--nova-text-faint)]">
                  {t('storyStage.state.archive.source')}: <code>{entry.sourceTurnId}</code>
                </p>
              ) : null}
            </article>
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}
