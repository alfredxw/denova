import { AlertCircle, ImageIcon, Maximize2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ImagePreviewDialog } from '@/components/common/ImagePreviewDialog'
import { MISSING_WORKSPACE_REVISION } from '@/lib/api-client/workspace'
import { projectFileAssetURL } from '@/lib/api-client/project-files'

interface ImageFilePreviewProps {
  projectId: string
  path: string
  revision?: string
}

/** Image-only preview; editable text files are routed to a document or source editor. */
export function ImageFilePreview({ projectId, path, revision = '' }: ImageFilePreviewProps) {
  const { t } = useTranslation()
  const title = fileName(path)

  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <div className="flex min-h-11 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3">
        <ImageIcon className="h-4 w-4 shrink-0 text-[var(--nova-text-muted)]" />
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-medium">{title}</div>
          <div className="truncate font-mono text-[11px] text-[var(--nova-text-faint)]">{path}</div>
        </div>
        <span className="shrink-0 rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-[11px] text-[var(--nova-text-muted)]">
          {t('editor.preview.readOnly')}
        </span>
      </div>
      {revision === MISSING_WORKSPACE_REVISION && (
        <div role="alert" className="flex shrink-0 items-start gap-2 border-b border-[var(--nova-warning)]/30 bg-[var(--nova-warning-bg)] px-3 py-2 text-[11px] text-[var(--nova-text-muted)]">
          <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-[var(--nova-warning)]" />
          <div className="min-w-0">
            <div className="font-medium text-[var(--nova-text)]">{t('editor.orphaned.title')}</div>
            <div className="mt-0.5 text-[var(--nova-text-faint)]">{t('editor.orphaned.readOnlyDescription')}</div>
          </div>
        </div>
      )}
      <ImagePreviewContent projectId={projectId} path={path} revision={revision} />
    </div>
  )
}

function ImagePreviewContent({ projectId, path, revision }: { projectId: string; path: string; revision: string }) {
  const { t } = useTranslation()
  const src = projectFileAssetURL(projectId, path, revision)
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

function fileName(path: string) {
  return path.split('/').pop() || path
}
