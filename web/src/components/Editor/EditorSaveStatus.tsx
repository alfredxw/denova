import { Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'

export type SaveStatus = 'dirty' | 'auto-saving' | 'auto-saved' | 'manual-saving' | 'manual-saved' | 'error'

const SAVE_STATUS_META: Record<SaveStatus, { labelKey: string; ariaLabelKey: string; className: string; dotClassName?: string; subtle?: boolean }> = {
  dirty: {
    labelKey: 'editor.status.dirty',
    ariaLabelKey: 'editor.status.dirtyAria',
    className: 'text-[var(--nova-text-faint)]',
    dotClassName: 'bg-[var(--nova-text-faint)] opacity-60',
    subtle: true,
  },
  'auto-saving': {
    labelKey: 'editor.status.autoSaving',
    ariaLabelKey: 'editor.status.autoSavingAria',
    className: 'text-[var(--nova-text-faint)]',
    dotClassName: 'animate-pulse bg-[var(--nova-text-muted)] opacity-70',
    subtle: true,
  },
  'auto-saved': {
    labelKey: 'editor.status.autoSaved',
    ariaLabelKey: 'editor.status.autoSavedAria',
    className: 'text-[var(--nova-text-faint)]',
    subtle: true,
  },
  'manual-saving': {
    labelKey: 'editor.status.manualSaving',
    ariaLabelKey: 'editor.status.manualSavingAria',
    className: 'text-[var(--nova-text-muted)]',
  },
  'manual-saved': {
    labelKey: 'editor.status.manualSaved',
    ariaLabelKey: 'editor.status.manualSavedAria',
    className: 'text-[var(--nova-accent-green)]',
  },
  error: {
    labelKey: 'editor.status.error',
    ariaLabelKey: 'editor.status.errorAria',
    className: 'text-[var(--nova-danger)]',
  },
}

export function EditorSaveStatus({ status }: { status: SaveStatus | null }) {
  const { t } = useTranslation()
  if (!status) return null
  const meta = SAVE_STATUS_META[status]

  return (
    <span
      className={`inline-flex h-5 min-w-5 items-center justify-end gap-1 text-[11px] transition-colors ${meta.className}`}
      aria-live="polite"
      aria-label={t(meta.ariaLabelKey)}
    >
      {status === 'auto-saved' ? (
        <Check className="h-3 w-3 opacity-45" />
      ) : meta.dotClassName ? (
        <span className={`h-1.5 w-1.5 rounded-full ${meta.dotClassName}`} />
      ) : null}
      <span className={meta.subtle ? 'sr-only' : ''}>{t(meta.labelKey)}</span>
    </span>
  )
}
