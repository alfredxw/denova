import { Send, Square } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'

interface AgentComposerControlsProps {
  generationActive: boolean
  /** Keeps Send beside Stop when the user has drafted a follow-up during an active run. */
  hasSendableContent: boolean
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

/** Operation-aware actions shared by writing and game composers. */
export function AgentComposerControls({
  generationActive,
  hasSendableContent,
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
  const showStop = generationActive && Boolean(onStop)
  const showSend = !showStop || hasSendableContent

  return (
    <div className="flex shrink-0 items-center gap-2" aria-busy={abortPending || actionPending || undefined}>
      {showStop ? (
        <Button
          type="button"
          data-action="stop"
          onClick={onStop}
          disabled={stopControlDisabled}
          variant="destructive"
          size="icon-lg"
          className="nova-agent-composer-stop rounded-[10px]"
          aria-label={t('chat.input.stop')}
        >
          <Square className="fill-current" />
        </Button>
      ) : null}
      {showSend ? (
        <Button
          type="button"
          data-action="send"
          onClick={onSend}
          disabled={commandControlsDisabled || sendDisabled}
          variant="default"
          size="icon-lg"
          className="nova-agent-composer-submit rounded-[10px] bg-[var(--nova-active)] text-[var(--nova-text)] shadow-[inset_0_1px_0_rgba(255,255,255,0.12)] hover:bg-[var(--nova-hover)] disabled:bg-[var(--nova-active)]"
          aria-label={resolvedSendLabel}
        >
          {sendIcon || <Send />}
        </Button>
      ) : null}
    </div>
  )
}
