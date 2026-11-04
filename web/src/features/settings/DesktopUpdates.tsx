import { useEffect, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { APP_VERSION } from '@/app-version'
import { Button } from '@/components/ui/button'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { desktopBridge } from '@/lib/desktop'
import { errorMessage } from '@/lib/error-diagnostics'
import { Switch } from '@/components/ui/switch'
import { FieldRow } from './SettingsFields'

export function DesktopUpdates() {
  const { t } = useTranslation()
  const [preferences, setPreferences] = useState<{ automaticChecks: boolean; automaticDownloads: boolean }>()
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const updates = desktopBridge()!.updates
  useEffect(() => {
    let active = true
    const refresh = () => { void updates.preferences().then(value => { if (active) setPreferences(value) }).catch(e => { if (active) setError(errorMessage(e, t('settings.updates.error'))) }) }
    refresh()
    // The native update window can also change these preferences.
    window.addEventListener('focus', refresh)
    return () => { active = false; window.removeEventListener('focus', refresh) }
  }, [updates, t])
  const change = async (key: 'automaticChecks' | 'automaticDownloads', value: boolean) => {
    if (!preferences) return
    setSaving(true)
    try { await updates.setPreferences({ ...preferences, [key]: value }); setPreferences(await updates.preferences()); setError('') }
    catch (e) { setError(errorMessage(e, t('settings.updates.error'))) }
    finally { setSaving(false) }
  }
  return <>
    {preferences && <div inert={saving}>
      <FieldRow label={t('settings.updates.autoCheck')}><Switch aria-label={t('settings.updates.autoCheck')} checked={preferences.automaticChecks} onCheckedChange={v => { void change('automaticChecks', v) }} /></FieldRow>
      <FieldRow label={t('settings.updates.autoDownload')}><Switch aria-label={t('settings.updates.autoDownload')} checked={preferences.automaticDownloads} onCheckedChange={v => { void change('automaticDownloads', v) }} /></FieldRow>
    </div>}
    <p className="text-xs text-[var(--nova-text-muted)]">{t('settings.updates.desktopHint')}</p>
    <div className="flex flex-wrap items-center justify-between gap-3">
      <span className="text-sm text-[var(--nova-text-muted)]">{t('settings.updates.currentVersion', { version: APP_VERSION })}</span>
      <Button variant="outline" onClick={() => { void updates.open().catch(e => setError(errorMessage(e, t('settings.updates.error')))) }}>
        <RefreshCw className="mr-2 size-4" />{t('settings.updates.check')}
      </Button>
    </div>
    {error && <InlineErrorNotice message={error} title={t('settings.updates.error')} />}
  </>
}
