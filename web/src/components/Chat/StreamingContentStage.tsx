import type { ReactNode } from 'react'

interface StreamingContentStageProps {
  content: string
  targetContent?: string
  streaming: boolean
  children: (content: string) => ReactNode
}

/** Renders the latest complete streaming snapshot as a single content tree. */
export function StreamingContentStage({ content, targetContent, streaming, children }: StreamingContentStageProps) {
  return children(streaming && targetContent !== undefined ? targetContent : content)
}
