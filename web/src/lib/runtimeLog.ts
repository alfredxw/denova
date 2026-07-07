const RUNTIME_LOG_KEY = 'nova.runtime.logs'
const MAX_LOGS = 80

type RuntimeLogType = 'react_error' | 'window_error' | 'unhandled_rejection' | 'white_screen' | 'startup'

export interface RuntimeLogEntry {
  type: RuntimeLogType
  message: string
  reason: string
  stack?: string
  componentStack?: string
  url: string
  userAgent: string
  timestamp: string
}

/** 记录前端运行时异常，写入控制台和 localStorage，便于排查崩溃/白屏原因。 */
export function recordRuntimeLog(entry: Omit<RuntimeLogEntry, 'url' | 'userAgent' | 'timestamp'>) {
  const fullEntry: RuntimeLogEntry = {
    ...entry,
    url: window.location.href,
    userAgent: window.navigator.userAgent,
    timestamp: new Date().toISOString(),
  }
  console.error('[nova-runtime]', fullEntry)
  try {
    const prev = readRuntimeLogs()
    const next = [...prev, fullEntry].slice(-MAX_LOGS)
    window.localStorage.setItem(RUNTIME_LOG_KEY, JSON.stringify(next))
  } catch (error) {
    console.error('[nova-runtime] 写入本地日志失败', error)
  }
}

/** 读取最近的前端运行时日志。 */
function readRuntimeLogs(): RuntimeLogEntry[] {
  try {
    const raw = window.localStorage.getItem(RUNTIME_LOG_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw) as RuntimeLogEntry[]
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

/** 将未知错误对象转换为可读信息。 */
export function normalizeRuntimeError(error: unknown) {
  if (error instanceof Error) {
    return {
      message: error.message || error.name,
      stack: error.stack,
    }
  }
  if (typeof error === 'string') {
    return { message: error }
  }
  try {
    return { message: JSON.stringify(error) }
  } catch {
    return { message: String(error) }
  }
}

/** 注册全局 JS 异常和 Promise 异常监听。 */
export function installGlobalRuntimeLoggers() {
  window.addEventListener('error', event => {
    const normalized = normalizeRuntimeError(event.error || event.message)
    recordRuntimeLog({
      type: 'window_error',
      message: normalized.message,
      reason: `${event.filename || 'unknown'}:${event.lineno || 0}:${event.colno || 0}`,
      stack: normalized.stack,
    })
  })

  window.addEventListener('unhandledrejection', event => {
    const normalized = normalizeRuntimeError(event.reason)
    recordRuntimeLog({
      type: 'unhandled_rejection',
      message: normalized.message,
      reason: 'Promise 未处理异常',
      stack: normalized.stack,
    })
  })
}

/** 延迟检测白屏：root 为空、不可见或 App shell 未挂载都会记录原因。 */
export function scheduleWhiteScreenCheck(root: HTMLElement | null) {
  window.setTimeout(() => {
    const reason = detectWhiteScreenReason(root)
    if (!reason) return
    recordRuntimeLog({
      type: 'white_screen',
      message: '检测到前端白屏',
      reason,
    })
  }, 3000)
}

function detectWhiteScreenReason(root: HTMLElement | null) {
  if (!root) return 'root 节点不存在'
  if (!root.innerHTML.trim()) return 'root 节点为空，React 可能未成功挂载'
  const rect = root.getBoundingClientRect()
  if (rect.width === 0 || rect.height === 0) return `root 尺寸异常 width=${rect.width} height=${rect.height}`
  const style = window.getComputedStyle(root)
  if (style.display === 'none' || style.visibility === 'hidden' || style.opacity === '0') {
    return `root 不可见 display=${style.display} visibility=${style.visibility} opacity=${style.opacity}`
  }
  if (!root.querySelector('[data-nova-app-shell="true"]')) return 'App shell 未挂载或渲染中断'
  return ''
}
