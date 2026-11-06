import { randomUUID } from '@/lib/uuid'
import { useCallback, useEffect, useImperativeHandle, useRef, useState, type ReactNode, type Ref } from 'react'
import { flushSync } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { useTheme } from 'next-themes'
import { ArrowLeft, Ellipsis, Maximize } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { management, platformError, type Instance, type RuntimeSnapshot } from './api'
import { parsePagePresentation, type PagePresentation } from './page-presentation'

/** Host close controls must use the same save guard and runtime disposal as the view. */
export interface GamePlayerHandle {
  requestExit: () => Promise<void>
  refresh: () => void
  isBusy: () => boolean
  pageAction: (id: string) => void
}

/** Only the selected cross-origin frame receives this instance's credential. */
export function GamePlayer({
  ref,
  runtime,
  title,
  visible,
  onExit,
  onOpenInstance,
  menuContent,
  variant = 'game',
  onStop,
  onContext,
  onPresentation,
  onOpenLore,
  showMenu = true,
  externalBusy = false,
}: {
  ref?: Ref<GamePlayerHandle>
  runtime: RuntimeSnapshot
  title?: string
  visible: boolean
  onExit: () => void
  onOpenInstance?: (instance: Instance) => void | Promise<void>
  /** Host-owned story controls belong in the overlay, never outside the game surface. */
  menuContent?: ReactNode
  variant?: 'game' | 'plugin'
  onStop?: () => Promise<void>
  onContext?: (context: unknown) => void
  onPresentation?: (presentation: PagePresentation) => void
  /** Only a top-level plugin page can open native Project content. */
  onOpenLore?: () => void
  showMenu?: boolean
  /** Host-owned page Agent work participates in the same close guard. */
  externalBusy?: boolean
}) {
  const { t, i18n } = useTranslation()
  const { resolvedTheme } = useTheme()
  const frame = useRef<HTMLIFrameElement>(null)
  const [menuOpen, setMenuOpen] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [frameMounted, setFrameMounted] = useState(true)
  const [confirmClose, setConfirmClose] = useState(false)
  const viewState = useRef({ busy: false, dirty: false })
  const exitGuard = useRef(false)
  const exiting = useRef(false)
  const exitReply = useRef<{ id: string; finish: (ok: boolean) => void } | null>(null)
  const exit = useCallback(async (force = false) => {
    if (exiting.current) return
    if (variant === 'plugin' && !force && (externalBusy || viewState.current.busy || viewState.current.dirty)) { setMenuOpen(false); setConfirmClose(true); return }
    exiting.current = true
    setBusy(true)
    setMenuOpen(false)
    setError('')
    try {
      if (!force && exitGuard.current && frame.current?.contentWindow) {
        const ready = await new Promise<boolean>(resolve => {
          const id = randomUUID()
          // This bounds a frame handshake, never the Agent's execution time.
          const timer = window.setTimeout(() => { exitReply.current = null; resolve(false) }, 45000)
          exitReply.current = { id, finish: ok => { clearTimeout(timer); exitReply.current = null; resolve(ok) } }
          frame.current?.contentWindow?.postMessage({ type: 'denova:prepare-exit', requestId: id }, new URL(runtime.viewUrl!).origin)
        })
        if (!ready) { setError(t(variant === 'plugin' ? 'platform.plugins.exitNotReady' : 'platform.exitNotReady')); return }
      }
      // After save guards permit exit, cancel frame requests before revoking
      // their runtime. A failed stop rehydrates the view and reports the error.
      flushSync(() => setFrameMounted(false))
      if (onStop) await onStop()
      else await management(`/runtimes/${runtime.id}/stop`, 'POST', {})
      onExit()
    } catch (error) { setFrameMounted(true); setError(platformError(error)) }
    finally { exiting.current = false; setBusy(false) }
  }, [runtime.id, runtime.viewUrl, onExit, onStop, variant, externalBusy, t])
  useImperativeHandle(ref, () => ({ requestExit: () => exit(), isBusy: () => externalBusy || viewState.current.busy || viewState.current.dirty, refresh: () => frame.current?.contentWindow?.postMessage({ type: 'denova:refresh' }, new URL(runtime.viewUrl!).origin), pageAction: id => frame.current?.contentWindow?.postMessage({ type: 'denova:page-action', id }, new URL(runtime.viewUrl!).origin) }), [exit, runtime.viewUrl, externalBusy])
  useEffect(() => {
    if (!runtime.viewUrl) return
    const origin = new URL(runtime.viewUrl).origin
    const receive = (event: MessageEvent) => {
      if (
        event.source !== frame.current?.contentWindow ||
        event.origin !== origin
      ) return
      if (event.data?.type === 'denova:open-tab' && event.data.tab === 'lore' && variant === 'plugin' && visible && onOpenLore) {
        onOpenLore()
        return
      }
      if (event.data?.type === 'denova:page-presentation' && onPresentation) {
        const value = parsePagePresentation(event.data.presentation)
        if (value) onPresentation(value)
        return
      }
      if (event.data?.type === 'denova:page-context' && onContext) {
        try {
          if (new TextEncoder().encode(JSON.stringify(event.data.context)).byteLength > 32768) setError(t('platform.errors.LIMIT_EXCEEDED'))
          else onContext(event.data.context)
        } catch { setError(t('platform.errors.LIMIT_EXCEEDED')) }
        return
      }
      if (event.data?.type === 'denova:state' && variant === 'plugin') {
        viewState.current = { busy: event.data.busy === true, dirty: event.data.dirty === true }
        return
      }
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
  }, [runtime, i18n.language, resolvedTheme, visible, onOpenInstance, onContext, onPresentation, onOpenLore, t, exit, variant])
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
            setError(t(variant === 'plugin' ? 'platform.plugins.runtimeStopped' : 'platform.runtimeStopped'))
        })
        .catch((error) => setError(platformError(error)))
    }, 3000)
    return () => window.clearInterval(timer)
  }, [runtime.id, t, visible, variant])
  return (
    <div className="relative flex min-h-0 flex-1 flex-col overflow-hidden bg-background" data-game-surface>
      {showMenu && <Button data-slot="game-player-menu" className="absolute top-3 left-3 z-10 bg-background/85 shadow-sm backdrop-blur-sm" size="icon" variant="outline" aria-label={t(variant === 'plugin' ? 'platform.plugins.panelMenu' : 'platform.storyMenu')} disabled={busy} onClick={() => setMenuOpen(true)}><Ellipsis /></Button>}
      {error && <InlineErrorNotice className="absolute top-16 right-3 left-3 z-10" message={error} />}
      {frameMounted && <iframe
        ref={frame}
        title={t(variant === 'plugin' ? 'platform.plugins.frame' : 'platform.gameFrame')}
        src={runtime.viewUrl}
        sandbox="allow-scripts allow-same-origin allow-forms allow-downloads"
        allow="autoplay; fullscreen"
        referrerPolicy="origin"
        className="min-h-0 w-full flex-1 border-0"
      />}
      <Dialog open={menuOpen} onOpenChange={setMenuOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t(variant === 'plugin' ? 'platform.plugins.panelMenu' : 'platform.storyMenu')}</DialogTitle>
            <DialogDescription>{title ?? t('platform.gameFrame')}{runtime.context.environment === 'preview' ? ` · ${t('platform.preview')}` : ''}</DialogDescription>
          </DialogHeader>
          {menuContent}
          <Button variant="outline" onClick={() => {
            setMenuOpen(false)
            // Fullscreen the game document so its own dialogs remain in the top layer.
            void frame.current?.requestFullscreen().catch(error => toast.error(platformError(error)))
          }}><Maximize />{t('platform.fullscreen')}</Button>
          <Button variant="outline" disabled={busy} onClick={() => void exit()}><ArrowLeft />{t(variant === 'plugin' ? 'platform.plugins.closePanel' : 'platform.exitGame')}</Button>
          {variant === 'plugin' && error && <Button variant="destructive" disabled={busy} onClick={() => { setMenuOpen(false); setConfirmClose(true) }}>{t('platform.plugins.forceClose')}</Button>}
        </DialogContent>
      </Dialog>
      <Dialog open={confirmClose} onOpenChange={setConfirmClose}><DialogContent>
        <DialogHeader><DialogTitle>{t('platform.plugins.closePanel')}</DialogTitle><DialogDescription>{t('platform.plugins.closeWarning')}</DialogDescription></DialogHeader>
        <Button variant="outline" onClick={() => setConfirmClose(false)}>{t('common.cancel')}</Button>
        <Button variant="destructive" onClick={() => { setConfirmClose(false); void exit(true) }}>{t('platform.plugins.forceClose')}</Button>
      </DialogContent></Dialog>
    </div>
  )
}
