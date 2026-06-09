import { ChapterDiffView } from '@/features/chapters/components/chapter-diff-view'
import { useTranslation } from 'react-i18next'
import { Columns2, FileText, X } from 'lucide-react'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

interface VersionDiffDialogProps {
  open: boolean
  title?: string
  original: string
  modified: string
  language?: string
  sideBySide?: boolean
  onOpenChange: (open: boolean) => void
}

/** 版本差异弹窗，仅展示外部传入的 diff 内容。 */
export function VersionDiffDialog({
  open,
  title,
  original,
  modified,
  language,
  sideBySide,
  onOpenChange,
}: VersionDiffDialogProps) {
  const { t } = useTranslation()
  const displayTitle = title || t('versions.diffTitle')
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        style={{ width: 'min(1280px, calc(100vw - 32px))', maxWidth: 'none' }}
        className="!flex h-[86vh] max-h-[900px] flex-col gap-0 overflow-hidden rounded-[var(--nova-radius)] border border-[#2a2a2a] bg-[#050505] p-0 text-[var(--nova-text)] shadow-[0_24px_72px_rgba(0,0,0,0.72)]"
      >
        <DialogHeader className="shrink-0 gap-0 border-b border-[#202020] bg-[#0b0b0b] px-4 py-3 text-left">
          <div className="flex min-w-0 items-start gap-3">
            <div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-[var(--nova-radius)] border border-[#2a2a2a] bg-[#151515] text-[var(--nova-text-muted)]">
              <FileText className="h-4 w-4" />
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex min-w-0 items-center gap-2">
                <DialogTitle className="truncate text-sm font-semibold leading-5 text-[var(--nova-text)]" title={displayTitle}>
                  {displayTitle}
                </DialogTitle>
                <span className="shrink-0 rounded border border-[#303030] bg-[#141414] px-1.5 py-0.5 text-[10px] font-medium uppercase text-[var(--nova-text-faint)]">
                  {t('versions.diffReadOnly')}
                </span>
              </div>
              <DialogDescription className="mt-1 truncate text-xs text-[var(--nova-text-faint)]">
                {t('versions.diffDescription')}
              </DialogDescription>
            </div>
            <DialogClose className="nova-nav-item flex h-8 w-8 shrink-0 items-center justify-center border border-[#2a2a2a] bg-[#111111] text-[var(--nova-text-faint)] hover:bg-[#1d1d1d] hover:text-[var(--nova-text)]" aria-label={t('common.close')}>
              <X className="h-4 w-4" />
            </DialogClose>
          </div>
        </DialogHeader>

        <div className="grid h-9 shrink-0 grid-cols-2 border-b border-[#202020] bg-[#080808] text-xs text-[var(--nova-text-muted)] max-md:hidden">
          <div className="flex min-w-0 items-center gap-2 border-r border-[#202020] px-4">
            <span className="h-2 w-2 rounded-full bg-[var(--nova-accent-blue)]" />
            <span className="truncate">{t('versions.diffSnapshot')}</span>
          </div>
          <div className="flex min-w-0 items-center gap-2 px-4">
            <span className="h-2 w-2 rounded-full bg-[var(--nova-accent-green)]" />
            <span className="truncate">{t('versions.diffWorkspace')}</span>
          </div>
        </div>

        <div className="min-h-0 flex-1 overflow-hidden border-b border-[#202020] bg-[#050505]">
          <ChapterDiffView
            original={original}
            modified={modified}
            language={language}
            sideBySide={sideBySide}
          />
        </div>

        <div className="flex h-9 shrink-0 items-center justify-between gap-3 bg-[#0b0b0b] px-4 text-[11px] text-[var(--nova-text-faint)]">
          <div className="flex min-w-0 items-center gap-2">
            <Columns2 className="h-3.5 w-3.5 shrink-0" />
            <span className="truncate">{t('versions.diffModeSideBySide')}</span>
          </div>
          <span className="shrink-0">{t('versions.diffReadOnly')}</span>
        </div>
      </DialogContent>
    </Dialog>
  )
}
