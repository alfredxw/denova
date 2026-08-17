import type { ReactElement } from 'react'
import { Bot, Folder } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card'
import { cn } from '@/lib/utils'
import type { AgentChatProject } from './api'

interface AgentChatProjectDetailsCardProps {
  project: AgentChatProject
  active: boolean
  manualSorting: boolean
  children: ReactElement
}

/** Shows complete Project identity beside the navigation without covering neighboring rows. */
export function AgentChatProjectDetailsCard({
  project,
  active,
  manualSorting,
  children,
}: AgentChatProjectDetailsCardProps) {
  const { t } = useTranslation()
  const name = project.name || project.path
  const Icon = project.type === 'general' ? Bot : Folder

  return (
    <HoverCard openDelay={800} closeDelay={150}>
      <HoverCardTrigger asChild>{children}</HoverCardTrigger>
      <HoverCardContent
        data-slot="agent-chat-project-details"
        side="right"
        align="start"
        sideOffset={8}
        collisionPadding={12}
        className="w-60 rounded-xl border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3 text-[var(--nova-text)] shadow-[var(--nova-shadow)]"
      >
        <div className="flex min-w-0 items-center gap-2">
          <Icon
            aria-hidden="true"
            className={cn('size-4 shrink-0', active ? 'text-[var(--nova-accent)]' : 'text-[var(--nova-text-muted)]')}
          />
          <span className="min-w-0 break-words text-xs font-medium leading-4">{name}</span>
        </div>
        <code className="mt-2 block break-all rounded-md bg-[var(--nova-surface-2)] px-2 py-1.5 text-[10px] leading-4 text-[var(--nova-text-muted)]">
          {project.path}
        </code>
        {manualSorting ? (
          <p className="mt-2 text-[10px] leading-4 text-[var(--nova-text-faint)]">
            {t('agentChat.sidebar.longPressToReorder')}
          </p>
        ) : null}
      </HoverCardContent>
    </HoverCard>
  )
}
