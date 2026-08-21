import { MessageSquareText, PenLine } from 'lucide-react'
import { LayoutGroup, motion } from 'motion/react'
import { useTranslation } from 'react-i18next'

import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { novaSpring } from '@/features/motion/motion-tokens'

interface WorkbenchModeSwitchProps {
  navigationMode: 'ide' | 'interactive'
  collapsed?: boolean
  compact?: boolean
  onSwitch: (mode: 'ide' | 'interactive') => void
}

/** Explicitly switches creation modes without coupling shared navigation to mode changes. */
export function WorkbenchModeSwitch({
  navigationMode,
  collapsed = false,
  compact = false,
  onSwitch,
}: WorkbenchModeSwitchProps) {
  const { t } = useTranslation()

  if (collapsed) {
    return (
      <div role="group" className="flex flex-col items-center gap-1" aria-label={t('workbench.modeSwitch')}>
        <TooltipIconButton
          label={t('workbench.mode.ideButton')}
          tooltipSide="right"
          size="icon-sm"
          aria-pressed={navigationMode === 'ide'}
          data-onboarding-anchor="mode-ide"
          className={`size-8 text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] ${navigationMode === 'ide' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : ''}`}
          onClick={() => onSwitch('ide')}
        >
          <PenLine className="size-4" />
        </TooltipIconButton>
        <TooltipIconButton
          label={t('workbench.mode.interactiveButton')}
          tooltipSide="right"
          size="icon-sm"
          aria-pressed={navigationMode === 'interactive'}
          data-onboarding-anchor="mode-interactive"
          className={`size-8 text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] ${navigationMode === 'interactive' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : ''}`}
          onClick={() => onSwitch('interactive')}
        >
          <MessageSquareText className="size-4" />
        </TooltipIconButton>
      </div>
    )
  }

  const layoutID = compact ? 'workbench-mobile-mode-active' : 'workbench-sidebar-mode-active'
  return (
    <LayoutGroup id={compact ? 'workbench-mobile-mode-switch' : 'workbench-sidebar-mode-switch'}>
      <div
        role="group"
        className={`flex h-8 items-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0.5 ${compact ? 'shrink-0' : 'w-full'}`}
        aria-label={t('workbench.modeSwitch')}
      >
        {(['ide', 'interactive'] as const).map((item) => {
          const selected = navigationMode === item
          return (
            <button
              key={item}
              type="button"
              aria-pressed={selected}
              onClick={() => onSwitch(item)}
              data-onboarding-anchor={`mode-${item}`}
              className={`relative min-w-0 flex-1 overflow-hidden rounded-[6px] px-2 py-1 text-[11px] transition-colors ${selected ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
            >
              {selected ? <motion.span layoutId={layoutID} className="absolute inset-0 rounded-[6px] bg-[var(--nova-active)]" transition={novaSpring} /> : null}
              <span className="relative z-10 block truncate">{t(item === 'ide' ? 'workbench.mode.ideButton' : 'workbench.mode.interactiveButton')}</span>
            </button>
          )
        })}
      </div>
    </LayoutGroup>
  )
}
