import { ChevronDown, type LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'

interface AgentConfigurationDisclosureProps {
  id: string
  icon: LucideIcon
  title: string
  summary: string
  defaultOpen?: boolean
  children: ReactNode
}

export function AgentConfigurationDisclosure({
  id,
  icon: Icon,
  title,
  summary,
  defaultOpen = false,
  children,
}: AgentConfigurationDisclosureProps): ReactNode {
  return (
    <Collapsible
      defaultOpen={defaultOpen}
      data-agent-configuration-section={id}
      className="overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]"
    >
      <CollapsibleTrigger asChild>
        <button
          type="button"
          aria-label={title}
          className="group/configuration flex w-full items-center gap-3 px-3 py-3 text-left outline-none transition-colors hover:bg-[var(--nova-hover)] focus-visible:ring-2 focus-visible:ring-ring/50 motion-reduce:transition-none"
        >
          <Icon className="size-4 shrink-0 text-[var(--nova-text-muted)]" />
          <span className="min-w-0 flex-1">
            <span className="block text-xs font-semibold text-[var(--nova-text)]">{title}</span>
            <span className="mt-0.5 block truncate text-[11px] text-[var(--nova-text-faint)]">{summary}</span>
          </span>
          <ChevronDown
            aria-hidden="true"
            className="size-4 shrink-0 text-[var(--nova-text-faint)] transition-transform group-data-[state=open]/configuration:rotate-180 motion-reduce:transition-none"
          />
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent className="flex flex-col gap-5 border-t border-[var(--nova-border)] bg-[var(--nova-surface)] p-4 [&>section:last-child]:border-b-0 [&>section:last-child]:pb-0">
        {children}
      </CollapsibleContent>
    </Collapsible>
  )
}
