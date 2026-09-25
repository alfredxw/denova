import { useState } from 'react'
import { AlertTriangle, Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { errorMessage } from '@/lib/error-diagnostics'
import { APP_VERSION } from '@/app-version'

interface InlineErrorNoticeProps {
  message: unknown
  title?: string
  className?: string
}

/** A persistent error with enough visible context for a support screenshot. */
export function InlineErrorNotice({ message, title, className = '' }: InlineErrorNoticeProps) {
  const { t } = useTranslation()
  const displayTitle = title ?? t('inlineError.defaultTitle')
  return (
    <div role="alert" className={`flex min-w-0 items-start gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-surface)] px-2.5 py-2 text-xs leading-5 text-[var(--nova-text-muted)] ${className}`}>
      <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] text-[var(--nova-danger)]">
        <AlertTriangle className="h-3.5 w-3.5" />
      </span>
      <div className="min-w-0 flex-1">
        <div className="font-medium text-[var(--nova-text)]">{displayTitle}</div>
        <ErrorDiagnostic message={errorMessage(message)} title={displayTitle} />
      </div>
    </div>
  )
}

/** Also used by error toasts and the crash boundary; no extra error store. */
export function ErrorDiagnostic({ message, title = '' }: { message: string; title?: string }) {
  const { t } = useTranslation()
  const [copyState, setCopyState] = useState<'ready' | 'copied' | 'failed'>('ready')
  const version = t('inlineError.frontend', { version: APP_VERSION })
  const copy = async () => {
    try {
      await navigator.clipboard.writeText([title, message, version].filter(Boolean).join('\n'))
      setCopyState('copied')
    } catch (error) {
      console.warn('[error-diagnostic] Copy failed', error)
      setCopyState('failed')
    }
  }
  return <div className="min-w-0 text-xs leading-5 [overflow-wrap:anywhere]">
    {message && <div className="whitespace-pre-wrap">{message}</div>}
    <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 opacity-80">
      <span>{version}</span>
      <button type="button" className="inline-flex items-center gap-1 underline underline-offset-2" onClick={() => void copy()}>
        <Copy className="h-3 w-3 shrink-0" />{t(copyState === 'copied' ? 'inlineError.copied' : 'inlineError.copy')}
      </button>
    </div>
    {copyState === 'failed' && <div>{t('inlineError.copyFailed')}</div>}
  </div>
}
