import { Send, Square } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'

interface AgentComposerControlsProps {
  generationActive: boolean
  onStop?: () => void
  onSend: () => void
  sendDisabled: boolean
  disabled: boolean
  abortPending?: boolean
  actionPending?: boolean
  activeControlsDisabled?: boolean
  stopDisabled?: boolean
  sendLabel?: string
  sendIcon?: ReactNode
}

/** Operation-aware controls shared by the writing composer transport seam. */
export function AgentComposerControls({
  generationActive,
  onStop,
  onSend,
  sendDisabled,
  disabled,
  abortPending = false,
  actionPending = false,
  activeControlsDisabled = false,
  stopDisabled,
  sendLabel,
  sendIcon,
}: AgentComposerControlsProps) {
  const { t } = useTranslation()
  const resolvedSendLabel = sendLabel || t('chat.input.send')
  const commandControlsDisabled = disabled || activeControlsDisabled || abortPending || actionPending
  const stopControlDisabled = disabled || (stopDisabled ?? activeControlsDisabled) || abortPending || actionPending
  return (
    <div className="flex shrink-0 items-center gap-1" aria-busy={abortPending || actionPending || undefined}>
      {generationActive && onStop ? (
        <Button
          type="button"
          onClick={onStop}
          disabled={stopControlDisabled}
          variant="destructive"
          size="icon-lg"
          className="nova-agent-composer-stop rounded-[10px]"
          aria-label={t('chat.input.stop')}
          title={t('chat.input.stop')}
        >
          <Square className="fill-current" />
        </Button>
      ) : null}
      <Button
        type="button"
        onClick={onSend}
        disabled={commandControlsDisabled || sendDisabled}
        size="icon-lg"
        className="nova-agent-composer-submit rounded-[10px] bg-[var(--nova-active)] text-[var(--nova-text)] shadow-[inset_0_1px_0_rgba(255,255,255,0.12)] hover:bg-[var(--nova-hover)] disabled:bg-[var(--nova-active)]"
        aria-label={resolvedSendLabel}
        title={resolvedSendLabel}
      >
        {sendIcon || <Send />}
      </Button>
    </div>
  )
}
