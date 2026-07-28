import { useCallback, useEffect, useRef, useState, type MutableRefObject } from 'react'
import { useTranslation } from 'react-i18next'
import { useTheme } from 'next-themes'
import { FitAddon } from '@xterm/addon-fit'
import { WebglAddon } from '@xterm/addon-webgl'
import { Terminal } from '@xterm/xterm'
import { RotateCcw, TerminalSquare } from 'lucide-react'
import '@xterm/xterm/css/xterm.css'
import { Button } from '@/components/ui/button'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import type { AgentChatTerminalTab } from '../types'
import {
  closeTerminalSession,
  createTerminalSession,
  getTerminalRuntimeStatus,
  terminalAttachURL,
  type TerminalSessionInfo,
} from './api'
import { TerminalConnection } from './connection'
import { terminalTheme } from './theme'

/** Attach lifecycle for one terminal tab. Every state is rendered explicitly. */
type TerminalStatus = 'connecting' | 'ready' | 'exited' | 'error'

/**
 * The stack has to be a literal: the WebGL renderer rasterises glyphs onto a canvas through
 * `ctx.font`, and a CSS custom property cannot be resolved in that context — the whole
 * declaration would be dropped and every cell would fall back to the canvas default font.
 */
const TERMINAL_FONT_FAMILY =
  'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace'

interface TerminalTabViewProps {
  tab: AgentChatTerminalTab
  /** Hidden tabs stay mounted so a running CLI keeps its screen, but they must not be re-fitted. */
  active: boolean
  /** Reports the backend session so the tab can re-attach after a reload. */
  /** Returns false when the tab was closed while its asynchronous creation was still running. */
  onSessionEstablished: (tabId: string, session: TerminalSessionInfo) => boolean
  /** Receives the standard OSC 0/2 window title emitted by the foreground terminal program. */
  onTitleChange: (tabId: string, title: string) => void
}

/** Terminal surface backed by a backend pty session (used to run codex / claude code / any shell). */
export function TerminalTabView({ tab, active, onSessionEstablished, onTitleChange }: TerminalTabViewProps) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const dark = resolvedTheme !== 'light'
  const containerRef = useRef<HTMLDivElement | null>(null)
  const terminalRef = useRef<Terminal | null>(null)
  const fitRef = useRef<FitAddon | null>(null)
  const webglRef = useRef<WebglAddon | null>(null)
  const connectionRef = useRef<TerminalConnection | null>(null)
  const onTitleChangeRef = useRef(onTitleChange)
  onTitleChangeRef.current = onTitleChange
  /** Survives effect re-runs (including StrictMode double-invoke) so one tab never spawns two ptys. */
  const establishedRef = useRef<TerminalSessionInfo | null>(null)
  /** Shares the asynchronous resolution itself; the result ref alone is too late for StrictMode. */
  const resolvingRef = useRef<Promise<TerminalSessionInfo> | null>(null)
  const releasedSessionIdsRef = useRef(new Set<string>())
  const [status, setStatus] = useState<TerminalStatus>('connecting')
  const [statusDetail, setStatusDetail] = useState('')
  const [attempt, setAttempt] = useState(0)

  const fitTerminal = useCallback(() => {
    const terminal = terminalRef.current
    const fit = fitRef.current
    const container = containerRef.current
    if (!terminal || !fit || !container || container.clientWidth === 0 || container.clientHeight === 0) return
    try {
      fit.fit()
    } catch (error) {
      console.warn('[features/agent-chat/terminal/TerminalTabView.tsx] fit failed', { tabId: tab.id, error })
    }
  }, [tab.id])

  // Own the xterm instance for the lifetime of the tab; input is forwarded to whatever
  // connection is currently attached, so re-attaching never rebuilds the screen.
  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    const terminal = new Terminal({
      allowProposedApi: true,
      convertEol: false,
      cursorBlink: true,
      fontSize: 12,
      fontFamily: TERMINAL_FONT_FAMILY,
      scrollback: 5000,
      theme: terminalTheme(dark),
    })
    const fit = new FitAddon()
    terminal.loadAddon(fit)
    terminal.open(container)
    loadRenderer(terminal, webglRef)
    terminal.onData((data) => connectionRef.current?.send(data))
    terminal.onResize(({ cols, rows }) => connectionRef.current?.resize(cols, rows))
    terminal.onTitleChange((title) => {
      const normalized = normalizeTerminalTitle(title)
      if (normalized) onTitleChangeRef.current(tab.id, normalized)
    })
    terminalRef.current = terminal
    fitRef.current = fit

    const observer = new ResizeObserver(() => fitTerminal())
    observer.observe(container)
    return () => {
      observer.disconnect()
      terminal.dispose()
      terminalRef.current = null
      fitRef.current = null
    }
    // The instance intentionally outlives theme changes; only the palette is patched below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fitTerminal])

  useEffect(() => {
    // next-themes commits its data attribute in the same React update. Reading CSS variables
    // immediately can still see the previous palette, so apply on the next frame after styles
    // have resolved. This also repaints already-open terminals instead of requiring a restart.
    const id = window.requestAnimationFrame(() => {
      const terminal = terminalRef.current
      if (!terminal) return
      terminal.options.theme = terminalTheme(dark)
      terminal.refresh(0, terminal.rows - 1)
    })
    return () => window.cancelAnimationFrame(id)
  }, [dark])

  useEffect(() => {
    if (!active) return
    // A hidden tab is `display:none`, which makes the browser drop the WebGL context. The
    // loss handler disposes the addon and xterm falls back to the DOM renderer, but its
    // box-drawing glyphs come out garbled once the tab is shown again. Reloading the WebGL
    // renderer on activation restores a clean surface without rebuilding the screen.
    const id = window.requestAnimationFrame(() => {
      const terminal = terminalRef.current
      if (terminal) loadRenderer(terminal, webglRef)
      fitTerminal()
    })
    return () => window.cancelAnimationFrame(id)
  }, [active, fitTerminal])

  // Resolve the backend session and attach. Re-runs only when the tab asks for a restart.
  useEffect(() => {
    let cancelled = false
    const terminal = terminalRef.current
    if (!terminal) return

    setStatus('connecting')
    setStatusDetail('')
    fitTerminal()

    const attach = async () => {
      const pending = resolvingRef.current
        ?? resolveSession(tab, establishedRef.current, terminal.cols, terminal.rows)
      resolvingRef.current = pending
      let session: TerminalSessionInfo
      try {
        session = await pending
      } finally {
        if (resolvingRef.current === pending) resolvingRef.current = null
      }
      establishedRef.current = session
      if (!onSessionEstablished(tab.id, session)) {
        establishedRef.current = null
        if (!releasedSessionIdsRef.current.has(session.id)) {
          releasedSessionIdsRef.current.add(session.id)
          try {
            await closeTerminalSession(session.id)
          } catch (error) {
            releasedSessionIdsRef.current.delete(session.id)
            console.warn('[features/agent-chat/terminal/TerminalTabView.tsx] releasing abandoned session failed', {
              tabId: tab.id,
              sessionId: session.id,
              error,
            })
          }
        }
        return
      }
      if (cancelled) return
      const connection = new TerminalConnection(terminalAttachURL(session.id, session.token), {
        onOutput: (chunk) => terminalRef.current?.write(chunk),
        onControl: (frame) => {
          switch (frame.type) {
            case 'ready':
              setStatus('ready')
              connectionRef.current?.resize(terminalRef.current?.cols ?? session.cols, terminalRef.current?.rows ?? session.rows)
              break
            case 'exit':
              setStatus('exited')
              setStatusDetail(frame.error || String(frame.code))
              break
            case 'error':
              setStatus('error')
              setStatusDetail(frame.error)
              break
            case 'pong':
              break
          }
        },
        onClosed: (origin) => {
          if (origin === 'local' || cancelled) return
          // The pty outlives the socket, so a dropped transport is recoverable: keep the
          // screen and let the user re-attach instead of silently freezing the tab.
          setStatus((current) => (current === 'exited' ? current : 'error'))
          setStatusDetail((current) => current || t('agentChat.terminal.disconnected'))
        },
      })
      connectionRef.current = connection
    }

    attach().catch((error) => {
      if (cancelled) return
      console.error('[features/agent-chat/terminal/TerminalTabView.tsx] terminal attach failed', { tabId: tab.id, error })
      setStatus('error')
      setStatusDetail(error instanceof Error ? error.message : String(error))
    })

    return () => {
      cancelled = true
      connectionRef.current?.close()
      connectionRef.current = null
    }
    // onSessionEstablished is stable by construction; restarts are driven by `attempt`.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [attempt, tab.id])

  const restart = () => {
    const previousSessionId = establishedRef.current?.id || tab.terminalSessionId
    setStatus('connecting')
    setStatusDetail('')
    void (async () => {
      if (previousSessionId) await closeTerminalSession(previousSessionId)
      establishedRef.current = null
      resolvingRef.current = null
      terminalRef.current?.reset()
      setAttempt((value) => value + 1)
    })().catch((error) => {
      console.error('[features/agent-chat/terminal/TerminalTabView.tsx] restarting terminal failed', {
        tabId: tab.id,
        sessionId: previousSessionId,
        error,
      })
      setStatus('error')
      setStatusDetail(error instanceof Error ? error.message : String(error))
    })
  }

  const reattach = () => setAttempt((value) => value + 1)

  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)]">
      <div ref={containerRef} className="min-h-0 flex-1 overflow-hidden px-2 pt-2" />
      <TerminalStatusBar
        status={status}
        detail={statusDetail}
        label={terminalTabLabel(tab, t)}
        onRestart={restart}
        onReattach={reattach}
      />
    </div>
  )
}

/**
 * Interactive CLIs draw their frames out of box-drawing and block characters, and the DOM
 * renderer paints those from the font, whose glyphs are narrower than the cell — every border
 * comes out as a row of dashes. The WebGL renderer substitutes its own glyphs (`customGlyphs`,
 * which the DOM renderer ignores) so the segments join into continuous lines. A missing or lost
 * context degrades back to the DOM renderer rather than leaving the tab blank.
 *
 * Idempotent: disposing any previously loaded addon first lets the renderer be reloaded when a
 * tab regains focus (its WebGL context is dropped while the tab is `display:none`).
 */
function loadRenderer(terminal: Terminal, webglRef: MutableRefObject<WebglAddon | null>) {
  try {
    webglRef.current?.dispose()
    const webgl = new WebglAddon()
    webgl.onContextLoss(() => {
      webglRef.current = null
      webgl.dispose()
    })
    terminal.loadAddon(webgl)
    webglRef.current = webgl
  } catch (error) {
    console.warn('[features/agent-chat/terminal/TerminalTabView.tsx] webgl renderer unavailable', { error })
  }
}

function TerminalStatusBar({
  status,
  detail,
  label,
  onRestart,
  onReattach,
}: {
  status: TerminalStatus
  detail: string
  label: string
  onRestart: () => void
  onReattach: () => void
}) {
  const { t } = useTranslation()
  if (status === 'error') {
    return (
      <div className="flex shrink-0 items-center gap-2 px-2 pb-2 pt-1">
        <InlineErrorNotice
          className="min-w-0 flex-1"
          message={detail || t('agentChat.terminal.error')}
          title={t('agentChat.terminal.errorTitle')}
        />
        <Button type="button" variant="outline" size="xs" className="shrink-0" onClick={onReattach}>
          {t('agentChat.terminal.reattach')}
        </Button>
        <Button type="button" variant="ghost" size="xs" className="shrink-0" onClick={onRestart}>
          <RotateCcw data-icon="inline-start" />
          {t('agentChat.terminal.restart')}
        </Button>
      </div>
    )
  }
  return (
    <div className="flex h-7 shrink-0 items-center gap-2 border-t border-[var(--nova-border)] px-3 text-[11px] text-[var(--nova-text-faint)]">
      <TerminalSquare className="size-3" aria-hidden="true" />
      <span className="min-w-0 truncate">{label}</span>
      <span className="ml-auto shrink-0">
        {status === 'connecting' && t('agentChat.terminal.connecting')}
        {status === 'ready' && t('agentChat.terminal.running')}
        {status === 'exited' && t('agentChat.terminal.exited', { detail: detail || '0' })}
      </span>
      {status === 'exited' && (
        <Button type="button" variant="ghost" size="xs" onClick={onRestart}>
          <RotateCcw data-icon="inline-start" />
          {t('agentChat.terminal.restart')}
        </Button>
      )}
    </div>
  )
}

/**
 * Pick the session this tab should be attached to, in order of preference:
 * a session this tab already owns, a session persisted from a previous page load,
 * and finally a brand new pty.
 */
async function resolveSession(
  tab: AgentChatTerminalTab,
  established: TerminalSessionInfo | null,
  cols: number,
  rows: number,
): Promise<TerminalSessionInfo> {
  if (established) return established
  if (tab.terminalSessionId) {
    const runtime = await getTerminalRuntimeStatus()
    const existing = runtime.sessions.find((session) => session.id === tab.terminalSessionId)
    if (existing) return existing
  }
  // Built-in profiles are deliberately resolved by the backend from user settings. `custom`
  // remains only so terminal tabs persisted by earlier beta builds can still be restored.
  const legacyCustomLaunch = tab.profileId === 'custom'
    ? { command: tab.command || '', args: [] }
    : {}
  return createTerminalSession({
    owner_tab_id: tab.id,
    workspace: tab.workspace,
    profile_id: tab.profileId,
    title: tab.title,
    ...legacyCustomLaunch,
    cwd: tab.workspace,
    cols,
    rows,
  })
}

/** Human label for the status bar: the custom command, otherwise the profile name. */
export function terminalTabLabel(tab: AgentChatTerminalTab, t: (key: string) => string): string {
  if (tab.title) return tab.title
  if (tab.profileId === 'custom') return tab.command || t('agentChat.terminal.profile.custom')
  return t(`agentChat.terminal.profile.${tab.profileId}`)
}

/** Keep terminal-owned titles display-safe and bounded before persisting them as local UI state. */
export function normalizeTerminalTitle(title: string): string {
  const printable = title.replace(/[\u0000-\u001f\u007f]/g, '').trim()
  const application = [
    { pattern: /\bclaude(?:\s+code)?\b/i, label: 'claude' },
    { pattern: /\bcodex\b/i, label: 'codex' },
    { pattern: /\bnvim\b/i, label: 'nvim' },
    { pattern: /\bvim\b/i, label: 'vim' },
  ].find(({ pattern }) => pattern.test(printable))
  return application?.label ?? Array.from(printable).slice(0, 80).join('')
}
