import { useId, useState } from 'react'
import { Copy, Eye, EyeOff, RefreshCw } from 'lucide-react'
import { nanoid } from 'nanoid'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { RemoteAccessConnections } from '@/features/remote-access/RemoteAccessConnections'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from '@/components/ui/input-group'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { Settings } from './types'

/** Edits user-level LAN credentials. onChange merges one atomic patch into the autosaved draft. */
export function LANAccessSettings({ draft, inherited, onChange }: {
  draft: Settings
  inherited: Settings
  onChange: (patch: Partial<Settings>) => void
}) {
  const { t } = useTranslation()
  const id = useId()
  // The server returns only password_set after saving. Keep the new password in
  // this mounted form so autosave cannot erase it before the owner copies it.
  const [sessionPassword, setSessionPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const password = draft.remote_access_password || sessionPassword
  const passwordSet = draft.remote_access_password_set || inherited.remote_access_password_set

  const changeAccess = (value: string) => {
    const enabled = value === 'inherit' ? null : value === 'true'
    const patch: Partial<Settings> = { allow_lan_access: enabled }
    if (enabled ?? inherited.allow_lan_access) {
      if (!draft.remote_access_username?.trim()) {
        patch.remote_access_username = inherited.remote_access_username?.trim() || `denova-${nanoid(6)}`
      }
      if (!draft.remote_access_password?.trim() && !passwordSet) {
        patch.remote_access_password = nanoid(24)
        setSessionPassword(patch.remote_access_password)
        setShowPassword(true)
      }
    }
    // Enablement and missing credentials must enter the same autosave request.
    onChange(patch)
    console.info('[settings] LAN access changed', { enabled, generatedPassword: Boolean(patch.remote_access_password) })
  }

  const generatePassword = () => {
    const next = nanoid(24)
    setSessionPassword(next)
    setShowPassword(true)
    onChange({ remote_access_password: next })
    console.info('[settings] Generated a new LAN access password')
  }

  const copyPassword = async () => {
    try {
      await navigator.clipboard.writeText(password)
      toast.success(t('settings.access.passwordCopied'))
    } catch (error) {
      console.warn('[settings] Failed to copy LAN access password', error)
      toast.error(t('settings.access.copyFailed'))
    }
  }

  return (
    <FieldGroup>
      <Field>
        <FieldLabel htmlFor={`${id}-enabled`}>{t('settings.access.allowLan')}</FieldLabel>
        <Select value={draft.allow_lan_access == null ? 'inherit' : String(draft.allow_lan_access)} onValueChange={changeAccess}>
          <SelectTrigger id={`${id}-enabled`} className="w-full"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="inherit">{t('common.defaultValue', { value: t(inherited.allow_lan_access ? 'settings.bool.true' : 'settings.bool.false') })}</SelectItem>
              <SelectItem value="true">{t('settings.bool.true')}</SelectItem>
              <SelectItem value="false">{t('settings.bool.false')}</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
        <FieldDescription>{t('settings.access.credentialsHint')}</FieldDescription>
      </Field>
      <Field>
        <FieldLabel htmlFor={`${id}-username`}>{t('settings.access.username')}</FieldLabel>
        <Input id={`${id}-username`} value={draft.remote_access_username ?? ''} placeholder={inherited.remote_access_username}
          autoComplete="off" spellCheck={false}
          onChange={event => onChange({ remote_access_username: event.target.value })} />
      </Field>
      <Field>
        <FieldLabel htmlFor={`${id}-password`}>{t('settings.access.password')}</FieldLabel>
        <InputGroup>
          <InputGroupInput id={`${id}-password`} type={showPassword ? 'text' : 'password'} value={password}
            autoComplete="new-password" spellCheck={false}
            placeholder={t(passwordSet ? 'settings.access.passwordSetPlaceholder' : 'settings.access.passwordPlaceholder')}
            onChange={event => {
              setSessionPassword(event.target.value)
              onChange({ remote_access_password: event.target.value })
            }} />
          <InputGroupAddon align="inline-end">
            <InputGroupButton type="button" aria-label={t(showPassword ? 'settings.access.hidePassword' : 'settings.access.showPassword')}
              disabled={!password} onClick={() => setShowPassword(value => !value)}>
              {showPassword ? <EyeOff /> : <Eye />}
            </InputGroupButton>
            <InputGroupButton type="button" aria-label={t('settings.access.copyPassword')} disabled={!password} onClick={() => void copyPassword()}>
              <Copy />
            </InputGroupButton>
          </InputGroupAddon>
        </InputGroup>
        {password && <FieldDescription>{t('settings.access.passwordHint')}</FieldDescription>}
        <Button type="button" variant="outline" className="self-start" onClick={generatePassword}>
          <RefreshCw data-icon="inline-start" />{t('settings.access.generatePassword')}
        </Button>
      </Field>
      <FieldDescription>{t('settings.access.restartHint')}</FieldDescription>
      <RemoteAccessConnections />
    </FieldGroup>
  )
}
