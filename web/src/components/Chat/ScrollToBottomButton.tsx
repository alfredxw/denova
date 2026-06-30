import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'

interface ScrollToBottomButtonProps {
  visible: boolean
  onClick: () => void
  bottomOffsetPx?: number
  rightOffsetPx?: number
}

export function ScrollToBottomButton({ visible, onClick, bottomOffsetPx = 24, rightOffsetPx = 24 }: ScrollToBottomButtonProps) {
  const { t } = useTranslation()
  if (!visible) return null

  return (
    <button
      type="button"
      data-nova-scroll-to-bottom
      onPointerDown={(event) => {
        if (event.button !== 0) return
        event.preventDefault()
        onClick()
      }}
      onClick={(event) => {
        if (event.detail === 0) onClick()
      }}
      aria-label={t('chat.action.scrollToBottom')}
      title={t('chat.action.scrollToBottom')}
      className="absolute z-30 flex h-8 w-8 items-center justify-center rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)]/85 text-[var(--nova-text-faint)] opacity-80 shadow-[0_10px_24px_rgba(0,0,0,0.16)] backdrop-blur-xl transition hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] hover:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--nova-ring)]"
      style={{ bottom: bottomOffsetPx, right: rightOffsetPx }}
    >
      <ChevronDown className="h-3.5 w-3.5" />
    </button>
  )
}
