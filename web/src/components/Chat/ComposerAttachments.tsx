import { useCallback, useEffect, useRef, useState, type ClipboardEvent, type DragEvent } from 'react'
import { Paperclip } from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import {
  Attachment,
  AttachmentInfo,
  AttachmentPreview,
  AttachmentRemove,
  Attachments,
  type AttachmentData,
} from '@/components/ai-elements/attachments'
import {
  MAX_CHAT_ATTACHMENT_BYTES,
  MAX_CHAT_ATTACHMENT_FILES,
  MAX_CHAT_ATTACHMENTS_BYTES,
  type ChatAttachmentDescriptor,
} from '@/lib/chat-attachments'

interface ComposerAttachment {
  id: string
  file: File
  previewURL: string
}

const attachmentDrafts = new Map<string, File[]>()
const MAX_ATTACHMENT_DRAFTS = 8

export function useComposerAttachments(enabled = true, draftKey?: string) {
  const { t } = useTranslation()
  const normalizedDraftKey = draftKey?.trim() || ''
  const [items, setItems] = useState<ComposerAttachment[]>(() => draftItems(normalizedDraftKey))
  const inputRef = useRef<HTMLInputElement>(null)
  const itemsRef = useRef(items)
  const draftKeyRef = useRef(normalizedDraftKey)

  useEffect(() => () => {
    for (const item of itemsRef.current) URL.revokeObjectURL(item.previewURL)
  }, [])

  useEffect(() => {
    if (draftKeyRef.current === normalizedDraftKey) return
    for (const item of itemsRef.current) URL.revokeObjectURL(item.previewURL)
    const next = draftItems(normalizedDraftKey)
    draftKeyRef.current = normalizedDraftKey
    itemsRef.current = next
    setItems(next)
    if (inputRef.current) inputRef.current.value = ''
  }, [normalizedDraftKey])

  const addFiles = useCallback((files: File[]) => {
    if (!enabled || files.length === 0) return
    const next = [...itemsRef.current]
    let totalBytes = next.reduce((sum, item) => sum + item.file.size, 0)
    const known = new Set(next.map(({ file }) => file))
    let rejectedForCount = false
    let rejectedForTotal = false
    for (const file of files) {
      if (known.has(file)) continue
      if (file.size > MAX_CHAT_ATTACHMENT_BYTES) {
        toast.error(t('chat.attachment.fileTooLarge', { name: file.name, maximum: formatBytes(MAX_CHAT_ATTACHMENT_BYTES) }))
        continue
      }
      if (next.length >= MAX_CHAT_ATTACHMENT_FILES) {
        rejectedForCount = true
        continue
      }
      if (totalBytes + file.size > MAX_CHAT_ATTACHMENTS_BYTES) {
        rejectedForTotal = true
        continue
      }
      known.add(file)
      totalBytes += file.size
      next.push({
        id: attachmentItemID(next.length),
        file,
        previewURL: URL.createObjectURL(file),
      })
    }
    itemsRef.current = next
    setItems(next)
    rememberAttachmentDraft(normalizedDraftKey, next.map((item) => item.file))
    if (rejectedForCount) toast.error(t('chat.attachment.tooMany', { maximum: MAX_CHAT_ATTACHMENT_FILES }))
    if (rejectedForTotal) toast.error(t('chat.attachment.totalTooLarge', { maximum: formatBytes(MAX_CHAT_ATTACHMENTS_BYTES) }))
  }, [enabled, normalizedDraftKey, t])

  const remove = useCallback((id: string) => {
    const removed = itemsRef.current.find((item) => item.id === id)
    if (removed) URL.revokeObjectURL(removed.previewURL)
    const next = itemsRef.current.filter((item) => item.id !== id)
    itemsRef.current = next
    setItems(next)
    rememberAttachmentDraft(normalizedDraftKey, next.map((item) => item.file))
  }, [normalizedDraftKey])

  const clear = useCallback(() => {
    for (const item of itemsRef.current) URL.revokeObjectURL(item.previewURL)
    itemsRef.current = []
    setItems([])
    rememberAttachmentDraft(normalizedDraftKey, [])
    if (inputRef.current) inputRef.current.value = ''
  }, [normalizedDraftKey])

  const openPicker = useCallback(() => {
    if (enabled) inputRef.current?.click()
  }, [enabled])

  const onPaste = useCallback((event: ClipboardEvent<HTMLElement>) => {
    if (!enabled) return
    const files = Array.from(event.clipboardData.files)
    if (!files.length) return
    event.preventDefault()
    addFiles(files)
  }, [addFiles, enabled])

  const onDragOver = useCallback((event: DragEvent<HTMLElement>) => {
    if (!enabled || !Array.from(event.dataTransfer.types).includes('Files')) return
    event.preventDefault()
    event.dataTransfer.dropEffect = 'copy'
  }, [enabled])

  const onDrop = useCallback((event: DragEvent<HTMLElement>) => {
    if (!enabled) return
    const files = Array.from(event.dataTransfer.files)
    if (!files.length) return
    event.preventDefault()
    addFiles(files)
  }, [addFiles, enabled])

  return {
    files: items.map((item) => item.file),
    items,
    addFiles,
    clear,
    input: (
      <input
        ref={inputRef}
        type="file"
        multiple
        hidden
        aria-label={t('chat.attachment.add')}
        onChange={(event) => {
          addFiles(Array.from(event.currentTarget.files || []))
          event.currentTarget.value = ''
        }}
      />
    ),
    openPicker,
    remove,
    dropProps: { onPaste, onDragOver, onDrop },
  }
}

export function ComposerAttachmentTray({ items, onRemove }: {
  items: ComposerAttachment[]
  onRemove: (id: string) => void
}) {
  const { t } = useTranslation()
  if (!items.length) return null
  return (
    <Attachments variant="inline" className="mb-2 max-w-full">
      {items.map((item) => (
        <Attachment
          key={item.id}
          data={{ type: 'file', id: item.id, url: item.previewURL, filename: item.file.name, mediaType: item.file.type || 'application/octet-stream' } satisfies AttachmentData}
          onRemove={() => onRemove(item.id)}
          className="max-w-[min(18rem,100%)] border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-xs text-[var(--nova-text)]"
        >
          <AttachmentPreview />
          <AttachmentInfo />
          <AttachmentRemove label={t('chat.attachment.remove', { name: item.file.name })} />
        </Attachment>
      ))}
    </Attachments>
  )
}

export function SentMessageAttachments({ attachments }: { attachments?: ChatAttachmentDescriptor[] }) {
  if (!attachments?.length) return null
  return (
    <div data-testid="sent-message-attachments" className="mb-1.5 flex max-w-full flex-wrap gap-1.5 border-b border-current/10 pb-1.5">
      {attachments.map((attachment, index) => (
        <span key={`${attachment.id || attachment.name}:${index}`} className="inline-flex max-w-full items-center gap-1 rounded border border-current/15 bg-black/5 px-1.5 py-0.5 text-[11px] dark:bg-white/5">
          <Paperclip className="h-3 w-3 shrink-0 opacity-70" />
          <span className="truncate">{attachment.name}</span>
        </span>
      ))}
    </div>
  )
}

function formatBytes(bytes: number) {
  return bytes >= 1024 * 1024 ? `${Math.round(bytes / (1024 * 1024))} MB` : `${Math.round(bytes / 1024)} KB`
}

function draftItems(key: string): ComposerAttachment[] {
  if (!key) return []
  return (attachmentDrafts.get(key) || []).map((file, index) => ({
    id: attachmentItemID(index),
    file,
    previewURL: URL.createObjectURL(file),
  }))
}

function rememberAttachmentDraft(key: string, files: File[]) {
  if (!key) return
  attachmentDrafts.delete(key)
  if (files.length) attachmentDrafts.set(key, [...files])
  while (attachmentDrafts.size > MAX_ATTACHMENT_DRAFTS) {
    const oldest = attachmentDrafts.keys().next().value
    if (oldest === undefined) break
    attachmentDrafts.delete(oldest)
  }
}

function attachmentItemID(index: number) {
  return typeof crypto.randomUUID === 'function' ? crypto.randomUUID() : `${Date.now()}-${index}`
}
