import { useEffect, useMemo, useRef, useState } from 'react'
import { CircleCheck, CircleX, Loader2, PlugZap } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { pingModelProfile } from './api'
import type { ModelPingResult, ModelProfileSettings } from './types'

type PingState =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'success'; result: ModelPingResult }
  | { status: 'error'; message: string }

/** Owns one cancellable connection check so editing or removing a profile
 * cannot leave stale validation state attached to a different route. */
export function ModelProfilePingButton({ profile }: { profile: ModelProfileSettings }) {
  const { t } = useTranslation()
  const [state, setState] = useState<PingState>({ status: 'idle' })
  const requestRef = useRef<AbortController | null>(null)
  const fingerprint = useMemo(() => JSON.stringify(profile), [profile])

  useEffect(() => {
    requestRef.current?.abort()
    requestRef.current = null
    setState({ status: 'idle' })
  }, [fingerprint])

  useEffect(() => () => requestRef.current?.abort(), [])

  const ping = async () => {
    requestRef.current?.abort()
    const request = new AbortController()
    requestRef.current = request
    setState({ status: 'loading' })
    try {
      const result = await pingModelProfile(profile, request.signal)
      if (requestRef.current === request) {
        requestRef.current = null
        setState({ status: 'success', result })
      }
    } catch (error) {
      if (request.signal.aborted) return
      requestRef.current = null
      setState({ status: 'error', message: error instanceof Error ? error.message : String(error) })
    }
  }

  return (
    <div className="flex min-w-0 flex-col gap-1.5 sm:flex-row sm:items-center">
      <Button type="button" variant="outline" size="sm" disabled={state.status === 'loading'} onClick={() => void ping()}>
        {state.status === 'loading'
          ? <Loader2 className="animate-spin" data-icon="inline-start" />
          : <PlugZap data-icon="inline-start" />}
        {state.status === 'loading' ? t('settings.model.pingTesting') : t('settings.model.pingAction')}
      </Button>
      <div className="min-w-0 text-[11px] leading-5 text-[var(--nova-text-faint)]" aria-live="polite">
        {state.status === 'idle' && t('settings.model.pingHint')}
        {state.status === 'success' && (
          <span className="inline-flex min-w-0 items-center gap-1 text-emerald-600 dark:text-emerald-400">
            <CircleCheck className="size-3.5 shrink-0" />
            <span className="truncate">
              {t('settings.model.pingSuccess', {
                latency: state.result.latency_ms,
                provider: state.result.provider,
                protocol: state.result.protocol,
              })}
            </span>
          </span>
        )}
        {state.status === 'error' && (
          <span className="inline-flex min-w-0 items-start gap-1 text-red-600 dark:text-red-400" role="alert">
            <CircleX className="mt-0.5 size-3.5 shrink-0" />
            <span className="break-words">{t('settings.model.pingFailure', { error: state.message })}</span>
          </span>
        )}
      </div>
    </div>
  )
}
