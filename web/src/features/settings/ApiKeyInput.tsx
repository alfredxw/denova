import { useState } from 'react'
import { Eye, EyeOff, KeyRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from '@/components/ui/input-group'

export function ApiKeyInput({ label, value, placeholder, onChange }: {
  label: string
  value: string
  placeholder: string
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const [visible, setVisible] = useState(false)
  const visibilityLabel = visible
    ? t('settings.apiKey.hide')
    : t('settings.apiKey.show')

  return (
    <InputGroup className="nova-field">
      <InputGroupAddon className="pl-2 pr-0">
        <KeyRound className="size-3.5" aria-hidden="true" />
      </InputGroupAddon>
      <InputGroupInput
        type={visible ? 'text' : 'password'}
        value={value}
        placeholder={placeholder}
        aria-label={label}
        autoComplete="off"
        autoCapitalize="none"
        spellCheck={false}
        className="font-mono text-xs"
        onChange={(event) => onChange(event.target.value)}
      />
      <InputGroupAddon align="inline-end" className="pr-1">
        <InputGroupButton
          type="button"
          size="icon-xs"
          aria-label={visibilityLabel}
          aria-pressed={visible}
          title={visibilityLabel}
          className="text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]"
          onClick={() => setVisible((current) => !current)}
        >
          {visible
            ? <EyeOff className="size-3.5" aria-hidden="true" />
            : <Eye className="size-3.5" aria-hidden="true" />}
        </InputGroupButton>
      </InputGroupAddon>
    </InputGroup>
  )
}
