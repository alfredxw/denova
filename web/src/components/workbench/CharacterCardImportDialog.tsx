import { Upload } from 'lucide-react'
import type { RefObject } from 'react'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type { CharacterCardPreview } from '@/lib/api'

export type CharacterCardTargetMode = 'current' | 'new_book'

interface CharacterCardImportDialogProps {
  open: boolean
  workspace: string
  currentBookName: string
  novaDir: string
  file: File | null
  preview: CharacterCardPreview | null
  targetMode: CharacterCardTargetMode
  bookTitle: string
  previewing: boolean
  importing: boolean
  error: string
  fileInputRef: RefObject<HTMLInputElement | null>
  onOpenChange: (open: boolean) => void
  onFileSelected: (file: File | undefined) => void | Promise<void>
  onTargetModeChange: (mode: CharacterCardTargetMode) => void
  onBookTitleChange: (title: string) => void
  onImport: () => void | Promise<void>
}

export function CharacterCardImportDialog({
  open,
  workspace,
  currentBookName,
  novaDir,
  file,
  preview,
  targetMode,
  bookTitle,
  previewing,
  importing,
  error,
  fileInputRef,
  onOpenChange,
  onFileSelected,
  onTargetModeChange,
  onBookTitleChange,
  onImport,
}: CharacterCardImportDialogProps) {
  return (
    <>
      <input
        ref={fileInputRef}
        type="file"
        accept=".png,.json,application/json,image/png"
        className="hidden"
        onChange={(event) => void onFileSelected(event.target.files?.[0])}
      />
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent
          className="nova-panel w-[min(520px,calc(100vw-2rem))] max-w-[min(520px,calc(100vw-2rem))] rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)]"
          aria-describedby="character-card-import-desc"
        >
          <div className="border-b border-[var(--nova-border)] px-4 py-3">
            <DialogTitle className="text-sm font-semibold text-[var(--nova-text)]">导入酒馆角色卡</DialogTitle>
            <DialogDescription id="character-card-import-desc" className="mt-1 text-xs text-[var(--nova-text-faint)]">
              选择 PNG 或 JSON 角色卡，并决定写入当前书还是创建一本新书。
            </DialogDescription>
          </div>
          <div className="space-y-4 px-4 py-4 text-xs">
            <div className="flex min-w-0 items-center gap-2">
              <Button
                type="button"
                size="xs"
                variant="ghost"
                className="nova-nav-item border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)]"
                onClick={() => fileInputRef.current?.click()}
                disabled={previewing || importing}
              >
                <Upload className="h-3.5 w-3.5" />
                选择文件
              </Button>
              <div className="min-w-0 flex-1 truncate text-[var(--nova-text-faint)]">
                {file ? file.name : '未选择文件'}
              </div>
              {previewing && <span className="shrink-0 text-[var(--nova-text-muted)]">解析中...</span>}
            </div>

            {preview && (
              <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
                <div className="truncate text-sm font-medium text-[var(--nova-text)]">{preview.name}</div>
                <div className="mt-1 flex flex-wrap items-center gap-2 text-[11px] text-[var(--nova-text-faint)]">
                  <span>{preview.entry_count} 个世界书条目</span>
                  {preview.tags?.map((tag) => (
                    <span key={tag} className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 text-[var(--nova-text-muted)]">{tag}</span>
                  ))}
                </div>
              </div>
            )}

            <div className="grid grid-cols-2 gap-2 rounded-[var(--nova-radius)] bg-[var(--nova-surface)] p-1">
              <button
                type="button"
                className={`nova-nav-item h-8 px-3 text-xs ${targetMode === 'current' ? 'is-active' : ''}`}
                onClick={() => onTargetModeChange('current')}
                disabled={!workspace || importing}
                title={workspace ? '导入到当前书籍资料库' : '当前没有打开的书籍'}
              >
                导入到本书
              </button>
              <button
                type="button"
                className={`nova-nav-item h-8 px-3 text-xs ${targetMode === 'new_book' ? 'is-active' : ''}`}
                onClick={() => onTargetModeChange('new_book')}
                disabled={importing}
              >
                导入成新书
              </button>
            </div>

            {targetMode === 'current' ? (
              <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-[var(--nova-text-faint)]">
                当前书籍：<span className="text-[var(--nova-text-muted)]">{workspace ? currentBookName : '未选择书籍'}</span>
              </div>
            ) : (
              <div className="space-y-2">
                <Input
                  value={bookTitle}
                  onChange={(event) => onBookTitleChange(event.target.value)}
                  placeholder={preview?.name || '新书书名'}
                  className="nova-field w-full rounded-[var(--nova-radius)] border px-2.5 py-1.5 outline-none placeholder:text-[var(--nova-text-faint)] focus:border-[#3a3a3a] focus:bg-[var(--nova-surface-3)]"
                  disabled={importing}
                />
                <div className="truncate text-[11px] text-[var(--nova-text-faint)]">新书将创建在 {novaDir || 'Nova 数据目录'}</div>
              </div>
            )}

            {error && (
              <div className="rounded-[var(--nova-radius)] border border-red-500/30 bg-red-500/10 px-3 py-2 text-red-200">
                {error}
              </div>
            )}
          </div>
          <div className="flex items-center justify-end gap-2 border-t border-[var(--nova-border)] px-4 py-3">
            <Button
              type="button"
              size="xs"
              variant="ghost"
              className="nova-nav-item border border-transparent text-[var(--nova-text-muted)]"
              onClick={() => onOpenChange(false)}
              disabled={importing}
            >
              取消
            </Button>
            <Button
              type="button"
              size="xs"
              className="border border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)] hover:bg-[var(--nova-hover)]"
              onClick={onImport}
              disabled={!file || !preview || previewing || importing}
            >
              {importing ? '导入中...' : '导入'}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
