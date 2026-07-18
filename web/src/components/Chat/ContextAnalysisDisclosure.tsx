import { useState, type ReactNode } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { cn } from '@/lib/utils'

interface ContextAnalysisDisclosureProps {
  title: ReactNode
  meta?: ReactNode
  size?: ReactNode
  action?: ReactNode
  error?: ReactNode
  children: ReactNode
  defaultOpen?: boolean
  variant?: 'card' | 'inline'
  contentClassName?: string
}

/** Shared expandable frame for context groups, standalone parts and inline parts. */
export function ContextAnalysisDisclosure({
  title,
  meta,
  size,
  action,
  error,
  children,
  defaultOpen = false,
  variant = 'card',
  contentClassName,
}: ContextAnalysisDisclosureProps) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className={cn(variant === 'card' && 'rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]')}
    >
      <div className={cn('flex items-center gap-2', variant === 'card' ? 'px-3 py-2' : '')}>
        <CollapsibleTrigger asChild>
          <button
            type="button"
            className={cn(
              'flex min-w-0 flex-1 items-center gap-2 text-left',
              variant === 'inline' && 'rounded-[6px] px-2 py-1.5 hover:bg-[var(--nova-hover)]',
            )}
          >
            {open ? <ChevronDown className="size-3.5 shrink-0 text-muted-foreground" /> : <ChevronRight className="size-3.5 shrink-0 text-muted-foreground" />}
            <span className="min-w-0 flex-1">
              <span className="block truncate text-[11px] font-medium text-foreground">{title}</span>
              {meta ? <span className="block truncate text-[10px] text-muted-foreground">{meta}</span> : null}
            </span>
            {variant === 'inline' ? size : null}
          </button>
        </CollapsibleTrigger>
        {action}
        {variant === 'card' ? size : null}
      </div>
      {error ? <div role="alert" className="border-t border-border px-3 py-2 text-[11px] text-destructive">{error}</div> : null}
      <CollapsibleContent className={cn(variant === 'card' && 'border-t border-border', contentClassName)}>
        {children}
      </CollapsibleContent>
    </Collapsible>
  )
}
