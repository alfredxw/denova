import { AlertCircle, FileJson2, FileText, ImageIcon, Maximize2 } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { ImagePreviewDialog } from '@/components/common/ImagePreviewDialog'
import { workspaceAssetURL } from '@/lib/api'
import { workspaceFileKind, type WorkspaceFileKind } from '@/lib/workspace-file-kind'

const JSON_FORMAT_INPUT_LIMIT = 1_000_000
const TEXT_PREVIEW_LIMIT = 400_000

interface FilePreviewProps {
  path: string
  content: string
}

interface TextPreview {
  text: string
  issueKey?: string
  issueOptions?: Record<string, string | number>
}

export function FilePreview({ path, content }: FilePreviewProps) {
  const { t } = useTranslation()
  const kind = workspaceFileKind(path)
  const title = fileName(path)

  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <div className="flex min-h-11 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3">
        <PreviewIcon kind={kind} />
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-medium">{title}</div>
          <div className="truncate font-mono text-[11px] text-[var(--nova-text-faint)]">{path}</div>
        </div>
        <span className="shrink-0 rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-[11px] text-[var(--nova-text-muted)]">
          {t('editor.preview.readOnly')}
        </span>
      </div>
      {kind === 'image' ? (
        <ImageFilePreview path={path} />
      ) : (
        <StructuredTextPreview path={path} content={content} kind={kind} />
      )}
    </div>
  )
}

function ImageFilePreview({ path }: { path: string }) {
  const { t } = useTranslation()
  const src = workspaceAssetURL(path)
  const title = t('editor.preview.imageTitle', { file: fileName(path) })

  return (
    <div className="flex min-h-0 flex-1 items-center justify-center overflow-auto bg-[var(--nova-surface-2)] p-4">
      <ImagePreviewDialog src={src} title={title} alt={title} path={path}>
        <button
          type="button"
          className="group relative flex max-h-full max-w-full items-center justify-center rounded-lg border border-[var(--nova-border)] bg-black/80 p-2 shadow-[var(--nova-shadow)] transition hover:border-[var(--nova-accent)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--nova-accent)]"
          aria-label={t('editor.preview.openImage')}
        >
          <img src={src} alt={title} className="max-h-[calc(100vh-8rem)] max-w-full rounded-md object-contain" />
          <span className="absolute right-3 top-3 rounded-md border border-white/20 bg-black/50 p-1.5 text-white opacity-0 backdrop-blur transition group-hover:opacity-100 group-focus-visible:opacity-100">
            <Maximize2 className="h-4 w-4" />
          </span>
        </button>
      </ImagePreviewDialog>
    </div>
  )
}

function StructuredTextPreview({ path, content, kind }: { path: string; content: string; kind: WorkspaceFileKind }) {
  const { t } = useTranslation()
  const preview = useMemo(() => buildTextPreview(content, kind), [content, kind])
  const labelKey = kind === 'json' ? 'editor.preview.json' : kind === 'jsonl' ? 'editor.preview.jsonl' : 'editor.preview.text'

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <div className="flex shrink-0 items-center gap-2 border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] text-[var(--nova-text-muted)]">
        <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface)] px-1.5 py-0.5">{t(labelKey)}</span>
        {preview.issueKey && (
          <span className="inline-flex min-w-0 items-center gap-1 text-[var(--nova-text-faint)]">
            <AlertCircle className="h-3.5 w-3.5 shrink-0" />
            <span className="truncate">{t(preview.issueKey, preview.issueOptions)}</span>
          </span>
        )}
      </div>
      <pre className="min-h-0 flex-1 overflow-auto bg-[var(--nova-bg)] px-4 py-3 font-mono text-[12px] leading-6 text-[var(--nova-text-muted)] tabular-nums whitespace-pre">
        {preview.text || t('editor.preview.empty')}
      </pre>
      <div className="shrink-0 border-t border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
        <code className="block truncate font-mono text-[11px] text-[var(--nova-text-faint)]" title={path}>{path}</code>
      </div>
    </div>
  )
}

function PreviewIcon({ kind }: { kind: WorkspaceFileKind }) {
  const className = 'h-4 w-4 shrink-0 text-[var(--nova-text-muted)]'
  if (kind === 'image') return <ImageIcon className={className} />
  if (kind === 'json' || kind === 'jsonl') return <FileJson2 className={className} />
  return <FileText className={className} />
}

function buildTextPreview(content: string, kind: WorkspaceFileKind): TextPreview {
  if (kind === 'json') return formatJSON(content)
  if (kind === 'jsonl') return formatJSONL(content)
  return limitText(content)
}

function formatJSON(content: string): TextPreview {
  if (content.length > JSON_FORMAT_INPUT_LIMIT) {
    return { ...limitText(content), issueKey: 'editor.preview.rawLarge' }
  }
  try {
    return limitText(JSON.stringify(JSON.parse(content), null, 2))
  } catch {
    return { ...limitText(content), issueKey: 'editor.preview.invalidJson' }
  }
}

function formatJSONL(content: string): TextPreview {
  if (content.length > JSON_FORMAT_INPUT_LIMIT) {
    return { ...limitText(content), issueKey: 'editor.preview.rawLarge' }
  }
  let invalidLines = 0
  const formatted = content.split(/\r?\n/).map((line) => {
    const trimmed = line.trim()
    if (!trimmed) return ''
    try {
      return JSON.stringify(JSON.parse(trimmed), null, 2)
    } catch {
      invalidLines += 1
      return line
    }
  }).join('\n')
  const preview = limitText(formatted)
  if (invalidLines > 0) {
    return { ...preview, issueKey: 'editor.preview.invalidJsonl', issueOptions: { count: invalidLines } }
  }
  return preview
}

function limitText(text: string): TextPreview {
  if (text.length <= TEXT_PREVIEW_LIMIT) return { text }
  return {
    text: text.slice(0, TEXT_PREVIEW_LIMIT),
    issueKey: 'editor.preview.truncated',
    issueOptions: { count: TEXT_PREVIEW_LIMIT },
  }
}

function fileName(path: string) {
  return path.split('/').pop() || path
}
