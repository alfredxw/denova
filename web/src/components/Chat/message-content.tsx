import { Children, Fragment, cloneElement, isValidElement, memo, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ImagePreviewDialog } from '@/components/common/ImagePreviewDialog'
import { MarkdownRenderer, type MarkdownRendererComponents } from '@/components/common/MarkdownRenderer'
import { projectFileAssetURL, type ThinkingChatMessage } from '@/lib/api'
import { findDialogueHighlightRanges } from '@/lib/dialogue-highlight'
import { useBottomScrollLock } from '@/hooks/useBottomScrollLock'
import { isWorkspaceImagePath } from '@/lib/workspace-file-kind'
import { Reasoning, ReasoningContent, ReasoningTrigger } from '@/components/ai-elements/reasoning'
import { Shimmer } from '@/components/ai-elements/shimmer'
import { AgentSourceBadge } from './message-source-badge'
import { StreamingContentStage } from './StreamingContentStage'

export function StreamingPlaceholder() {
  const { t } = useTranslation()
  return (
    <div className="py-1" role="status" aria-live="polite">
      <Shimmer as="span" className="text-sm font-medium">{t('chat.activity.thinking')}</Shimmer>
    </div>
  )
}

export function sanitizeThinkTags(text: string): string {
  let result = text
  // Remove provider-internal tokens and textual tool-call residue from history;
  // current conversations execute parsed tool calls on the backend.
  result = result.replace(/\]<\]minimax\[>\[/g, '')
  result = result.replace(/<tool_call>[\s\S]*?<\/tool_call>/gi, '')
  result = result.replace(/<invoke\s+name="[^"]*"[\s\S]*?<\/invoke>/gi, '')
  // Remove paired or unclosed <think> blocks.
  result = result.replace(/<think>[\s\S]*?(?:<\/think>|$)/gi, '')
  // Some providers omit the opening tag; drop the prefix through the first close.
  const close = result.search(/<\s*\/\s*think\s*>/i)
  if (close >= 0) {
    result = result.slice(close).replace(/<\s*\/\s*think\s*>/i, '')
  }
  // Remove any remaining standalone tags.
  return result.replace(/<\/?\s*think\s*>/gi, '')
}

export const PlainTextContent = memo(function PlainTextContent({ content }: { content: string }) {
  return content
    .split(/\n[ \t]*\n+/)
    .filter((paragraph) => paragraph.length > 0)
    .map((paragraph, index) => (
      <p key={index} className="whitespace-pre-wrap break-words">{paragraph}</p>
    ))
})

export function isPlainAssistantText(content: string) {
  return !(/[\n*_`~\[\]<>#]|^\s*(?:[-+>] |\d+[.)] |-{3,}|={3,})/.test(content)
    || /\b(?:https?:\/\/|www\.)/i.test(content))
}

export const MarkdownContent = memo(function MarkdownContent({ content, highlightDialogue, projectId }: { content: string; highlightDialogue: boolean; projectId: string }) {
  const components = useMemo<MarkdownRendererComponents>(() => ({
    ...(highlightDialogue ? dialogueMarkdownComponents : markdownComponents),
    img: (props) => <ChatMarkdownImage {...props} projectId={projectId} />,
  }), [highlightDialogue, projectId])
  return (
    <MarkdownRenderer content={content} components={components} />
  )
})

const markdownComponents: MarkdownRendererComponents = {
  a: ChatMarkdownLink,
  img: ChatMarkdownImage,
}

const dialogueMarkdownComponents: MarkdownRendererComponents = {
  ...markdownComponents,
  p: ({ children }: { children?: ReactNode }) => <p>{highlightDialogueNodes(children)}</p>,
  li: ({ children }: { children?: ReactNode }) => <li>{highlightDialogueNodes(children)}</li>,
  h1: ({ children }: { children?: ReactNode }) => <h1>{highlightDialogueNodes(children)}</h1>,
  h2: ({ children }: { children?: ReactNode }) => <h2>{highlightDialogueNodes(children)}</h2>,
  h3: ({ children }: { children?: ReactNode }) => <h3>{highlightDialogueNodes(children)}</h3>,
  h4: ({ children }: { children?: ReactNode }) => <h4>{highlightDialogueNodes(children)}</h4>,
  h5: ({ children }: { children?: ReactNode }) => <h5>{highlightDialogueNodes(children)}</h5>,
  h6: ({ children }: { children?: ReactNode }) => <h6>{highlightDialogueNodes(children)}</h6>,
  blockquote: ({ children }: { children?: ReactNode }) => <blockquote>{highlightDialogueNodes(children)}</blockquote>,
}

function ChatMarkdownLink({ href, children }: { href?: string; title?: string; children?: ReactNode }) {
  const external = /^https?:\/\//i.test(href?.trim() || '')
  return (
    <a
      href={href}
      {...(external ? { target: '_blank', rel: 'noopener noreferrer' } : {})}
    >
      {children}
    </a>
  )
}

function ChatMarkdownImage({ src = '', alt = '', title = '', projectId = '' }: { src?: string; alt?: string; title?: string; projectId?: string }) {
  const { t } = useTranslation()
  const imageSrc = normalizeChatImageSrc(src, projectId)
  if (!imageSrc) return null
  const imageTitle = alt || title || t('chat.image.previewTitle')
  const imagePath = shouldShowImagePath(src) ? src : undefined

  return (
    <ImagePreviewDialog src={imageSrc} title={imageTitle} alt={alt || imageTitle} path={imagePath}>
      <button type="button" className="nova-chat-image-button" aria-label={t('chat.image.openPreview')}>
        <img src={imageSrc} alt={alt || imageTitle} loading="lazy" />
      </button>
    </ImagePreviewDialog>
  )
}

function normalizeChatImageSrc(src: string, projectId: string) {
  const trimmed = src.trim()
  if (!trimmed) return ''
  if (/^(https?:|data:|blob:|\/)/i.test(trimmed)) return trimmed
  if (isWorkspaceImagePath(trimmed)) return chatAssetURL(projectId, trimmed)
  return trimmed
}

export function chatAssetURL(projectId: string, path: string) {
  return projectId ? projectFileAssetURL(projectId, path) : ''
}

function shouldShowImagePath(src: string) {
  const trimmed = src.trim()
  return Boolean(trimmed && !/^(data:|blob:)/i.test(trimmed))
}

function highlightDialogueNodes(children: ReactNode): ReactNode {
  return Children.map(children, (child, index) => {
    if (typeof child === 'string') return highlightDialogueText(child, true, `md-${index}`)
    if (!isValidElement(child)) return child
    const props = child.props as { children?: ReactNode }
    if (props.children === undefined) return child
    return cloneElement(child, undefined, highlightDialogueNodes(props.children))
  })
}

function highlightDialogueText(text: string, enabled: boolean, keyPrefix: string): ReactNode {
  if (!enabled || !text) return text
  const nodes: ReactNode[] = []
  const ranges = findDialogueHighlightRanges(text)
  let lastIndex = 0

  ranges.forEach((range, index) => {
    if (range.from > lastIndex) nodes.push(text.slice(lastIndex, range.from))
    nodes.push(
      <span key={`${keyPrefix}-dialogue-${index}`} className="nova-dialogue-highlight">
        {text.slice(range.from, range.to)}
      </span>,
    )
    lastIndex = range.to
  })

  if (lastIndex < text.length) nodes.push(text.slice(lastIndex))
  if (nodes.length === 0) return text
  return <Fragment>{nodes}</Fragment>
}

/** Reasoning follows streaming until the user takes explicit control. */
export function ThinkingBlock({ message, content, streaming }: { message: ThinkingChatMessage; content: string; streaming: boolean }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(streaming)
  const userToggledRef = useRef(false)
  const wasStreamingRef = useRef(streaming)
  const contentScrollLock = useBottomScrollLock<HTMLDivElement>({
    enabled: streaming && expanded,
    resetKey: `${message.id || message.created_at || 'thinking'}:thinking-stream`,
    contentKey: `${expanded ? 'expanded' : 'collapsed'}:${content.length}`,
  })

  useLayoutEffect(() => {
    const wasStreaming = wasStreamingRef.current
    wasStreamingRef.current = streaming
    if (!wasStreaming && streaming) {
      userToggledRef.current = false
      setExpanded(true)
    } else if (wasStreaming && !streaming && !userToggledRef.current) {
      setExpanded(false)
    }
  }, [streaming])

  const handleOpenChange = (open: boolean) => {
    userToggledRef.current = true
    setExpanded(open)
  }

  return (
    <div className="flex justify-start">
      <div className="w-full">
        <Reasoning isStreaming={streaming} open={expanded} onOpenChange={handleOpenChange} className="mb-0">
          <ReasoningTrigger className="flex items-center gap-1 py-1 text-xs text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]">
            {expanded ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
            {streaming ? (
              <Shimmer as="span" className="text-xs font-medium">{t('chat.activity.thinking')}</Shimmer>
            ) : (
              <span>{t('chat.trace.thinking')}</span>
            )}
            {message.subagent && <AgentSourceBadge message={message} compact />}
          </ReasoningTrigger>
          <ReasoningContent className="mt-0 text-xs">
            <div data-thinking-scroll-frame className="overflow-hidden rounded-md border border-border/60">
              <div
                ref={contentScrollLock.ref}
                role="region"
                tabIndex={0}
                aria-label={t('chat.trace.thinkingContent')}
                data-nova-scroll-lock="thinking"
                onScroll={contentScrollLock.onScroll}
                onWheel={contentScrollLock.onWheel}
                onKeyDown={contentScrollLock.onKeyDown}
                className="scroll-fade-y scroll-fade-8 max-h-40 overflow-x-hidden overflow-y-auto overscroll-contain px-2.5 py-2 text-xs leading-4 text-[var(--nova-text-muted)] whitespace-pre-wrap break-words [overflow-anchor:none] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-[var(--nova-accent)]"
              >
                <StreamingContentStage content={content} targetContent={streaming ? message.streaming_target_content : undefined} streaming={streaming}>
                  {(value) => value}
                </StreamingContentStage>
              </div>
            </div>
          </ReasoningContent>
        </Reasoning>
      </div>
    </div>
  )
}
