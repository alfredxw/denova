import { useState } from 'react'
import { ChevronDown, Map } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ThemedMarkdownRenderer } from '@/components/common/MarkdownRenderer'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { cn } from '@/lib/utils'
import type { BranchPlan } from '../../types'

interface BranchPlanSummaryProps {
  plan?: BranchPlan
  planningEnabled: boolean
}

/** Compact plan projection for the overview. The whole card toggles its details. */
export function BranchPlanSummary({ plan, planningEnabled }: BranchPlanSummaryProps) {
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState(false)
  const markdown = plan?.markdown.trim() || ''
  const updatedAt = formatUpdatedAt(plan?.updated_at, i18n.language)
  const preview = markdown ? markdown.replace(/[#*_`>\[\]()\-]+/g, ' ').replace(/\s+/g, ' ').trim() : t('directorPanel.plan.empty')

  return (
    <Collapsible open={open} onOpenChange={setOpen} className="overflow-hidden rounded-xl border border-[var(--nova-border)] bg-[var(--director-panel)]">
      <CollapsibleTrigger asChild>
        <button type="button" className="flex w-full min-w-0 items-center gap-3 px-3 py-3 text-left transition-colors hover:bg-[var(--nova-hover)]">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--director-brass)]"><Map className="size-4" /></span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-xs font-semibold text-[var(--nova-text)]">{t('directorPanel.plan.title')}</span>
            <span className="mt-0.5 block truncate text-[10px] text-[var(--nova-text-faint)]">{t(planningEnabled ? 'directorPanel.planning.enabled' : 'directorPanel.planning.disabled')} · {preview}</span>
          </span>
          <ChevronDown className={cn('size-4 shrink-0 text-[var(--nova-text-faint)] transition-transform', open && 'rotate-180')} />
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="border-t border-[var(--nova-border)] px-3 py-3">
          {markdown ? (
            <>
              <ThemedMarkdownRenderer content={markdown} className="text-xs leading-6 text-[var(--nova-text-muted)]" />
              {updatedAt ? <p className="mt-3 border-t border-[var(--nova-border-soft)] pt-2 text-[9px] text-[var(--nova-text-faint)]">{t('directorPanel.plan.updatedAt', { time: updatedAt })}</p> : null}
            </>
          ) : (
            <p className="py-3 text-center text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('directorPanel.plan.emptyHint')}</p>
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

function formatUpdatedAt(value: string | undefined, locale: string): string {
  if (!value) return ''
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? '' : parsed.toLocaleString(locale)
}
