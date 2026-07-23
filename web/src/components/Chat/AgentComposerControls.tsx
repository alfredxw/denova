import { Check, ChevronDown, List, PenLine, Send, Square } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { AgentCommandDelivery } from '@/lib/api'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

interface AgentComposerControlsProps {
  generationActive: boolean
  delivery: AgentCommandDelivery
  onDeliveryChange?: (delivery: AgentCommandDelivery) => void
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
  delivery,
  onDeliveryChange,
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
  const deliveryLabel = t(`chat.input.delivery.${delivery}`)
  const resolvedSendLabel = sendLabel || t('chat.input.send')
  const commandControlsDisabled = disabled || activeControlsDisabled || abortPending || actionPending
  const stopControlDisabled = disabled || (stopDisabled ?? activeControlsDisabled) || abortPending || actionPending
  return (
    <div className="flex shrink-0 items-center gap-1" aria-busy={abortPending || actionPending || undefined}>
      {generationActive && onDeliveryChange ? (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              type="button"
              variant="outline"
              size="lg"
              disabled={commandControlsDisabled}
              className="max-w-28 rounded-[10px] border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-xs text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
              aria-label={t('chat.input.delivery.label', { mode: deliveryLabel })}
              title={t('chat.input.delivery.label', { mode: deliveryLabel })}
            >
              {delivery === 'follow_up' ? <List data-icon="inline-start" /> : <PenLine data-icon="inline-start" />}
              <span className="truncate">{deliveryLabel}</span>
              <ChevronDown data-icon="inline-end" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" side="top" className="w-72 border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-1.5 text-[var(--nova-text)]">
            <DropdownMenuGroup>
              {(['follow_up', 'steer'] as const).map((candidate) => (
                <DropdownMenuItem
                  key={candidate}
                  role="menuitemradio"
                  aria-checked={candidate === delivery}
                  onSelect={() => onDeliveryChange(candidate)}
                  className="cursor-pointer items-start gap-2 px-2 py-2 focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
                >
                  {candidate === 'follow_up' ? <List /> : <PenLine />}
                  <span className="min-w-0 flex-1">
                    <span className="block text-xs font-medium">{t(`chat.input.delivery.${candidate}`)}</span>
                    <span className="mt-0.5 block text-[11px] leading-4 text-[var(--nova-text-faint)]">
                      {t(`chat.input.delivery.${candidate}Description`)}
                    </span>
                  </span>
                  <Check className={cn('mt-0.5', candidate !== delivery && 'invisible')} />
                </DropdownMenuItem>
              ))}
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      ) : null}
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
