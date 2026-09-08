import { type ReactNode, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { APIError } from '@/lib/api-client/client'
import { LoadingState } from '@/components/common/LoadingState'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { Button } from '@/components/ui/button'
import { RemoteAccessLogin } from '@/components/RemoteAccessLogin'
import { remoteAccessQuery } from './api'

export function RemoteAccessGate({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const authentication = useQuery(remoteAccessQuery)
  const { refetch } = authentication

  useEffect(() => {
    if (!authentication.isError) return
    if (authentication.error instanceof APIError && authentication.error.status === 401) return
    const reconnect = () => {
      if (document.visibilityState === 'visible') void refetch()
    }
    document.addEventListener('visibilitychange', reconnect)
    window.addEventListener('pageshow', reconnect)
    window.addEventListener('online', reconnect)
    return () => {
      document.removeEventListener('visibilitychange', reconnect)
      window.removeEventListener('pageshow', reconnect)
      window.removeEventListener('online', reconnect)
    }
  }, [authentication.error, authentication.isError, refetch])

  useEffect(() => {
    // Opening a scanned link in an existing tab can be a fragment-only navigation.
    const connectFromLink = () => {
      if (new URLSearchParams(window.location.hash.slice(1)).has('pair')) void refetch()
    }
    window.addEventListener('hashchange', connectFromLink)
    connectFromLink()
    return () => window.removeEventListener('hashchange', connectFromLink)
  }, [refetch])

  useEffect(() => {
    // Released clients stored the password here; cookies replace that credential.
    try { window.sessionStorage.removeItem('nova.remoteAccess.credentials') } catch { /* Storage may be unavailable. */ }
  }, [])

  if (authentication.data?.authenticated) return children
  if (authentication.isPending) return <LoadingState data-nova-app-shell="true" label={t('remoteAccess.connecting')} />
  if (authentication.error && !(authentication.error instanceof APIError && authentication.error.status === 401)) {
    return (
      <main data-nova-app-shell="true" className="flex min-h-dvh items-center justify-center bg-background p-4 text-foreground">
        <div className="flex w-full max-w-sm flex-col gap-4">
          <InlineErrorNotice title={t('remoteAccess.connectionFailed')} message={authentication.error.message} />
          <Button onClick={() => void authentication.refetch()} disabled={authentication.isFetching}>{t('remoteAccess.retry')}</Button>
        </div>
      </main>
    )
  }
  return <RemoteAccessLogin key={authentication.error?.message} initialError={authentication.error?.message} />
}
