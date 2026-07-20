import { useEffect, useRef, useState } from 'react'
import { Check, Copy, Loader2, TriangleAlert } from 'lucide-react'
import { Button } from '@/components/ui/button'

interface ContextCopyButtonProps {
  content: string
  label: string
  copiedLabel: string
  failedLabel: string
  showLabel?: boolean
}

type CopyState = 'idle' | 'copying' | 'copied' | 'failed'

const CLIPBOARD_WRITE_TIMEOUT_MS = 2000

/** Copies bounded context diagnostics with feedback and an embedded-browser fallback. */
export function ContextCopyButton({ content, label, copiedLabel, failedLabel, showLabel = false }: ContextCopyButtonProps) {
  const [state, setState] = useState<CopyState>('idle')
  const resetTimerRef = useRef<number | null>(null)

  useEffect(() => () => {
    if (resetTimerRef.current !== null) window.clearTimeout(resetTimerRef.current)
  }, [])

  const handleCopy = async () => {
    setState('copying')
    try {
      await writeContextText(content)
      setState('copied')
    } catch (error) {
      console.error('[context-analysis] failed to copy context content', error)
      setState('failed')
    }
    if (resetTimerRef.current !== null) window.clearTimeout(resetTimerRef.current)
    resetTimerRef.current = window.setTimeout(() => setState('idle'), 1800)
  }

  const currentLabel = state === 'copying' ? `${label}…` : state === 'copied' ? copiedLabel : state === 'failed' ? failedLabel : label
  const Icon = state === 'copying' ? Loader2 : state === 'copied' ? Check : state === 'failed' ? TriangleAlert : Copy
  return (
    <Button
      type="button"
      variant="ghost"
      size={showLabel ? 'xs' : 'icon-xs'}
      disabled={!content || state === 'copying'}
      aria-label={currentLabel}
      title={currentLabel}
      className={showLabel ? 'h-7 shrink-0 gap-1.5 px-2 text-[11px]' : 'size-7 shrink-0 text-[var(--nova-text-faint)]'}
      onClick={() => void handleCopy()}
    >
      <Icon className={state === 'copying' ? 'size-3.5 animate-spin' : 'size-3.5'} />
      {showLabel ? currentLabel : null}
    </Button>
  )
}

async function writeContextText(content: string) {
  if (writeContextTextLegacy(content)) return
  if (navigator.clipboard?.writeText) {
    let timeoutID: number | undefined
    try {
      await Promise.race([
        navigator.clipboard.writeText(content),
        new Promise<never>((_, reject) => {
          timeoutID = window.setTimeout(() => reject(new Error('Clipboard write timed out')), CLIPBOARD_WRITE_TIMEOUT_MS)
        }),
      ])
    } finally {
      if (timeoutID !== undefined) window.clearTimeout(timeoutID)
    }
    return
  }
  throw new Error('Clipboard API unavailable')
}

function writeContextTextLegacy(content: string) {
  if (typeof document.execCommand !== 'function') return false
  const textarea = document.createElement('textarea')
  textarea.value = content
  textarea.setAttribute('readonly', 'true')
  textarea.style.position = 'fixed'
  textarea.style.left = '-9999px'
  textarea.style.top = '0'
  document.body.appendChild(textarea)
  textarea.select()
  try {
    return document.execCommand('copy')
  } finally {
    document.body.removeChild(textarea)
  }
}
