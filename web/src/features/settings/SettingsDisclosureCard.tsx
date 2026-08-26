import type { ReactNode } from 'react'
import { ChevronDown } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { cn } from '@/lib/utils'

export function SettingsDisclosureCard({ badge, title, subtitle, defaultOpen, actions, children, className }: {
  badge: string
  title: string
  subtitle: string
  defaultOpen: boolean
  actions?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <Collapsible defaultOpen={defaultOpen} className={cn('group/disclosure rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]', className)}>
      <div className="flex min-w-0 items-stretch">
        <CollapsibleTrigger asChild>
          <button type="button" className="flex min-w-0 flex-1 items-center gap-2 rounded-l-[var(--nova-radius)] px-2.5 py-2 text-left outline-none transition-colors hover:bg-[var(--nova-hover)] focus-visible:ring-2 focus-visible:ring-ring/50">
            <Badge variant="outline" className="shrink-0">{badge}</Badge>
            <span className="min-w-0 flex-1">
              <span className="block truncate text-xs font-medium text-[var(--nova-text)]">{title}</span>
              <span className="block truncate text-[11px] text-[var(--nova-text-faint)]">{subtitle}</span>
            </span>
            <ChevronDown className="size-4 shrink-0 text-[var(--nova-text-faint)] transition-transform group-data-[state=open]/disclosure:rotate-180" aria-hidden="true" />
          </button>
        </CollapsibleTrigger>
        {actions && <div className="flex shrink-0 items-center border-l border-[var(--nova-border)] px-1.5">{actions}</div>}
      </div>
      <CollapsibleContent className="border-t border-[var(--nova-border)]">
        {children}
      </CollapsibleContent>
    </Collapsible>
  )
}
