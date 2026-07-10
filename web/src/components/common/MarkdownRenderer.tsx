import ReactMarkdown from 'react-markdown'
import type { Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { cn } from '@/lib/utils'

export type MarkdownRendererComponents = Components

interface MarkdownRendererProps {
  content: string
  components?: Components
}

export function MarkdownRenderer({ content, components }: MarkdownRendererProps) {
  return (
    <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
      {content}
    </ReactMarkdown>
  )
}

interface ThemedMarkdownRendererProps extends MarkdownRendererProps {
  className?: string
}

export function ThemedMarkdownRenderer({ content, components, className }: ThemedMarkdownRendererProps) {
  return (
    <div className={cn('chat-agent-message min-w-0 text-[var(--nova-text)]', className)}>
      <MarkdownRenderer content={content} components={components} />
    </div>
  )
}
