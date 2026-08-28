import { Children, Fragment, cloneElement, isValidElement, memo, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { cjk } from '@streamdown/cjk'
import { math } from '@streamdown/math'
import { ChevronDown, ChevronRight } from 'lucide-react'
import {
  Streamdown,
  defaultRehypePlugins,
  type Components as StreamdownComponents,
  type ControlsConfig,
  type StreamdownTranslations,
} from 'streamdown'
import { useTranslation } from 'react-i18next'
import { ImagePreviewDialog } from '@/components/common/ImagePreviewDialog'
import { projectFileAssetURL, type ThinkingChatMessage } from '@/lib/api'
import { findDialogueHighlightRanges } from '@/lib/dialogue-highlight'
import { useBottomScrollLock } from '@/hooks/useBottomScrollLock'
import { isWorkspaceImagePath } from '@/lib/workspace-file-kind'
import { Reasoning, ReasoningContent, ReasoningTrigger } from '@/components/ai-elements/reasoning'
import { agentContentPreview } from './agent-content-preview'
import { AgentSourceBadge } from './message-source-badge'
import { StreamingContentStage } from './StreamingContentStage'

const chatMarkdownPlugins = { cjk, math }
const chatMarkdownControls = {
  code: { copy: true, download: false },
  table: { copy: true, download: false, fullscreen: true },
  image: false,
  mermaid: false,
} satisfies ControlsConfig
// Keep Streamdown's HTML sanitization without its final URL hardener: that
// hardener replaces Denova's bare workspace image paths before our image
// component can resolve them through the authenticated Project asset API.
const chatMarkdownRehypePlugins = [
  defaultRehypePlugins.raw,
  defaultRehypePlugins.sanitize,
]

interface MarkdownContentProps {
  content: string
  highlightDialogue: boolean
  projectId: string
  streaming?: boolean
}

interface ChatMarkdownImageProps {
  src?: string
  alt?: string
  title?: string
  projectId?: string
}

export function StreamingPlaceholder() {
  const { t } = useTranslation()
  return (
    <div className="py-1" role="status" aria-live="polite">
      <span className="shimmer text-sm font-medium [--shimmer-color:var(--nova-text-faint)]">{t('chat.activity.thinking')}</span>
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

export const MarkdownContent = memo(function MarkdownContent({
  content,
  highlightDialogue,
  projectId,
  streaming = false,
}: MarkdownContentProps) {
  const { t } = useTranslation()
  const components = useMemo(() => ({
    ...(highlightDialogue ? dialogueMarkdownComponents : markdownComponents),
    img: (props: ChatMarkdownImageProps) => <ChatMarkdownImage {...props} projectId={projectId} />,
  }) as StreamdownComponents, [highlightDialogue, projectId])
  const translations = useMemo<Partial<StreamdownTranslations>>(() => ({
    copied: t('chat.action.copyMessageDone'),
    copyCode: t('chat.markdown.copyCode'),
    copyTable: t('chat.markdown.copyTable'),
    copyTableAsCsv: t('chat.markdown.copyTableAsCsv'),
    copyTableAsMarkdown: t('chat.markdown.copyTableAsMarkdown'),
    copyTableAsTsv: t('chat.markdown.copyTableAsTsv'),
    exitFullscreen: t('chat.markdown.exitFullscreen'),
    viewFullscreen: t('chat.markdown.viewFullscreen'),
  }), [t])
  return (
    <Streamdown
      animated
      className="nova-streamdown min-w-0 space-y-2"
      components={components}
      controls={chatMarkdownControls}
      isAnimating={streaming}
      lineNumbers={false}
      mode="streaming"
      plugins={chatMarkdownPlugins}
      rehypePlugins={chatMarkdownRehypePlugins}
      translations={translations}
    >
      {content}
    </Streamdown>
  )
})

const markdownComponents = {
  a: ChatMarkdownLink,
  img: ChatMarkdownImage,
}

const dialogueMarkdownComponents = {
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

function ChatMarkdownImage({ src = '', alt = '', title = '', projectId = '' }: ChatMarkdownImageProps) {
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
  let workspacePath = trimmed
  try {
    // Markdown normalizes non-ASCII destinations to encoded URIs. Decode that
    // representation once before URLSearchParams encodes the API query value.
    workspacePath = decodeURIComponent(trimmed)
  } catch {
    // Invalid percent sequences can be literal filename characters.
  }
  if (isWorkspaceImagePath(workspacePath)) return chatAssetURL(projectId, workspacePath)
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
export function ThinkingBlock({ projectId, message, content, streaming, showAgentSource = true }: { projectId: string; message: ThinkingChatMessage; content: string; streaming: boolean; showAgentSource?: boolean }) {
  const { t } = useTranslation()
  const preview = agentContentPreview(content)
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
          <ReasoningTrigger
            aria-label={t(expanded ? 'chat.trace.collapseThinking' : 'chat.trace.expandThinking')}
            className="flex min-w-0 items-center gap-1 py-1 text-xs text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]"
          >
            {streaming && !preview ? (
              <span className="shimmer shrink-0 text-xs font-medium">{t('chat.activity.thinking')}</span>
            ) : streaming ? (
              <span className="shimmer shrink-0 text-xs font-medium">{t('chat.trace.thinking')}</span>
            ) : (
              <span className="shrink-0">{t('chat.trace.thinking')}</span>
            )}
            {preview ? (
              <>
                <span aria-hidden="true" className="shrink-0">·</span>
                <span data-thinking-preview className="min-w-0 flex-1 truncate text-left">{preview}</span>
              </>
            ) : <span className="flex-1" />}
            {showAgentSource && message.subagent && <AgentSourceBadge message={message} compact />}
            {expanded ? <ChevronDown aria-hidden="true" className="size-3 shrink-0" /> : <ChevronRight aria-hidden="true" className="size-3 shrink-0" />}
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
                className="scroll-fade-y scroll-fade-8 max-h-80 overflow-x-hidden overflow-y-auto overscroll-contain px-2.5 py-2 text-xs leading-4 text-[var(--nova-text-muted)] whitespace-normal break-words [overflow-anchor:none] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-[var(--nova-accent)]"
              >
                <StreamingContentStage content={content} targetContent={streaming ? message.streaming_target_content : undefined} streaming={streaming}>
                  {(value) => (
                    <MarkdownContent
                      content={value}
                      highlightDialogue={false}
                      projectId={projectId}
                      streaming={streaming}
                    />
                  )}
                </StreamingContentStage>
              </div>
            </div>
          </ReasoningContent>
        </Reasoning>
      </div>
    </div>
  )
}
