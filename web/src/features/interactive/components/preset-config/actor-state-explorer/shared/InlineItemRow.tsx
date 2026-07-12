import { GripVertical, Trash2 } from 'lucide-react'
import { motion } from 'motion/react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { novaEase } from '@/features/motion/motion-tokens'

export interface InlineItemRowAction {
  icon: React.ReactNode
  label: string
  onClick: () => void
  danger?: boolean
}

interface InlineItemRowProps {
  selected?: boolean
  onClick?: () => void
  onRemove?: () => void
  onDragHandleRef?: (el: HTMLElement | null) => void
  dragHandleProps?: Record<string, unknown>
  isDragging?: boolean
  className?: string
  children: React.ReactNode
  actions?: InlineItemRowAction[]
}

export function InlineItemRow({
  selected = false,
  onClick,
  onRemove,
  onDragHandleRef,
  dragHandleProps,
  isDragging = false,
  className,
  children,
  actions = [],
}: InlineItemRowProps) {
  const { t } = useTranslation()
  return (
    <motion.div
      layout
      initial={{ opacity: 0, y: -4 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: -4 }}
      transition={{ duration: 0.15, ease: novaEase }}
      className={cn(
        'group relative flex min-h-10 cursor-pointer items-center gap-1.5 rounded-[9px] border px-2 py-1.5 transition-colors',
        selected
          ? 'border-[var(--nova-border)] bg-[var(--nova-active)]'
          : 'border-transparent bg-transparent hover:bg-[var(--nova-hover)]',
        isDragging && 'opacity-50',
        className,
      )}
      onClick={onClick}
    >
      <button
        type="button"
        ref={onDragHandleRef as unknown as React.Ref<HTMLButtonElement>}
        className="flex size-5 shrink-0 cursor-grab items-center justify-center rounded text-[var(--nova-text-faint)] opacity-35 transition-opacity hover:bg-[var(--nova-hover)] group-hover:opacity-100 group-focus-within:opacity-100 focus-visible:opacity-100 active:cursor-grabbing [@media(pointer:coarse)]:opacity-100"
        aria-label={t('settingPanel.actorState.explorer.drag')}
        {...(dragHandleProps ?? {})}
      >
        <GripVertical className="h-3 w-3" />
      </button>

      <div className="min-w-0 flex-1">{children}</div>

      <div className="flex shrink-0 items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100 [@media(pointer:coarse)]:opacity-100">
        {actions.map((action, i) => (
          <button
            key={i}
            type="button"
            className={cn(
              'flex size-8 items-center justify-center rounded-full transition-colors hover:bg-[var(--nova-hover)]',
              action.danger ? 'text-[var(--nova-danger)] hover:bg-[var(--nova-danger-bg)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]',
            )}
            onClick={(e) => {
              e.stopPropagation()
              action.onClick()
            }}
            aria-label={action.label}
            title={action.label}
          >
            {action.icon}
          </button>
        ))}
        {onRemove ? (
          <button
            type="button"
            className="flex size-8 items-center justify-center rounded-full text-[var(--nova-text-faint)] transition-colors hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)]"
            onClick={(e) => {
              e.stopPropagation()
              onRemove()
            }}
            aria-label={t('common.delete')}
            title={t('common.delete')}
          >
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        ) : null}
      </div>
    </motion.div>
  )
}
