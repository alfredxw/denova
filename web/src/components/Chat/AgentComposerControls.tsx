import { Send, Square } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'

interface AgentComposerControlsProps {
  generationActive: boolean
  /** Selects Send while a run is active; otherwise the single action becomes Stop. */
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

/** Single operation-aware action shared by writing and game composers. */
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
  const stopMode = generationActive && !hasSendableContent && Boolean(onStop)
  const actionLabel = stopMode ? t('chat.input.stop') : resolvedSendLabel
  return (
    <div className="flex shrink-0 items-center" aria-busy={abortPending || actionPending || undefined}>
      <Button
        type="button"
        data-action={stopMode ? 'stop' : 'send'}
        onClick={stopMode ? onStop : onSend}
        disabled={stopMode ? stopControlDisabled : commandControlsDisabled || sendDisabled}
        variant={stopMode ? 'destructive' : 'default'}
        size="icon-lg"
        className={stopMode
          ? 'nova-agent-composer-stop rounded-[10px]'
          : 'nova-agent-composer-submit rounded-[10px] bg-[var(--nova-active)] text-[var(--nova-text)] shadow-[inset_0_1px_0_rgba(255,255,255,0.12)] hover:bg-[var(--nova-hover)] disabled:bg-[var(--nova-active)]'}
        aria-label={actionLabel}
      >
        {stopMode ? <Square className="fill-current" /> : sendIcon || <Send />}
      </Button>
    </div>
  )
}
