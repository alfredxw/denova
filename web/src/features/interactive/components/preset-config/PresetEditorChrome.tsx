import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import type { ReactNode } from 'react'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { presetInputClassName } from './editor-styles'
import { PresetField } from './PresetField'

const controlClassName = `${presetInputClassName} min-w-0 shadow-none`

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
  const nameId = useId()
  const descriptionId = useId()

  return (
    <section
      className={cn('preset-metadata-shell shrink-0', sticky && 'sticky top-0 z-20')}
      data-testid={testId || 'preset-metadata'}
    >
      <div className="preset-metadata-grid">
        <PresetField className="preset-metadata-name" label={t('settingPanel.field.name')} htmlFor={nameId}>
          <Input
            id={nameId}
            className={controlClassName}
            value={name}
            onChange={(event) => onNameChange(event.target.value)}
          />
        </PresetField>
        <PresetField className="preset-metadata-description" label={t('settingPanel.field.description')} htmlFor={descriptionId}>
          <Input
            id={descriptionId}
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
