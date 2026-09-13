import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Info, LockKeyhole } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Switch } from '@/components/ui/switch'
import { Separator } from '@/components/ui/separator'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { management, platformError, type CatalogEntry, type Release } from './api'
import { ExtensionConfiguration } from './ExtensionConfiguration'

/** Consumer settings belong to the extension; permissions remain host policy. */
export function InstalledExtensionSettings({ item, release, active, onDirtyChange, onSaved }: {
  item: CatalogEntry; release: Release; active: boolean; onDirtyChange: (dirty: boolean) => void; onSaved: () => void
}) {
  const [settingsDirty, setSettingsDirty] = useState(false)
  const [permissionsDirty, setPermissionsDirty] = useState(false)
  useEffect(() => onDirtyChange(settingsDirty || permissionsDirty), [onDirtyChange, permissionsDirty, settingsDirty])
  return <div className="flex min-w-0 flex-col gap-5">
    <ExtensionConfiguration kind={item.kind} packageId={item.id} releaseId={release.ref.releaseId} active={active} onDirtyChange={setSettingsDirty} onSaved={onSaved} />
    <Separator />
    <InstalledPermissions item={item} release={release} onDirtyChange={setPermissionsDirty} onSaved={onSaved} />
  </div>
}

function InstalledPermissions({ item, release, onDirtyChange, onSaved }: { item: CatalogEntry; release: Release; onDirtyChange: (dirty: boolean) => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const id = useId()
  const [grants, setGrants] = useState(release.grants ?? item.grants)
  const [baseline, setBaseline] = useState(release.grants ?? item.grants)
  const observed = useRef(release.grants ?? item.grants)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const { required = [], optional = [] } = release.manifest.permissions
  const dirty = JSON.stringify([...grants].sort()) !== JSON.stringify([...baseline].sort())
  useEffect(() => onDirtyChange(dirty || busy), [busy, dirty, onDirtyChange])
  useEffect(() => {
    const next = release.grants ?? item.grants
    if (observed.current === next) return
    observed.current = next
    if (dirty || busy) return
    setBaseline(next)
    setGrants(next)
  }, [busy, dirty, release.grants, item.grants])
  const save = async () => {
    setBusy(true)
    setError('')
    try {
      await management(`/packages/${item.kind}/${item.id}/permissions`, 'PUT', { releaseId: release.ref.releaseId, grants })
      setBaseline(grants)
      onSaved()
    } catch (cause) {
      console.error('[extensions] saving installed settings failed', { kind: item.kind, packageId: item.id, cause })
      setError(platformError(cause))
    } finally { setBusy(false) }
  }
  return <section aria-label={t('platform.settings.permissions')} className="flex min-w-0 flex-col gap-3">
    <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
      <h2 className="text-sm font-semibold">{t('platform.settings.permissions')}</h2>
      {required.length + optional.length > 0 && <p className={'flex min-w-0 flex-1 items-start gap-1.5 text-xs leading-relaxed ' + (optional.length ? 'text-amber-700 dark:text-amber-400' : 'text-muted-foreground')}><Info className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />{t(optional.length ? 'platform.settings.permissionsHelp' : 'platform.settings.requiredHelp')}</p>}
      {optional.length > 0 && <div className="ml-auto flex items-center gap-2">
        {dirty && <Button variant="ghost" size="sm" disabled={busy} onClick={() => { setGrants(baseline); setError('') }}>{t('common.cancel')}</Button>}
        <Button variant="outline" size="sm" disabled={busy || !dirty} onClick={() => void save()}>{t('platform.settings.savePermissions')}</Button>
      </div>}
    </div>
    <FieldGroup className="gap-0 divide-y">
      {[...required, ...optional].map(permission => <Field key={permission} orientation="horizontal" className="min-h-12 gap-4 py-3">
        <FieldLabel htmlFor={id + permission} className="min-w-0 flex-1 flex-wrap [overflow-wrap:anywhere]">{t('platform.permission.' + permission)}{required.includes(permission) && <Badge variant="secondary"><LockKeyhole data-icon="inline-start" />{t('platform.required')}</Badge>}</FieldLabel>
        <Switch id={id + permission} aria-label={t('platform.permission.' + permission) + (required.includes(permission) ? ` (${t('platform.required')})` : '')} checked={grants.includes(permission)} disabled={busy || required.includes(permission)} onCheckedChange={checked => setGrants(current => checked ? [...current, permission] : current.filter(value => value !== permission))} />
      </Field>)}
      {required.length + optional.length === 0 && <p className="text-sm text-muted-foreground">{t('platform.settings.noPermissions')}</p>}
    </FieldGroup>
    {error && <InlineErrorNotice message={error} />}
    {dirty && <p role="status" className="text-xs text-muted-foreground">{t('platform.settings.unsaved')}</p>}
  </section>
}
