import { useEffect, useState } from 'react'
import { parsePartialJson } from 'ai'
import { CodeBlockContainer } from '@/components/ai-elements/code-block'
import { useBottomScrollLock } from '@/hooks/useBottomScrollLock'

interface StreamingToolInputProps {
  rawInput: string
  streamKey: string
}

/** Presents a readable projection without changing the execution-authoritative raw input. */
export function StreamingToolInput({ rawInput, streamKey }: StreamingToolInputProps) {
  const [displayInput, setDisplayInput] = useState(rawInput)
  const scrollLock = useBottomScrollLock<HTMLPreElement>({
    enabled: true,
    resetKey: `${streamKey}:tool-input-stream`,
    contentKey: rawInput.length,
  })

  useEffect(() => {
    let cancelled = false

    void parsePartialJson(rawInput).then(({ value }) => {
      if (cancelled) return
      const formattedInput = value === undefined ? rawInput : JSON.stringify(value, null, 2)
      setDisplayInput(formattedInput ?? rawInput)
    }, () => {
      if (!cancelled) setDisplayInput(rawInput)
    })

    return () => {
      cancelled = true
    }
  }, [rawInput])

  return (
    <CodeBlockContainer
      language="json"
      className="rounded-none border-x-0 border-b-0 border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-accent-green)]"
    >
      <pre
        ref={scrollLock.ref}
        onScroll={scrollLock.onScroll}
        onWheel={scrollLock.onWheel}
        onKeyDown={scrollLock.onKeyDown}
        aria-busy="true"
        data-nova-scroll-lock="tool-input-stream"
        data-nova-tool-input-stream
        className="m-0 min-w-0 max-w-full max-h-32 overflow-x-hidden overflow-y-auto px-3 py-2.5 font-mono text-[11px] leading-relaxed whitespace-pre-wrap [overflow-anchor:none] [overflow-wrap:anywhere]"
      >
        {displayInput}
      </pre>
    </CodeBlockContainer>
  )
}
