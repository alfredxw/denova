import { AlertTriangle, Check, ChevronDown, FileText, PanelRightClose } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import type { ReviewThreadFile } from '../types'

interface ReviewFileNavigatorProps {
  files: ReviewThreadFile[]
  selectedPath: string
  onSelect: (path: string) => void
  onCollapse: () => void
}

export function ReviewFileNavigator({ files, selectedPath, onSelect, onCollapse }: ReviewFileNavigatorProps) {
  const { t } = useTranslation()
  return (
    <aside data-review-file-navigator className="nova-review-file-navigator h-full min-h-0 w-56 shrink-0 border-l border-[var(--nova-border)] bg-[var(--nova-surface)]">
      <div className="nova-review-file-navigator-wide h-full min-h-0 flex-col">
        <div className="flex h-9 shrink-0 items-center justify-between border-b border-[var(--nova-border)] px-3 text-[10px] font-medium uppercase tracking-wide text-[var(--nova-text-faint)]">
          <span>{t('router.files', { defaultValue: 'Files' })}</span>
          <span className="ml-auto mr-2">{files.length}</span>
          <button
            type="button"
            onClick={onCollapse}
            aria-label={t('changes.hideFileNavigator')}
            title={t('changes.hideFileNavigator')}
            className="nova-nav-item flex size-6 items-center justify-center"
          >
            <PanelRightClose className="size-3.5" />
          </button>
        </div>
        <div role="listbox" aria-label={t('changes.fileNavigator')} className="nova-review-file-navigator-list min-h-0 flex-1 overflow-y-auto p-1.5">
          {files.map((file) => {
            const selected = file.path === selectedPath
            return (
              <button
                key={file.path}
                type="button"
                role="option"
                aria-selected={selected}
                aria-label={t('changes.jumpToFile', { path: file.path })}
                onClick={() => onSelect(file.path)}
                className={`nova-review-file-option mb-1 flex w-full min-w-0 items-center gap-2 rounded-md border px-2 py-2 text-left transition-colors ${selected ? 'border-[var(--nova-accent-blue)] bg-[var(--nova-active)]' : 'border-transparent hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)]'}`}
                title={t('changes.jumpToFile', { path: file.path })}
              >
                <ReviewFileItem file={file} />
              </button>
            )
          })}
        </div>
      </div>

      <div className="nova-review-file-navigator-compact min-w-0 items-center gap-1 p-1.5">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button type="button" size="sm" variant="ghost" className="min-w-0 flex-1 justify-between font-normal">
              <span className="flex min-w-0 items-center gap-2">
                <span className="truncate">{t('router.files', { defaultValue: 'Files' })}</span>
                <span className="text-[var(--nova-text-faint)]">{files.length}</span>
              </span>
              <ChevronDown data-icon="inline-end" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="max-h-[min(60vh,28rem)] w-[min(28rem,calc(100vw-1.5rem))]">
            <DropdownMenuGroup>
              {files.map((file) => (
                <DropdownMenuCheckboxItem
                  key={file.path}
                  checked={file.path === selectedPath}
                  onSelect={() => onSelect(file.path)}
                  aria-label={t('changes.jumpToFile', { path: file.path })}
                  className="min-w-0 py-2"
                >
                  <ReviewFileItem file={file} />
                </DropdownMenuCheckboxItem>
              ))}
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
        <Button
          type="button"
          size="icon-sm"
          variant="ghost"
          onClick={onCollapse}
          aria-label={t('changes.hideFileNavigator')}
          title={t('changes.hideFileNavigator')}
        >
          <PanelRightClose />
        </Button>
      </div>
    </aside>
  )
}

function ReviewFileItem({ file }: { file: ReviewThreadFile }) {
  const conflicted = file.continuity === 'conflicted' || file.continuity === 'discontinuous' || file.apply_state === 'conflicted'
  return (
    <>
      <FileText className="shrink-0 text-[var(--nova-text-faint)]" />
      <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-[var(--nova-text)]">{file.path}</span>
      {conflicted ? <AlertTriangle className="shrink-0 text-[var(--nova-warning)]" /> : file.review_status === 'accepted' ? <Check className="shrink-0 text-[var(--nova-success)]" /> : null}
      {file.pending_edit_ids.length > 0 && <span className="shrink-0 rounded border border-[var(--nova-border)] px-1 text-[9px] text-[var(--nova-warning)]">{file.pending_edit_ids.length}</span>}
    </>
  )
}
