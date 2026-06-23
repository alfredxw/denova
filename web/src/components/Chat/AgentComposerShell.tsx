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
      <div className={cn('nova-agent-composer-toolbar', toolbarClassName)}>
        <div className="nova-agent-composer-toolbar-start">
          {toolbarStart}
        </div>
        <div className={cn('nova-agent-composer-body', bodyClassName)}>
          {input}
        </div>
        <div className="nova-agent-composer-toolbar-end">
          {toolbarEnd}
          {submitControl}
        </div>
      </div>
    </div>
  )
}
