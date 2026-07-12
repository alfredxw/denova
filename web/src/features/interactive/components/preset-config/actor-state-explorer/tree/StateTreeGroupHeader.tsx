import { ChevronDown, ChevronRight, Plus } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useTranslation } from 'react-i18next'
import { novaEase } from '@/features/motion/motion-tokens'
import { Button } from '@/components/ui/button'

interface StateTreeGroupHeaderProps {
  nodeId: string
  label: string
  badge?: string
  expanded: boolean
  onToggle: () => void
  onAdd?: () => void
  addLabel?: string
  children?: React.ReactNode
  indentLevel?: number
}

export function StateTreeGroupHeader({
  nodeId,
  label,
  badge,
  expanded,
  onToggle,
  onAdd,
  addLabel,
  children,
  indentLevel = 0,
}: StateTreeGroupHeaderProps) {
  const { t } = useTranslation()
  const paddingLeft = 2
  const childrenId = `${nodeId}-children`

  return (
    <div
      role="treeitem"
      aria-label={label}
      aria-expanded={expanded}
      aria-level={indentLevel + 1}
      className="min-w-0 max-w-full overflow-hidden"
    >
      <div
        className="group flex min-h-8 w-full min-w-0 max-w-full items-center gap-1 overflow-hidden rounded-[8px] pr-1.5 transition-colors duration-150 hover:bg-[var(--nova-hover)] focus-within:bg-[var(--nova-hover)]"
        style={{ paddingLeft }}
      >
        <button
          type="button"
          className="flex size-6 shrink-0 items-center justify-center rounded-[6px] text-[var(--nova-text-faint)] transition-colors hover:bg-[var(--nova-surface)] hover:text-[var(--nova-text)] focus-visible:text-[var(--nova-text)]"
          onClick={onToggle}
          aria-label={expanded ? t('settingPanel.actorState.explorer.collapse') : t('settingPanel.actorState.explorer.expand')}
          aria-expanded={expanded}
          aria-controls={childrenId}
        >
          <AnimatePresence mode="wait" initial={false}>
            {expanded ? (
              <motion.span
                key="expanded"
                initial={{ rotate: -90, opacity: 0 }}
                animate={{ rotate: 0, opacity: 1 }}
                exit={{ rotate: -90, opacity: 0 }}
                transition={{ duration: 0.15, ease: novaEase }}
              >
                <ChevronDown className="h-3.5 w-3.5" />
              </motion.span>
            ) : (
              <motion.span
                key="collapsed"
                initial={{ rotate: 90, opacity: 0 }}
                animate={{ rotate: 0, opacity: 1 }}
                exit={{ rotate: 90, opacity: 0 }}
                transition={{ duration: 0.15, ease: novaEase }}
              >
                <ChevronRight className="h-3.5 w-3.5" />
              </motion.span>
            )}
          </AnimatePresence>
        </button>
        <span className="min-w-0 flex-1 truncate py-1 text-[11px] font-semibold text-[var(--nova-text-faint)]">
          {label}
        </span>
        {badge ? (
          <span className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] px-1.5 py-0.5 text-[10px] leading-none text-[var(--nova-text-faint)]">
            {badge}
          </span>
        ) : null}
        {onAdd ? (
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            className="ml-1 size-6 shrink-0 rounded-[6px] text-[var(--nova-text-faint)] opacity-0 transition-opacity duration-150 hover:bg-[var(--nova-surface)] hover:text-[var(--nova-text)] group-hover:opacity-100 group-focus-within:opacity-100 focus-visible:opacity-100 [@media(pointer:coarse)]:opacity-100"
            onClick={(e) => {
              e.stopPropagation()
              onAdd()
            }}
            aria-label={addLabel || t('settingPanel.actorState.explorer.addChild')}
            title={addLabel || t('settingPanel.actorState.explorer.addChild')}
          >
            <Plus className="h-3.5 w-3.5" />
          </Button>
        ) : null}
      </div>
      <AnimatePresence initial={false}>
        {expanded && children ? (
          <motion.div
            id={childrenId}
            role="group"
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.2, ease: novaEase }}
            className="min-w-0 max-w-full overflow-hidden"
          >
            {children}
          </motion.div>
        ) : null}
      </AnimatePresence>
    </div>
  )
}
