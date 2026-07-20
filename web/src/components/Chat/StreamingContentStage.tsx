import { useLayoutEffect, useState } from 'react'
import type { ReactNode } from 'react'

interface StreamingContentStageProps {
  content: string
  targetContent?: string
  streaming: boolean
  children: (content: string) => ReactNode
}

/** Reserves the target layout before replacing the currently visible stream content. */
export function StreamingContentStage({ content, targetContent, streaming, children }: StreamingContentStageProps) {
  const nextContent = streaming && targetContent !== undefined ? targetContent : content
  const [revealedContent, setRevealedContent] = useState(content)
  const [preparedTargetContent, setPreparedTargetContent] = useState<string | undefined>(undefined)
  const visibleContent = streaming && targetContent === undefined && preparedTargetContent === content
    ? content
    : revealedContent
  const canStage = streaming && nextContent !== visibleContent && nextContent.startsWith(visibleContent)

  useLayoutEffect(() => {
    if (streaming && targetContent !== undefined && preparedTargetContent !== targetContent) {
      setPreparedTargetContent(targetContent)
      return
    }
    if (!canStage) {
      setRevealedContent((current) => current === nextContent ? current : nextContent)
      if (targetContent === undefined && preparedTargetContent !== undefined) {
        setPreparedTargetContent(undefined)
      }
      return
    }
    const frameID = requestAnimationFrame(() => setRevealedContent(nextContent))
    return () => cancelAnimationFrame(frameID)
  }, [canStage, nextContent, preparedTargetContent, streaming, targetContent])

  if (!canStage) return children(nextContent)

  return (
    <div className="nova-streaming-content-stage">
      <div className="nova-streaming-content-reserve" aria-hidden="true">
        {children(nextContent)}
      </div>
      <div className="nova-streaming-content-overlay">
        {children(visibleContent)}
      </div>
    </div>
  )
}
