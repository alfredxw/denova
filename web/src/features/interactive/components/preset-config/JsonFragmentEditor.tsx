import { useEffect, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Textarea } from '@/components/ui/textarea'
import { PresetField } from './PresetField'
import { formatPresetJSON } from './utils'

interface JsonFragmentEditorProps {
  label: string
  value: unknown
  expected?: 'array' | 'object'
  onChange: (value: unknown) => void
  onValidChange: (valid: boolean) => void
}

/** JSON fragment editor with a single validation and error-display contract. */
export function JsonFragmentEditor({
  label,
  value,
  expected = 'array',
  onChange,
  onValidChange,
}: JsonFragmentEditorProps) {
  const { t } = useTranslation()
  const inputId = useId()
  const errorId = useId()
  const formattedValue = formatPresetJSON(value ?? (expected === 'array' ? [] : {}))
  const [text, setText] = useState(formattedValue)
  const [error, setError] = useState('')

  useEffect(() => {
    setText(formattedValue)
    setError('')
    onValidChange(true)
  }, [formattedValue, onValidChange])

  const update = (next: string) => {
    setText(next)
    try {
      const parsed = JSON.parse(next)
      if (expected === 'array' && !Array.isArray(parsed)) {
        throw new Error(t('settingPanel.presetConfig.jsonArrayRequired'))
      }
      if (expected === 'object' && (!parsed || typeof parsed !== 'object' || Array.isArray(parsed))) {
        throw new Error(t('settingPanel.storyDirector.jsonObjectRequired'))
      }
      setError('')
      onValidChange(true)
      onChange(parsed)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('settingPanel.storyDirector.invalidJSON'))
      onValidChange(false)
    }
  }

  return (
    <PresetField label={label} htmlFor={inputId} error={error || undefined} errorId={error ? errorId : undefined}>
      <Textarea
        id={inputId}
        autoResize={false}
        className="nova-field min-h-28 resize-y font-mono text-xs leading-5 shadow-none focus-visible:ring-0"
        value={text}
        aria-invalid={Boolean(error)}
        aria-describedby={error ? errorId : undefined}
        onChange={(event) => update(event.target.value)}
      />
    </PresetField>
  )
}
