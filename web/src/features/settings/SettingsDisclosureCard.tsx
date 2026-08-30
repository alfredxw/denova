import type { ReactNode } from 'react'
import { ChevronDown } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { cn } from '@/lib/utils'

type SettingsDisclosureLevel = 'connection' | 'model'

interface SettingsDisclosureCardProps {
  level: SettingsDisclosureLevel
  badge: string
  title: string
  subtitle: string
  defaultOpen: boolean
  actions?: ReactNode
  children: ReactNode
}

export function SettingsDisclosureCard({ level, badge, title, subtitle, defaultOpen, actions, children }: SettingsDisclosureCardProps) {
  const isConnection = level === 'connection'

  return (
    <Collapsible
      defaultOpen={defaultOpen}
      data-settings-level={level}
      className={cn(
        'overflow-hidden border bg-[var(--nova-surface-2)]',
        isConnection
          ? 'rounded-[var(--nova-radius)] border-[var(--nova-border)]'
          : 'rounded-md border-[var(--nova-border-soft)]',
      )}
    >
      <div className="flex min-w-0 items-stretch">
        <CollapsibleTrigger asChild>
          <button
            type="button"
            className={cn(
              'group/disclosure-trigger flex min-w-0 flex-1 items-center text-left outline-none transition-colors hover:bg-[var(--nova-hover)] focus-visible:ring-2 focus-visible:ring-ring/50',
              isConnection ? 'gap-2.5 rounded-l-[var(--nova-radius)] px-3 py-2.5' : 'gap-2 rounded-l-md px-2.5 py-2',
            )}
          >
            <Badge variant={isConnection ? 'secondary' : 'outline'} className="shrink-0">{badge}</Badge>
            <span className="min-w-0 flex-1">
              <span className={cn('block truncate text-xs text-[var(--nova-text)]', isConnection ? 'font-semibold' : 'font-medium')}>{title}</span>
              <span className="block truncate text-[11px] text-[var(--nova-text-faint)]">{subtitle}</span>
            </span>
            <ChevronDown className="size-4 shrink-0 text-[var(--nova-text-faint)] transition-transform group-data-[state=open]/disclosure-trigger:rotate-180" aria-hidden="true" />
          </button>
        </CollapsibleTrigger>
        {actions && (
          <div className={cn('flex shrink-0 items-center border-l px-1.5', isConnection ? 'border-[var(--nova-border)]' : 'border-[var(--nova-border-soft)]')}>
            {actions}
          </div>
        )}
      </div>
      <CollapsibleContent className={cn('border-t bg-[var(--nova-surface)]', isConnection ? 'border-[var(--nova-border)]' : 'border-[var(--nova-border-soft)]')}>
        {children}
      </CollapsibleContent>
    </Collapsible>
  )
}
