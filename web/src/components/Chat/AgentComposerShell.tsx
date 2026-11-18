import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { PromptInputBody, PromptInputTools } from '@/components/ai-elements/prompt-input'

interface AgentComposerShellProps {
  draftStatus?: 'restored' | 'saveFailed'
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
  draftStatus,
  references,
  input,
  toolbarStart,
  toolbarEnd,
  submitControl,
  className,
  bodyClassName,
  toolbarClassName,
}: AgentComposerShellProps) {
  const { t } = useTranslation()
  return (
    <PromptInputBody className={cn('nova-agent-composer', className)}>
      {references ? <div className="nova-agent-composer-references">{references}</div> : null}
      <div className={cn('nova-agent-composer-toolbar', toolbarClassName)} data-slot="agent-composer-layout">
        <PromptInputTools className="nova-agent-composer-toolbar-start" data-slot="agent-composer-start">
          {toolbarStart}
        </PromptInputTools>
        <div className={cn('nova-agent-composer-body', bodyClassName)} data-slot="agent-composer-input">
          {input}
          {draftStatus && <div role="status" className="px-1 pb-1 text-xs text-[var(--nova-text-muted)]">
            {t(draftStatus === 'restored' ? 'chat.input.draftRestored' : 'chat.input.draftSaveFailed')}
          </div>}
        </div>
        <PromptInputTools className="nova-agent-composer-toolbar-end" data-slot="agent-composer-end">
          {toolbarEnd}
          {submitControl}
        </PromptInputTools>
      </div>
    </PromptInputBody>
  )
}
