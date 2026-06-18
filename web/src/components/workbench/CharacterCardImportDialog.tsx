import { Upload } from 'lucide-react'
import type { RefObject } from 'react'
import { useTranslation } from 'react-i18next'
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
  userCharacterName: string
  previewing: boolean
  importing: boolean
  error: string
  fileInputRef: RefObject<HTMLInputElement | null>
  onOpenChange: (open: boolean) => void
  onFileSelected: (file: File | undefined) => void | Promise<void>
  onTargetModeChange: (mode: CharacterCardTargetMode) => void
  onBookTitleChange: (title: string) => void
  onUserCharacterNameChange: (name: string) => void
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
  userCharacterName,
  previewing,
  importing,
  error,
  fileInputRef,
  onOpenChange,
  onFileSelected,
  onTargetModeChange,
  onBookTitleChange,
  onUserCharacterNameChange,
  onImport,
}: CharacterCardImportDialogProps) {
  const { t } = useTranslation()
  const hasSelectedFile = Boolean(file)

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
            <DialogTitle className="text-sm font-semibold text-[var(--nova-text)]">{t('importCard.title')}</DialogTitle>
            <DialogDescription id="character-card-import-desc" className="mt-1 text-xs text-[var(--nova-text-faint)]">
              {t('importCard.description')}
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
                {t('importCard.chooseFile')}
              </Button>
              <div className="min-w-0 flex-1 truncate text-[var(--nova-text-faint)]">
                {file ? file.name : t('importCard.noFile')}
              </div>
              {previewing && <span className="shrink-0 text-[var(--nova-text-muted)]">{t('importCard.parsing')}</span>}
            </div>

            {preview && (
              <div className="space-y-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
                <div className="truncate text-sm font-medium text-[var(--nova-text)]">{preview.name}</div>
                <div className="mt-1 flex flex-wrap items-center gap-2 text-[11px] text-[var(--nova-text-faint)]">
                  <span>{t('importCard.entryCount', { count: preview.entry_count })}</span>
                  <span>{t('importCard.openingPresetCount', { count: preview.opening_preset_count })}</span>
                  {preview.will_import_cover && <span>{t('importCard.willImportCover')}</span>}
                  {preview.user_placeholder_found && <span>{t('importCard.willImportUser')}</span>}
                  {preview.tags?.map((tag) => (
                    <span key={tag} className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 text-[var(--nova-text-muted)]">{tag}</span>
                  ))}
                </div>
                <CompatibilityReport preview={preview} />
              </div>
            )}

            {hasSelectedFile && (
              <div className="grid grid-cols-2 gap-2 rounded-[var(--nova-radius)] bg-[var(--nova-surface)] p-1">
                <button
                  type="button"
                  className={`nova-nav-item h-8 px-3 text-xs ${targetMode === 'current' ? 'is-active' : ''}`}
                  onClick={() => onTargetModeChange('current')}
                  disabled={!workspace || importing}
                  title={workspace ? t('importCard.importCurrentTitle') : t('importCard.noCurrentBookTitle')}
                >
                  {t('importCard.importCurrent')}
                </button>
                <button
                  type="button"
                  className={`nova-nav-item h-8 px-3 text-xs ${targetMode === 'new_book' ? 'is-active' : ''}`}
                  onClick={() => onTargetModeChange('new_book')}
                  disabled={importing}
                >
                  {t('importCard.importNewBook')}
                </button>
              </div>
            )}

            {hasSelectedFile && (
              targetMode === 'current' ? (
                <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-[var(--nova-text-faint)]">
                  {t('importCard.currentBook')}<span className="text-[var(--nova-text-muted)]">{workspace ? currentBookName : t('workbench.noBook')}</span>
                </div>
              ) : (
                <div className="space-y-2">
                  <Input
                    value={bookTitle}
                    onChange={(event) => onBookTitleChange(event.target.value)}
                    placeholder={preview?.name || t('importCard.newBookTitle')}
                    className="nova-field w-full rounded-[var(--nova-radius)] border px-2.5 py-1.5 outline-none placeholder:text-[var(--nova-text-faint)] focus:border-[var(--nova-field-focus-border)] focus:bg-[var(--nova-surface-3)]"
                    disabled={importing}
                  />
                  <div className="truncate text-[11px] text-[var(--nova-text-faint)]">{t('importCard.createIn', { dir: novaDir || t('importCard.novaDir') })}</div>
                </div>
              )
            )}

            {preview?.user_placeholder_found && (
              <div className="space-y-2">
                <Input
                  value={userCharacterName}
                  onChange={(event) => onUserCharacterNameChange(event.target.value)}
                  placeholder={t('importCard.userCharacterName')}
                  className="nova-field w-full rounded-[var(--nova-radius)] border px-2.5 py-1.5 outline-none placeholder:text-[var(--nova-text-faint)] focus:border-[var(--nova-field-focus-border)] focus:bg-[var(--nova-surface-3)]"
                  disabled={importing}
                />
                <div className="text-[11px] leading-4 text-[var(--nova-text-faint)]">{t('importCard.userCharacterHint')}</div>
              </div>
            )}

            {error && (
              <div className="rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-[var(--nova-danger)]">
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
              {t('common.cancel')}
            </Button>
            <Button
              type="button"
              size="xs"
              className="border border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)] hover:bg-[var(--nova-hover)]"
              onClick={onImport}
              disabled={!file || !preview || previewing || importing}
            >
              {importing ? t('importCard.importing') : t('importCard.import')}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}

function CompatibilityReport({ preview }: { preview: CharacterCardPreview }) {
  const { t } = useTranslation()
  const groups = [
    { key: 'imported', fields: preview.compatibility?.imported_fields || [] },
    { key: 'downgraded', fields: preview.compatibility?.downgraded_fields || [] },
    { key: 'unsupported', fields: preview.compatibility?.unsupported_fields || [] },
  ].filter((group) => group.fields.length > 0)
  if (groups.length === 0) return null
  return (
    <div className="space-y-1 border-t border-[var(--nova-border)] pt-2 text-[11px] leading-5">
      {groups.map((group) => (
        <div key={group.key} className="flex min-w-0 gap-2">
          <span className="shrink-0 text-[var(--nova-text-muted)]">{t(`importCard.compat.${group.key}`)}</span>
          <span className="min-w-0 flex-1 text-[var(--nova-text-faint)]">
            {group.fields.map((field) => t(`importCard.compat.field.${field}`, { defaultValue: field })).join('、')}
          </span>
        </div>
      ))}
    </div>
  )
}
