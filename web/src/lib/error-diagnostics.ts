import i18n from '@/i18n'

/** Only diagnostic fields are rendered. Never stringify a server payload: it
 * may also contain document snapshots or credentials needed by its caller. */
export function errorMessage(source: unknown, fallback = i18n.t('inlineError.unexpected')): string {
  if (typeof source === 'string') return diagnosticText(source) || fallback
  if (!source || typeof source !== 'object') return fallback
  const value = source as Record<string, unknown>
  const details = value.details && typeof value.details === 'object' ? value.details as Record<string, unknown> : {}
  const text = (input: unknown) => typeof input === 'string' ? diagnosticText(input).trim() : ''
  const summary = text(value.summary) || text(value.message) || text(value.error) || text(value.content) || fallback
  const lines = [summary]
  const add = (key: string, input: unknown) => {
    const detail = text(input)
    if (detail && !summary.includes(detail)) lines.push(`${i18n.t(key)}: ${detail}`)
  }
  add('inlineError.cause', details.detail || details.reason)
  add('inlineError.operation', details.operation)
  add('inlineError.code', value.code || (value.name !== 'Error' ? value.name : undefined))
  if (typeof value.status === 'number' && value.status > 0) add('inlineError.httpStatus', String(value.status))
  add('common.logId', value.requestID || value.request_id)
  add('inlineError.run', value.run_id || value.operation_id || value.task_id || details.target_operation_id)
  add('inlineError.backend', [text(details.backend_version), text(details.platform)].filter(Boolean).join(' · '))
  return lines.join('\n')
}

// Match the server's bounded diagnostic projection for browser-only errors and
// older servers. This is a final display guard, not a substitute for typed data.
export function diagnosticText(value: string): string {
  const sanitized = value
    .replace(/https?:\/\/[^\s<>"']+/g, raw => {
      try { const url = new URL(raw); url.username = ''; url.password = ''; url.search = ''; url.hash = ''; return url.toString() }
      catch { return '[url]' }
    })
    .replace(/(authorization\s*[:=]\s*)(?:bearer|basic)\s+[^\s,;"']+/gi, '$1[redacted]')
    .replace(/(["']?(?:authorization|api[_-]?key|access[_-]?token|refresh[_-]?token|token|password|secret|cookie)["']?\s*[:=]\s*)(?:"[^"\n]*"|'[^'\n]*'|[^\s,;}]+)/gi, '$1[redacted]')
    .replace(/(["']?(?:prompt|messages|request_body|response_body|body)["']?\s*[:=]\s*)[\s\S]*/gi, '$1[redacted]')
    .replace(/(?:[A-Za-z]:\\|\/(?:Users|home|private|var|tmp)\/)[^\s"':;]+/g, '[local-path]')
  return sanitized.length > 4096 ? `${sanitized.slice(0, 4096)}…` : sanitized
}
