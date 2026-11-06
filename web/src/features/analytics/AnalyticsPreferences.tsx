import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SettingsFieldRow } from '@/components/forms/settings-field-row'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/lib/toast'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { ANALYTICS_PREFERENCES_CHANGED, fetchAnalyticsPreferences, recordAppOpened, setAnalyticsEnabled, type AnalyticsPreferences } from './api'

function useAnalyticsPreferences() {
  const { t } = useTranslation()
  const [preferences, setPreferences] = useState<AnalyticsPreferences | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState(false)
  useEffect(() => {
    if (preferences?.available && preferences.enabled && preferences.notice_seen) {
      void recordAppOpened().catch(cause => console.warn('[analytics] Failed to record the application opening', cause))
    }
  }, [preferences?.available, preferences?.enabled, preferences?.notice_seen])
  useEffect(() => {
    let active = true
    void fetchAnalyticsPreferences().then(value => { if (active) setPreferences(value) })
      .catch(cause => {
        console.warn('[analytics] Failed to load usage preferences', cause)
        // Optional statistics must never prevent the creation/onboarding UI.
        if (active) setPreferences({ enabled: false, notice_seen: true, available: false })
      })
    const onChange = (event: Event) => setPreferences((event as CustomEvent<AnalyticsPreferences>).detail)
    window.addEventListener(ANALYTICS_PREFERENCES_CHANGED, onChange)
    return () => { active = false; window.removeEventListener(ANALYTICS_PREFERENCES_CHANGED, onChange) }
  }, [])
  const update = async (enabled: boolean) => {
    if (saving) return
    setSaving(true); setError(false)
    try { setPreferences(await setAnalyticsEnabled(enabled)) }
    catch (cause) {
      console.warn('[analytics] Failed to save usage preferences', cause)
      setError(true)
      toast.error(t('settings.analytics.saveFailed'))
      try {
        const current = await fetchAnalyticsPreferences()
        setPreferences(current)
        window.dispatchEvent(new CustomEvent(ANALYTICS_PREFERENCES_CHANGED, { detail: current }))
      } catch { /* Collection remains fenced by the backend. */ }
    }
    finally { setSaving(false) }
  }
  return { preferences, saving, error, update }
}

export function AnalyticsNotice({ onReadyChange }: { onReadyChange?: (ready: boolean) => void }) {
  const { t } = useTranslation()
  const { preferences, saving, error, update } = useAnalyticsPreferences()
  useEffect(() => {
    onReadyChange?.(Boolean(preferences && (!preferences.available || preferences.notice_seen)))
  }, [preferences, onReadyChange])
  return (
    <Dialog open={Boolean(preferences?.available && !preferences.notice_seen)}>
      <DialogContent showCloseButton={false} onEscapeKeyDown={event => event.preventDefault()} onPointerDownOutside={event => event.preventDefault()}>
        <DialogHeader>
          <DialogTitle>{t('settings.analytics.noticeTitle')}</DialogTitle>
          <DialogDescription>{t('settings.analytics.noticeDescription')}</DialogDescription>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">{t('settings.analytics.privacyDescription')}</p>
        {error && <p role="alert" className="text-sm text-destructive">{t('settings.analytics.saveFailed')}</p>}
        <DialogFooter>
          <Button variant="outline" disabled={saving} onClick={() => void update(false)}>{t('settings.analytics.disable')}</Button>
          <Button autoFocus disabled={saving} onClick={() => void update(true)}>{t('settings.analytics.acknowledge')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function AnalyticsSettings() {
  const { t } = useTranslation()
  const { preferences, saving, error, update } = useAnalyticsPreferences()
  return (
    <div data-settings-field="settings.analytics.enabled" className="flex flex-col gap-2">
      <SettingsFieldRow title={t('settings.analytics.enabled')} description={t('settings.analytics.settingsDescription')} controlClassName="shrink-0">
        <Switch aria-label={t('settings.analytics.enabled')} checked={preferences?.available === true && preferences.enabled} disabled={!preferences || saving || !preferences.available} onCheckedChange={enabled => void update(enabled)} />
      </SettingsFieldRow>
      <p className="text-xs text-muted-foreground">{t('settings.analytics.privacyDescription')}</p>
      {preferences && !preferences.available && <p className="text-xs text-muted-foreground">{t('settings.analytics.developmentDisabled')}</p>}
      {error && <p role="alert" className="text-sm text-destructive">{t('settings.analytics.saveFailed')}</p>}
    </div>
  )
}
