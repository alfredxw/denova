import { AlertTriangle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ActorStateField, ActorStateTemplate } from '../../../../types'
import { StateValueEditor } from './StateValueEditor'
import { FieldTypeBadge } from './FieldTypeBadge'
import { VisibilityBadge } from './VisibilityBadge'

interface TemplateStateEditorProps {
  template: ActorStateTemplate
  state: Record<string, unknown>
  onChange: (state: Record<string, unknown>) => void
}

export function TemplateStateEditor({ template, state, onChange }: TemplateStateEditorProps) {
  const { t } = useTranslation()
  const fields = template.fields || []

  const updateFieldValue = (field: ActorStateField, value: unknown) => {
    const next = { ...state }
    if (value === undefined || value === null || value === '') {
      delete next[field.name]
    } else {
      next[field.name] = value
    }
    onChange(next)
  }

  return (
    <div className="space-y-2">
      {fields.length === 0 ? (
        <div className="rounded-[12px] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-4 text-center text-[11px] text-[var(--nova-text-faint)]">
          {t('settingPanel.actorState.explorer.noTemplateFields')}
        </div>
      ) : (
        <div className="space-y-2">
          {fields.map((field) => (
            <TemplateFieldRow
              key={field.name}
              field={field}
              value={state[field.name]}
              onChange={(v) => updateFieldValue(field, v)}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function TemplateFieldRow({
  field,
  value,
  onChange,
}: {
  field: ActorStateField
  value: unknown
  onChange: (value: unknown) => void
}) {
  const isSensitive = field.visibility === 'hidden' || field.visibility === 'spoiler'

  return (
    <div className="rounded-[12px] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2.5">
      <div className="mb-1.5 flex flex-wrap items-center gap-1.5">
        <span className="text-[12px] font-medium text-[var(--nova-text)]">
          {field.name}
        </span>
        <FieldTypeBadge type={field.type} />
        {field.visibility ? <VisibilityBadge visibility={field.visibility} /> : null}
        {isSensitive ? (
          <AlertTriangle className="h-3 w-3 text-[var(--nova-warning)]" />
        ) : null}
      </div>
      <div className="mt-2">
        <StateValueEditor
          type={field.type}
          value={value}
          onChange={onChange}
          options={field.options}
          min={field.min}
          max={field.max}
          compact
        />
      </div>
    </div>
  )
}
