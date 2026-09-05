import { useEffect, useRef, useState } from 'react'
import { ChevronLeft, ChevronRight, ImagePlus, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ImagePreviewDialog } from '@/components/common/ImagePreviewDialog'
import { ContextCopyButton } from './ContextCopyButton'
import type {
  AssistantChatMessage,
  ChapterIllustration,
  InteractiveImage,
  InteractiveImageError,
  ToolCallChatMessage,
  ToolResultChatMessage,
} from '@/lib/api'
import { chatAssetURL } from './message-content'
import { ToolStatusIcon } from './message-tool-status'

export function ChapterIllustrationBlock({ projectId, message, onInsert }: { projectId: string; message: ToolCallChatMessage | ToolResultChatMessage; onInsert?: (illustration: ChapterIllustration) => void }) {
  const { t } = useTranslation()
  const illustration = message.illustration
  if (!illustration) return null

  const status = message.status || 'running'
  const isMarkdownChapter = isMarkdownPath(illustration.chapter_path)
  const canInsert = status === 'success' && isMarkdownChapter && Boolean(onInsert)
  const canCopyReference = status === 'success' && isMarkdownChapter && Boolean(illustration.markdown)
  const imageSrc = chatAssetURL(projectId, illustration.image_path)
  const imageTitle = illustration.alt_text || t('chat.illustration.previewAlt')

  return (
    <div className="flex justify-start">
      <div className="w-full overflow-hidden rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] text-xs shadow-[var(--nova-shadow)]">
        <ImagePreviewDialog src={imageSrc} title={imageTitle} alt={imageTitle} path={illustration.image_path}>
          <button
            type="button"
            className="group relative block w-full overflow-hidden bg-black/90 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--nova-accent)]"
            aria-label={t('chat.illustration.openPreview')}
          >
            <img
              src={imageSrc}
              alt={imageTitle}
              className="max-h-80 w-full object-contain"
              loading="lazy"
            />
            <span className="absolute left-3 top-3 inline-flex items-center gap-1.5 rounded-md border border-white/15 bg-black/45 px-2 py-1 text-[11px] font-medium text-white opacity-90 backdrop-blur">
              <ToolStatusIcon status={status} />
              {t('chat.illustration.title')}
            </span>
          </button>
        </ImagePreviewDialog>
        <div className="flex min-w-0 flex-col gap-2 border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 sm:flex-row sm:items-center">
          <code className="min-w-0 flex-1 truncate rounded border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1 font-mono text-[10px] text-[var(--nova-text-muted)]">
            {illustration.image_path}
          </code>
          <div className="flex min-w-0 items-center justify-end gap-2">
            {!isMarkdownChapter && (
              <span className="min-w-0 truncate text-[11px] text-[var(--nova-text-faint)]">{t('chat.illustration.markdownOnly')}</span>
            )}
            {onInsert ? (
              <button
                type="button"
                disabled={!canInsert}
                onClick={() => onInsert(illustration)}
                className="inline-flex h-7 shrink-0 items-center gap-1 rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[11px] font-medium text-[var(--nova-text-muted)] transition hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:cursor-not-allowed disabled:opacity-45"
              >
                <ImagePlus className="h-3.5 w-3.5" />
                {status === 'running' ? t('chat.illustration.generating') : t('chat.illustration.insert')}
              </button>
            ) : (
              <ContextCopyButton
                content={canCopyReference ? illustration.markdown : ''}
                label={status === 'running' ? t('chat.illustration.generating') : t('chat.illustration.copyReference')}
                copiedLabel={t('chat.illustration.referenceCopied')}
                failedLabel={t('chat.illustration.referenceCopyFailed')}
                showLabel
              />
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

export function InteractiveImageBlock({ projectId, message }: { projectId: string; message: InteractiveImageMessage }) {
  return (
    <div className="flex justify-start">
      <div className="w-full">
        <InteractiveImageStrip message={message} projectId={projectId} />
      </div>
    </div>
  )
}

type InteractiveImageMessage = AssistantChatMessage | ToolCallChatMessage | ToolResultChatMessage

export function InteractiveImageStrip({ message, projectId }: { message: InteractiveImageMessage; projectId: string }) {
  const { t } = useTranslation()
  const images = interactiveImagesFromMessage(message)
  const error = message.interactive_image_error || readInteractiveImageErrorFromMessage(message)
  const status = message.interactive_image_status || (message.role === 'assistant' ? undefined : message.status)
  const [index, setIndex] = useState(Math.max(0, images.length - 1))
  const previousImageCountRef = useRef(images.length)

  useEffect(() => {
    const previousLength = previousImageCountRef.current
    previousImageCountRef.current = images.length
    setIndex((current) => {
      if (images.length > previousLength) return images.length - 1
      return Math.min(Math.max(0, images.length - 1), Math.max(0, current))
    })
  }, [images.length])

  if (images.length === 0) {
    if (status === 'running') {
      return (
        <div className="mt-3 flex items-center gap-2 px-1 text-xs text-[var(--nova-text-faint)]">
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
          <span>{t('chat.interactiveImage.generating')}</span>
        </div>
      )
    }
    if (error) {
      return (
        <div className="mt-3 rounded-md border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs text-[var(--nova-danger)]">
          {error.message || t('chat.interactiveImage.failed')}
        </div>
      )
    }
    return null
  }

  const safeIndex = Math.min(index, images.length - 1)
  const image = images[safeIndex]
  const title = image.alt_text || t('chat.interactiveImage.previewAlt')
  const src = chatAssetURL(projectId, image.image_path)
  const canSwitch = images.length > 1

  return (
    <div className="mt-3 max-w-full">
      <ImagePreviewDialog src={src} title={title} alt={title} path={image.image_path}>
        <div
          role="button"
          tabIndex={0}
          className="group relative block w-full overflow-hidden rounded-lg border border-[var(--nova-border)] bg-black/90 text-left shadow-[var(--nova-shadow)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--nova-accent)]"
          aria-label={t('chat.interactiveImage.openPreview')}
        >
          <img
            src={src}
            alt={title}
            className="max-h-[440px] w-full object-contain"
            loading="lazy"
          />
          {canSwitch && (
            <span className="absolute bottom-2 right-2 inline-flex items-center gap-1 rounded-md border border-white/10 bg-black/35 px-1 py-0.5 text-[10px] text-white/70 opacity-45 backdrop-blur transition group-hover:opacity-90">
              <button
                type="button"
                aria-label={t('chat.interactiveImage.prevVersion')}
                className={`flex h-5 w-5 items-center justify-center rounded border border-transparent ${safeIndex <= 0 ? 'opacity-30' : 'hover:border-white/15 hover:bg-white/10'}`}
                disabled={safeIndex <= 0}
                onClick={(event) => {
                  event.preventDefault()
                  event.stopPropagation()
                  setIndex((current) => Math.max(0, current - 1))
                }}
              >
                <ChevronLeft className="h-3 w-3" />
              </button>
              <span className="min-w-7 text-center font-mono leading-5">{safeIndex + 1}/{images.length}</span>
              <button
                type="button"
                aria-label={t('chat.interactiveImage.nextVersion')}
                className={`flex h-5 w-5 items-center justify-center rounded border border-transparent ${safeIndex >= images.length - 1 ? 'opacity-30' : 'hover:border-white/15 hover:bg-white/10'}`}
                disabled={safeIndex >= images.length - 1}
                onClick={(event) => {
                  event.preventDefault()
                  event.stopPropagation()
                  setIndex((current) => Math.min(images.length - 1, current + 1))
                }}
              >
                <ChevronRight className="h-3 w-3" />
              </button>
            </span>
          )}
        </div>
      </ImagePreviewDialog>
    </div>
  )
}

function interactiveImagesFromMessage(message: InteractiveImageMessage): InteractiveImage[] {
  if (message.interactive_images?.length) return message.interactive_images.filter((image) => Boolean(image.image_path))
  const image = message.interactive_image?.image_path ? message.interactive_image : readInteractiveImageFromMessage(message)
  return image?.image_path ? [image] : []
}

function readInteractiveImageFromMessage(message: InteractiveImageMessage): InteractiveImage | undefined {
  if (message.interactive_image?.image_path) return message.interactive_image
  const data = parseMessageResult(message.role === 'assistant' ? undefined : message.result)
  if (isInteractiveImage(data)) return data
  return undefined
}

function readInteractiveImageErrorFromMessage(message: InteractiveImageMessage): InteractiveImageError | undefined {
  if (message.interactive_image_error) return message.interactive_image_error
  const data = parseMessageResult(message.role === 'assistant' ? undefined : message.result)
  if (isInteractiveImageError(data)) return data
  return undefined
}

function parseMessageResult(result?: string): unknown {
  if (!result) return null
  try {
    return JSON.parse(result)
  } catch {
    return null
  }
}

function isInteractiveImage(value: unknown): value is InteractiveImage {
  if (!value || typeof value !== 'object') return false
  const data = value as Record<string, unknown>
  return data.schema === 'interactive_image.v1' && typeof data.image_path === 'string' && Boolean(data.image_path)
}

function isInteractiveImageError(value: unknown): value is InteractiveImageError {
  if (!value || typeof value !== 'object') return false
  const data = value as Record<string, unknown>
  return data.schema === 'interactive_image_error.v1'
}

function isMarkdownPath(path?: string) {
  return /\.(md|markdown)$/i.test(path || '')
}
