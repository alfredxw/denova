import { ChevronDown, ChevronRight, AlertTriangle } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { novaEase } from '@/features/motion/motion-tokens'
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
  const [showCustom, setShowCustom] = useState(false)

  // Determine which state keys are defined in the template
  const templatePaths = new Set(fields.map((f) => f.path))
  const customKeys = Object.keys(state).filter((k) => !templatePaths.has(k))

  const updateFieldValue = (field: ActorStateField, value: unknown) => {
    const next = { ...state }
    if (value === undefined || value === null || value === '') {
      delete next[field.path]
    } else {
      next[field.path] = value
    }
    onChange(next)
  }

  const updateCustomValue = (key: string, value: unknown) => {
    const next = { ...state }
    if (value === undefined || value === null || value === '') {
      delete next[key]
    } else {
      next[key] = value
    }
    onChange(next)
  }

  const removeCustomKey = (key: string) => {
    const next = { ...state }
    delete next[key]
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
              key={field.id || field.path}
              field={field}
              value={state[field.path]}
              onChange={(v) => updateFieldValue(field, v)}
            />
          ))}
        </div>
      )}

      {customKeys.length > 0 ? (
        <div className="mt-3">
          <button
            type="button"
            className="flex min-h-7 items-center gap-1 rounded-[8px] px-1 text-[11px] text-[var(--nova-text-faint)] transition-colors hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
            onClick={() => setShowCustom(!showCustom)}
            aria-expanded={showCustom}
          >
            {showCustom ? (
              <ChevronDown className="h-3 w-3" />
            ) : (
              <ChevronRight className="h-3 w-3" />
            )}
            <span className="font-medium">{t('settingPanel.actorState.explorer.customFields')}</span>
            <span className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] px-1.5 py-0.5 text-[10px]">
              {customKeys.length}
            </span>
          </button>
          <AnimatePresence>
            {showCustom ? (
              <motion.div
                initial={{ height: 0, opacity: 0 }}
                animate={{ height: 'auto', opacity: 1 }}
                exit={{ height: 0, opacity: 0 }}
                transition={{ duration: 0.15, ease: novaEase }}
                className="overflow-hidden"
              >
                <div className="mt-2 space-y-2">
                  {customKeys.map((key) => (
                    <CustomFieldRow
                      key={key}
                      path={key}
                      value={state[key]}
                      onChange={(v) => updateCustomValue(key, v)}
                      onRemove={() => removeCustomKey(key)}
                    />
                  ))}
                </div>
              </motion.div>
            ) : null}
          </AnimatePresence>
        </div>
      ) : null}
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
          {field.name || field.path}
        </span>
        <FieldTypeBadge type={field.type} />
        {field.visibility ? <VisibilityBadge visibility={field.visibility} /> : null}
        {isSensitive ? (
          <AlertTriangle className="h-3 w-3 text-[var(--nova-warning)]" />
        ) : null}
      </div>
      <div className="font-mono text-[10px] text-[var(--nova-text-faint)]">
        {field.path}
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

function CustomFieldRow({
  path,
  value,
  onChange,
  onRemove,
}: {
  path: string
  value: unknown
  onChange: (value: unknown) => void
  onRemove: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className="group rounded-[12px] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2.5">
      <div className="mb-1.5 flex items-center justify-between">
        <span className="font-mono text-[11px] text-[var(--nova-text-muted)]">
          {path}
        </span>
        <button
          type="button"
          className="min-h-7 rounded-[8px] px-2 text-[10px] text-[var(--nova-text-faint)] opacity-0 transition-opacity hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)] group-hover:opacity-100 group-focus-within:opacity-100 focus-visible:opacity-100 [@media(pointer:coarse)]:opacity-100"
          onClick={onRemove}
        >
          {t('settingPanel.actorState.explorer.remove')}
        </button>
      </div>
      <StateValueEditor
        type="string"
        value={typeof value === 'string' ? value : JSON.stringify(value ?? '')}
        onChange={onChange}
        compact
      />
    </div>
  )
}
