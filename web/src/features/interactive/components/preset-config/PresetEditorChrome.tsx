import { Layers3 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ReactNode } from 'react'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

const controlClassName = 'nova-field h-8 min-w-0 text-xs shadow-none focus-visible:ring-0'

interface PresetMetadataPanelProps {
  name: string
  description: string
  status: string
  onNameChange: (value: string) => void
  onDescriptionChange: (value: string) => void
  hint?: string
  extra?: ReactNode
  sticky?: boolean
  testId?: string
}

/** Shared resource identity editor used by every preset module. */
export function PresetMetadataPanel({
  name,
  description,
  status,
  onNameChange,
  onDescriptionChange,
  hint,
  extra,
  sticky = false,
  testId,
}: PresetMetadataPanelProps) {
  const { t } = useTranslation()

  return (
    <section
      className={cn('preset-metadata-shell shrink-0', sticky && 'sticky top-0 z-20')}
      data-testid={testId || 'preset-metadata'}
    >
      <div className="preset-metadata-grid">
        <PresetField className="preset-metadata-name" label={t('settingPanel.field.name')}>
          <Input
            className={controlClassName}
            value={name}
            onChange={(event) => onNameChange(event.target.value)}
          />
        </PresetField>
        <PresetField className="preset-metadata-description" label={t('settingPanel.field.description')}>
          <Input
            className={controlClassName}
            value={description}
            onChange={(event) => onDescriptionChange(event.target.value)}
            placeholder={t('settingPanel.placeholder.description')}
          />
        </PresetField>
        {extra}
        <div className="preset-metadata-status grid min-w-0 gap-1.5">
          <span className="preset-field-label">{t('settingPanel.presetConfig.status')}</span>
          <span className="preset-status-badge" title={status}>
            <span className="preset-status-dot" aria-hidden="true" />
            <span className="truncate">{status}</span>
          </span>
        </div>
      </div>
      {hint ? <p className="preset-metadata-hint">{hint}</p> : null}
    </section>
  )
}

export function PresetField({
  label,
  children,
  className,
}: {
  label: string
  children: ReactNode
  className?: string
}) {
  return (
    <label className={cn('grid min-w-0 gap-1', className)}>
      <span className="preset-field-label">{label}</span>
      {children}
    </label>
  )
}

export function PresetEmptyState({
  title,
  description,
  action,
}: {
  title: string
  description: string
  action?: ReactNode
}) {
  return (
    <div className="flex min-h-0 flex-1 items-center justify-center p-5 sm:p-8">
      <div className="preset-empty-state">
        <span className="preset-empty-state-icon" aria-hidden="true">
          <Layers3 className="size-5" />
        </span>
        <div className="text-sm font-semibold text-[var(--nova-text)]">{title}</div>
        <div className="max-w-[46ch] text-xs leading-5 text-[var(--nova-text-muted)]">{description}</div>
        {action}
      </div>
    </div>
  )
}
