import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { flushSync } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { useTheme } from 'next-themes'
import { ArrowLeft, Ellipsis, Maximize } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { management, platformError, type Instance, type RuntimeSnapshot } from './api'

/** Only the selected cross-origin frame receives this instance's credential. */
export function GamePlayer({
  runtime,
  title,
  visible,
  onExit,
  onOpenInstance,
  menuContent,
}: {
  runtime: RuntimeSnapshot
  title?: string
  visible: boolean
  onExit: () => void
  onOpenInstance?: (instance: Instance) => void | Promise<void>
  /** Host-owned story controls belong in the overlay, never outside the game surface. */
  menuContent?: ReactNode
}) {
  const { t, i18n } = useTranslation()
  const { resolvedTheme } = useTheme()
  const frame = useRef<HTMLIFrameElement>(null)
  const [menuOpen, setMenuOpen] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [frameMounted, setFrameMounted] = useState(true)
  const exitGuard = useRef(false)
  const exiting = useRef(false)
  const exitReply = useRef<{ id: string; finish: (ok: boolean) => void } | null>(null)
  const exit = useCallback(async () => {
    if (exiting.current) return
    exiting.current = true
    setBusy(true)
    setMenuOpen(false)
    setError('')
    try {
      if (exitGuard.current && frame.current?.contentWindow) {
        const ready = await new Promise<boolean>(resolve => {
          const id = crypto.randomUUID()
          // This bounds a frame handshake, never the Agent's execution time.
          const timer = window.setTimeout(() => { exitReply.current = null; resolve(false) }, 45000)
          exitReply.current = { id, finish: ok => { clearTimeout(timer); exitReply.current = null; resolve(ok) } }
          frame.current?.contentWindow?.postMessage({ type: 'denova:prepare-exit', requestId: id }, new URL(runtime.viewUrl!).origin)
        })
        if (!ready) { setError(t('platform.exitNotReady')); return }
        // The frame confirmed its durable cursor. Dispose optional material
        // requests before revoking their runtime; a stop failure can rehydrate it.
        flushSync(() => setFrameMounted(false))
      }
      // Views without a save handshake remain mounted until stopping succeeds.
      await management(`/runtimes/${runtime.id}/stop`, 'POST', {})
      flushSync(() => setFrameMounted(false))
      onExit()
    } catch (error) { setFrameMounted(true); setError(platformError(error)) }
    finally { exiting.current = false; setBusy(false) }
  }, [runtime.id, runtime.viewUrl, onExit, t])
  useEffect(() => {
    if (!runtime.viewUrl) return
    const origin = new URL(runtime.viewUrl).origin
    const receive = (event: MessageEvent) => {
      if (
        event.source !== frame.current?.contentWindow ||
        event.origin !== origin
      ) return
      if (event.data?.type === 'denova:exit-ready') {
        if (event.data.requestId === exitReply.current?.id) exitReply.current?.finish(event.data.ok === true)
        return
      }
      if (event.data?.type === 'denova:exit') { void exit(); return }
      if (event.data?.type === 'denova:open-instance' && onOpenInstance && typeof event.data.instanceId === 'string') {
        const instanceId = event.data.instanceId
        if (!/^[a-f\d-]{36}$/i.test(instanceId)) return
        void management<Instance>(`/instances/${instanceId}`).then(async instance => {
          // A preview stays inside its isolated environment, including work-authored children.
          if (instance.projectId !== runtime.context.scope.projectId || instance.gameId !== runtime.context.source.package.id || instance.preview !== (runtime.context.environment === 'preview')) {
            setError(t('platform.errors.PERMISSION_DENIED'))
            return
          }
          // Dispose the document and cancel its resource requests before revoking
          // the runtime; a work studio may still be loading material previews.
          flushSync(() => setFrameMounted(false))
          await management(`/runtimes/${runtime.id}/stop`, 'POST', {})
          await onOpenInstance(instance)
        }).catch(error => { setFrameMounted(true); setError(platformError(error)) })
        return
      }
      if (
        event.data?.type !== 'denova:ready' ||
        typeof event.data.nonce !== 'string' ||
        event.data.nonce.length > 128
      )
        return
      exitGuard.current = event.data.exitGuard === true
      frame.current.contentWindow?.postMessage(
        {
          type: 'denova:bootstrap',
          nonce: event.data.nonce,
          connection: runtime.connection,
          context: {
            ...runtime.context,
            locale: i18n.language,
            theme: resolvedTheme,
            visible,
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
  }, [runtime, i18n.language, resolvedTheme, visible, onOpenInstance, t, exit])
  useEffect(() => () => { exitReply.current?.finish(false) }, [])
  useEffect(() => {
    if (!runtime.viewUrl) return
    frame.current?.contentWindow?.postMessage({ type: 'denova:visibility', visible }, new URL(runtime.viewUrl).origin)
  }, [runtime.viewUrl, visible])
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
    <div className="relative flex min-h-0 flex-1 flex-col overflow-hidden bg-background" data-game-surface>
      <Button className="absolute top-3 left-3 z-10 bg-background/85 shadow-sm backdrop-blur-sm" size="icon" variant="outline" aria-label={t('platform.storyMenu')} disabled={busy} onClick={() => setMenuOpen(true)}><Ellipsis /></Button>
      {error && <InlineErrorNotice className="absolute top-16 right-3 left-3 z-10" message={error} />}
      {frameMounted && <iframe
        ref={frame}
        title={t('platform.gameFrame')}
        src={runtime.viewUrl}
        sandbox="allow-scripts allow-same-origin allow-forms allow-downloads"
        allow="autoplay; fullscreen"
        referrerPolicy="origin"
        className="min-h-0 w-full flex-1 border-0"
      />}
      <Dialog open={menuOpen} onOpenChange={setMenuOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('platform.storyMenu')}</DialogTitle>
            <DialogDescription>{title ?? t('platform.gameFrame')}{runtime.context.environment === 'preview' ? ` · ${t('platform.preview')}` : ''}</DialogDescription>
          </DialogHeader>
          {menuContent}
          <Button variant="outline" onClick={() => {
            setMenuOpen(false)
            // Fullscreen the game document so its own dialogs remain in the top layer.
            void frame.current?.requestFullscreen().catch(error => toast.error(platformError(error)))
          }}><Maximize />{t('platform.fullscreen')}</Button>
          <Button variant="outline" disabled={busy} onClick={() => void exit()}><ArrowLeft />{t('platform.exitGame')}</Button>
        </DialogContent>
      </Dialog>
    </div>
  )
}
