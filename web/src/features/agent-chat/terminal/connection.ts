/**
 * WebSocket client for a single attached terminal session.
 *
 * Frame contract, mirroring internal/api/handlers/handler_terminal.go:
 *   - server -> client: binary frames are raw pty output, text frames are JSON control messages.
 *   - client -> server: binary frames are raw user input, text frames are JSON control messages.
 *
 * The connection is intentionally single-shot: the pty lives on the backend, so a dropped
 * socket is surfaced to the caller, which decides whether to re-attach and replay scrollback.
 */

/** Keep-alive cadence. Idle WebSockets are dropped by dev proxies and reverse proxies. */
const HEARTBEAT_INTERVAL_MS = 25_000

/** Control messages the backend can push on the text channel. */
export type TerminalControlFrame =
  | { type: 'ready' }
  | { type: 'exit'; code: number; error: string }
  | { type: 'error'; error: string }
  | { type: 'pong' }

export interface TerminalConnectionHandlers {
  onOutput: (chunk: Uint8Array) => void
  onControl: (frame: TerminalControlFrame) => void
  /** Fired exactly once. `local` means close() was called, `remote` means the transport went away. */
  onClosed: (origin: 'local' | 'remote') => void
}

export class TerminalConnection {
  private readonly encoder = new TextEncoder()
  private readonly handlers: TerminalConnectionHandlers
  private socket: WebSocket | null = null
  private heartbeatTimer: number | null = null
  private closedByClient = false
  private closeReported = false

  constructor(url: string, handlers: TerminalConnectionHandlers) {
    this.handlers = handlers
    const socket = new WebSocket(url)
    socket.binaryType = 'arraybuffer'
    socket.onopen = () => this.startHeartbeat()
    socket.onmessage = (event) => this.receive(event)
    socket.onerror = () => {
      console.warn('[features/agent-chat/terminal/connection.ts] terminal socket error', { url })
    }
    socket.onclose = () => this.reportClosed()
    this.socket = socket
  }

  /** Send user keystrokes. Encoded as UTF-8 binary so control characters survive untouched. */
  send(data: string) {
    if (!data || !this.isOpen()) return
    this.socket?.send(this.encoder.encode(data))
  }

  resize(cols: number, rows: number) {
    if (!this.isOpen()) return
    this.socket?.send(JSON.stringify({ type: 'resize', cols, rows }))
  }

  close() {
    this.closedByClient = true
    this.stopHeartbeat()
    const socket = this.socket
    this.socket = null
    socket?.close()
    this.reportClosed()
  }

  private isOpen() {
    return this.socket?.readyState === WebSocket.OPEN
  }

  private receive(event: MessageEvent) {
    if (event.data instanceof ArrayBuffer) {
      this.handlers.onOutput(new Uint8Array(event.data))
      return
    }
    if (typeof event.data !== 'string') return
    const frame = parseControlFrame(event.data)
    if (frame) this.handlers.onControl(frame)
  }

  private startHeartbeat() {
    this.stopHeartbeat()
    this.heartbeatTimer = window.setInterval(() => {
      if (!this.isOpen()) return
      this.socket?.send(JSON.stringify({ type: 'ping' }))
    }, HEARTBEAT_INTERVAL_MS)
  }

  private stopHeartbeat() {
    if (this.heartbeatTimer === null) return
    window.clearInterval(this.heartbeatTimer)
    this.heartbeatTimer = null
  }

  private reportClosed() {
    if (this.closeReported) return
    this.closeReported = true
    this.stopHeartbeat()
    this.handlers.onClosed(this.closedByClient ? 'local' : 'remote')
  }
}

/** Parse a control frame, dropping anything that does not match the documented contract. */
function parseControlFrame(payload: string): TerminalControlFrame | null {
  let parsed: unknown
  try {
    parsed = JSON.parse(payload)
  } catch (error) {
    console.warn('[features/agent-chat/terminal/connection.ts] unparsable terminal control frame', { payload, error })
    return null
  }
  if (!parsed || typeof parsed !== 'object') return null
  const frame = parsed as Record<string, unknown>
  switch (frame.type) {
    case 'ready':
      return { type: 'ready' }
    case 'pong':
      return { type: 'pong' }
    case 'exit':
      return {
        type: 'exit',
        code: typeof frame.code === 'number' ? frame.code : 0,
        error: typeof frame.error === 'string' ? frame.error : '',
      }
    case 'error':
      return { type: 'error', error: typeof frame.error === 'string' ? frame.error : '' }
    default:
      console.warn('[features/agent-chat/terminal/connection.ts] unknown terminal control frame', { type: frame.type })
      return null
  }
}
