import { X } from 'lucide-react'

interface ComposerModeChipProps {
  label: string
  ariaLabel: string
  disabled: boolean
  onClose: () => void
}

/** Shared active-mode indicator for mutually exclusive composer modes. */
export function ComposerModeChip({ label, ariaLabel, disabled, onClose }: ComposerModeChipProps) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClose}
      className="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-full bg-[var(--nova-active)] px-2 text-sm text-[var(--nova-text)] transition-colors hover:bg-[var(--nova-hover)] disabled:opacity-45"
      aria-pressed={true}
      aria-label={ariaLabel}
    >
      <X className="h-3.5 w-3.5" />
      {label}
    </button>
  )
}
