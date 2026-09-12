import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { QRCodeSVG } from 'qrcode.react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { createConnectionLink, logoutRemoteAccess, remoteAccessQuery } from './api'

/** Local owners issue one-use links; remote browsers can retire their own login. */
export function RemoteAccessConnections() {
  const { t } = useTranslation()
  const { data } = useQuery(remoteAccessQuery)
  const [link, setLink] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const connect = async () => {
    setBusy(true)
    setError('')
    try { setLink((await createConnectionLink()).url) }
    catch (e) { console.warn('[remote-access] Connection link failed', e); setError((e as Error).message) }
    finally { setBusy(false) }
  }
  const logout = async () => {
    setBusy(true)
    setError('')
    try { await logoutRemoteAccess() }
    catch (e) { console.warn('[remote-access] Logout failed', e); setError((e as Error).message); setBusy(false) }
  }

  if (!data?.authenticated) return null
  return (
    <FieldGroup>
      {data.local && data.lan_url && <Field>
        <FieldLabel htmlFor="remote-lan-url">{t('remoteAccess.lanAddress')}</FieldLabel>
        <Input id="remote-lan-url" value={data.lan_url} readOnly onFocus={event => event.target.select()} />
        <FieldDescription>{t('remoteAccess.lanAddressHint')}</FieldDescription>
        <Button type="button" variant="outline" onClick={() => void connect()} disabled={busy}>{t(link ? 'remoteAccess.regenerateLink' : 'remoteAccess.createLink')}</Button>
        <FieldDescription>{t('remoteAccess.linkHint')}</FieldDescription>
      </Field>}
      {data.local && data.lan_url && link && <Field>
        <QRCodeSVG value={link} size={240} marginSize={4} level="M" title={t('remoteAccess.qrCode')} role="img" className="h-auto w-full max-w-60 self-center rounded-md" />
        <FieldDescription className="text-center">{t('remoteAccess.qrHint')}</FieldDescription>
        <FieldLabel htmlFor="remote-connection-link">{t('remoteAccess.connectionLink')}</FieldLabel>
        <Input id="remote-connection-link" value={link} readOnly onFocus={event => event.target.select()} />
        <FieldDescription>{t('remoteAccess.linkCopyHint')}</FieldDescription>
      </Field>}
      {!data.local && <Field>
        <Button type="button" variant="outline" onClick={() => void logout()} disabled={busy}>{t('remoteAccess.signOut')}</Button>
        <FieldDescription>{t('remoteAccess.rememberHint')}</FieldDescription>
      </Field>}
      <Field>
        <FieldLabel>{t('remoteAccess.installTitle')}</FieldLabel>
        <FieldDescription>{t('remoteAccess.installHint')}</FieldDescription>
      </Field>
      {error && <FieldError>{error}</FieldError>}
    </FieldGroup>
  )
}
