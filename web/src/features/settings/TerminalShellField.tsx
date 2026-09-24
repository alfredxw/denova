import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SettingsFieldRow } from '@/components/forms/settings-field-row'
import { Input } from '@/components/ui/input'
import { FieldDescription } from '@/components/ui/field'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

/** Edits the existing user shell preference; host detection must come from the backend. */
export function TerminalShellField({ value = '', inherited, windows, onChange }: {
  value?: string
  inherited?: string
  windows: boolean
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const [editingCustom, setEditingCustom] = useState(false)
  const normalized = value.trim().toLowerCase().replace(/\.exe$/, '')
  const preset = normalized === 'pwsh' || normalized === 'wsl' ? normalized : undefined
  const selection = editingCustom ? 'custom' : !value ? 'auto' : preset || 'custom'
  const showPath = !windows || selection === 'custom'

  return (
    <SettingsFieldRow
      title={t('settings.terminal.shell')}
      htmlFor={id}
      className="nova-settings-row rounded-md border-0 bg-transparent px-2 py-1.5"
      contentClassName="sm:w-44 sm:flex-none"
      controlClassName="flex-1 flex-col gap-2"
    >
      {windows && (
        <Select value={selection} onValueChange={(next) => {
          setEditingCustom(next === 'custom')
          if (next !== 'custom') onChange(next === 'auto' ? '' : `${next}.exe`)
        }}>
          <SelectTrigger id={id} aria-describedby={`${id}-hint`} size="sm" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="auto">{inherited ? t('common.defaultValue', { value: inherited }) : t('settings.terminal.shellAuto')}</SelectItem>
              <SelectItem value="pwsh">{t('settings.terminal.shellPwsh')}</SelectItem>
              <SelectItem value="wsl">{t('settings.terminal.shellWSL')}</SelectItem>
              <SelectItem value="custom">{t('settings.terminal.shellCustom')}</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      )}
      {showPath && (
        <Input
          id={windows ? undefined : id}
          aria-label={t('settings.terminal.shellCustom')}
          aria-describedby={`${id}-hint`}
          value={value}
          placeholder={inherited || t('settings.terminal.shellPathPlaceholder')}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
      <FieldDescription id={`${id}-hint`} className="text-[11px]">
        {t(windows ? 'settings.terminal.shellWindowsHint' : 'settings.terminal.shellHint')}
      </FieldDescription>
    </SettingsFieldRow>
  )
}
