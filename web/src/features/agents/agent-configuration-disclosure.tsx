import { ChevronDown, type LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'

interface AgentConfigurationDisclosureProps {
  id: string
  icon: LucideIcon
  title: string
  summary: string
  defaultOpen?: boolean
  headingLevel?: 2 | 3
  children: ReactNode
}

export function AgentConfigurationDisclosure({
  id,
  icon: Icon,
  title,
  summary,
  defaultOpen = false,
  headingLevel = 2,
  children,
}: AgentConfigurationDisclosureProps): ReactNode {
  const Heading = headingLevel === 2 ? 'h2' : 'h3'
  return (
    <Collapsible
      defaultOpen={defaultOpen}
      data-agent-configuration-section={id}
      className="min-w-0 border-b border-[var(--nova-border)] pb-4 last:border-b-0"
    >
      <Heading>
      <CollapsibleTrigger asChild>
        <button
          type="button"
          aria-label={title}
          className="group/configuration flex min-h-11 w-full items-center gap-3 rounded-[var(--nova-radius)] py-2 text-left outline-none transition-colors hover:bg-[var(--nova-hover)] focus-visible:ring-2 focus-visible:ring-ring/50 motion-reduce:transition-none"
        >
          <Icon className="size-4 shrink-0 text-[var(--nova-text-muted)]" />
          <span className="min-w-0 flex-1">
            <span className="block text-sm font-semibold text-[var(--nova-text)]">{title}</span>
            <span className="mt-1 block text-xs font-normal leading-relaxed text-[var(--nova-text-muted)]">{summary}</span>
          </span>
          <ChevronDown
            aria-hidden="true"
            className="size-4 shrink-0 text-[var(--nova-text-faint)] transition-transform group-data-[state=open]/configuration:rotate-180 motion-reduce:transition-none"
          />
        </button>
      </CollapsibleTrigger>
      </Heading>
      <CollapsibleContent className="flex min-w-0 flex-col gap-6 pt-3 [&>section:last-child]:border-b-0 [&>section:last-child]:pb-0">
        {children}
      </CollapsibleContent>
    </Collapsible>
  )
}
