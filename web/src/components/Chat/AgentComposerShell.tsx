import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

interface AgentComposerShellProps {
  references?: ReactNode
  input: ReactNode
  toolbarStart?: ReactNode
  toolbarEnd?: ReactNode
  submitControl?: ReactNode
  className?: string
  bodyClassName?: string
  toolbarClassName?: string
}

/** Shared shell for Agent message composers; slots keep feature-specific controls private to each caller. */
export function AgentComposerShell({
  references,
  input,
  toolbarStart,
  toolbarEnd,
  submitControl,
  className,
  bodyClassName,
  toolbarClassName,
}: AgentComposerShellProps) {
  return (
    <div className={cn('nova-agent-composer', className)}>
      {references ? <div className="nova-agent-composer-references">{references}</div> : null}
      <div className={cn('nova-agent-composer-toolbar', toolbarClassName)} data-slot="agent-composer-layout">
        <div className="nova-agent-composer-toolbar-start" data-slot="agent-composer-start">
          {toolbarStart}
        </div>
        <div className={cn('nova-agent-composer-body', bodyClassName)} data-slot="agent-composer-input">
          {input}
        </div>
        <div className="nova-agent-composer-toolbar-end" data-slot="agent-composer-end">
          {toolbarEnd}
          {submitControl}
        </div>
      </div>
    </div>
  )
}
