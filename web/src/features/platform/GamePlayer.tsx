import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useTheme } from 'next-themes'
import { ArrowLeft, Maximize } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { management, platformError, type RuntimeSnapshot } from './api'

/** Only the selected cross-origin frame receives this instance's credential. */
export function GamePlayer({
  runtime,
  title,
  visible,
  onExit,
}: {
  runtime: RuntimeSnapshot
  title?: string
  visible: boolean
  onExit: () => void
}) {
  const { t, i18n } = useTranslation()
  const { resolvedTheme } = useTheme()
  const frame = useRef<HTMLIFrameElement>(null)
  const surface = useRef<HTMLDivElement>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    if (!runtime.viewUrl) return
    const origin = new URL(runtime.viewUrl).origin
    const receive = (event: MessageEvent) => {
      if (
        event.source !== frame.current?.contentWindow ||
        event.origin !== origin ||
        event.data?.type !== 'denova:ready' ||
        typeof event.data.nonce !== 'string' ||
        event.data.nonce.length > 128
      )
        return
      frame.current.contentWindow?.postMessage(
        {
          type: 'denova:bootstrap',
          nonce: event.data.nonce,
          connection: runtime.connection,
          context: {
            ...runtime.context,
            locale: i18n.language,
            theme: resolvedTheme,
          },
        },
        origin,
      )
    }
    window.addEventListener('message', receive)
    frame.current?.contentWindow?.postMessage(
      {
        type: 'denova:appearance',
        locale: i18n.language,
        theme: resolvedTheme,
      },
      origin,
    )
    return () => window.removeEventListener('message', receive)
  }, [runtime, i18n.language, resolvedTheme])
  useEffect(() => {
    if (!visible) return
    const timer = window.setInterval(() => {
      void management<RuntimeSnapshot[]>('/runtimes')
        .then((items) => {
          if (
            !items.some(
              (item) => item.id === runtime.id && item.status === 'running',
            )
          )
            setError(t('platform.runtimeStopped'))
        })
        .catch((error) => setError(platformError(error)))
    }, 3000)
    return () => window.clearInterval(timer)
  }, [runtime.id, t, visible])
  return (
    <div ref={surface} className="flex h-full min-h-0 flex-col bg-background">
      <header className="flex flex-wrap items-center justify-between gap-2 border-b p-3">
        <Button
          variant="ghost"
          disabled={busy}
          onClick={async () => {
            setBusy(true)
            try {
              await management(`/runtimes/${runtime.id}/stop`, 'POST', {})
              onExit()
            } catch (error) {
              setError(platformError(error))
            } finally {
              setBusy(false)
            }
          }}
        >
          <ArrowLeft data-icon="inline-start" />
          {t('platform.exitGame')}
        </Button>
        <span className="min-w-0 break-words text-sm text-muted-foreground">
          {title ?? t('platform.gameFrame')}
          {runtime.context.environment === 'preview'
            ? ` · ${t('platform.preview')}`
            : ''}
        </span>
        <Button
          variant="ghost"
          aria-label={t('platform.fullscreen')}
          onClick={() =>
            void surface.current
              ?.requestFullscreen()
              .catch((error) => toast.error(platformError(error)))
          }
        >
          <Maximize />
        </Button>
      </header>
      {error && <InlineErrorNotice className="m-3" message={error} />}
      <iframe
        ref={frame}
        title={t('platform.gameFrame')}
        src={runtime.viewUrl}
        sandbox="allow-scripts allow-same-origin allow-forms"
        referrerPolicy="origin"
        className="min-h-0 w-full flex-1 border-0"
      />
    </div>
  )
}
