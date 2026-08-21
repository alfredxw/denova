import { X, type LucideIcon } from 'lucide-react'

interface ComposerModeChipProps {
  icon: LucideIcon
  label: string
  ariaLabel: string
  disabled: boolean
  onClose: () => void
}

/** Shared active-mode indicator for mutually exclusive composer modes. */
export function ComposerModeChip({ icon: Icon, label, ariaLabel, disabled, onClose }: ComposerModeChipProps) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClose}
      className="group inline-flex h-8 shrink-0 items-center gap-1.5 rounded-full bg-transparent px-2 text-sm text-[var(--nova-text-faint)] transition-colors hover:bg-[var(--nova-active)] hover:text-[var(--nova-text)] focus-visible:bg-[var(--nova-active)] focus-visible:text-[var(--nova-text)] active:bg-[var(--nova-active)] active:text-[var(--nova-text)] disabled:pointer-events-none disabled:opacity-45"
      aria-pressed={true}
      aria-label={ariaLabel}
    >
      <span className="relative size-3.5 shrink-0" aria-hidden="true">
        <Icon
          data-slot="composer-mode-icon"
          className="absolute inset-0 size-3.5 opacity-100 transition-opacity group-hover:opacity-0 group-focus-visible:opacity-0 group-active:opacity-0"
        />
        <X
          data-slot="composer-mode-close"
          className="absolute inset-0 size-3.5 opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100 group-active:opacity-100"
        />
      </span>
      {label}
    </button>
  )
}
