import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import type { Installation } from '@/features/market/api'
import { openResourceDestination } from '@/features/market/resource-navigation'
import { platformError } from './api'

/** A package member updates through its owning installation, including mixed bundles. */
export function ExtensionSource({ owner, blockedReason, onUpdate }: {
  owner: Installation
  blockedReason?: string
  onUpdate: (installation: Installation) => Promise<void>
}) {
  const { t } = useTranslation()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  return <section className="flex min-w-0 flex-col gap-2" aria-label={t('market.acquired.source')}>
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-sm text-muted-foreground">{t('market.providedBy')}</span>
      <Button variant="link" className="h-auto max-w-full whitespace-normal p-0 text-left [overflow-wrap:anywhere]" onClick={() => openResourceDestination({ section: 'acquired', installationID: owner.installation_id })}>{owner.package.name}</Button>
    </div>
    <p className="text-xs text-muted-foreground [overflow-wrap:anywhere]">{owner.source.url
      ? <a href={owner.source.url} target="_blank" rel="noreferrer" className="underline underline-offset-4">{owner.source.url}</a>
      : owner.source.filename}{owner.source.ref && ` · ${owner.source.ref}`}{owner.source.path && ` · ${owner.source.path}`}</p>
    {owner.source.commit && <p className="text-xs text-muted-foreground [overflow-wrap:anywhere]">{t('market.import.commit', { commit: owner.source.commit.slice(0, 12) })}</p>}
    {owner.bindings.length > 1 && <p className="text-xs text-muted-foreground">{t('market.bundleUpdateHelp')}</p>}
    <div><Button variant="outline" size="sm" disabled={busy || !!blockedReason} onClick={() => {
      setBusy(true); setError('')
      void onUpdate(owner).catch(cause => {
        console.error('[resources] checking extension owner failed', { installationID: owner.installation_id, cause })
        setError(platformError(cause))
      }).finally(() => setBusy(false))
    }}>{t(busy ? 'market.working' : owner.source.kind === 'file' ? 'market.update.fromFile' : 'market.update.installed')}</Button></div>
    {error && <InlineErrorNotice message={error} />}
  </section>
}
