import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BookPlus, ChevronRight, FolderOpen, Loader2 } from 'lucide-react'
import { BookFormDialog } from '@/components/Home/BookFormDialog'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { ImagePreset } from '@/features/interactive/types'
import type { BookRecord } from '@/lib/api'

interface AgentChatAddProjectDialogProps {
  open: boolean
  novaDir: string
  imagePresets: ImagePreset[]
  defaultImagePresetId: string
  onOpenChange: (open: boolean) => void
  onOpenDirectory: () => void | Promise<void>
  onBeforeCreateBook?: () => Promise<boolean>
  onBookCreated?: (workspace: string, projectId: string) => void | Promise<void>
  onBooksChange: () => void | Promise<void>
}

/** Separates the Project entry decision from the existing, full-featured Book creation flow. */
export function AgentChatAddProjectDialog({
  open,
  novaDir,
  imagePresets,
  defaultImagePresetId,
  onOpenChange,
  onOpenDirectory,
  onBeforeCreateBook,
  onBookCreated,
  onBooksChange,
}: AgentChatAddProjectDialogProps) {
  const { t } = useTranslation()
  const [step, setStep] = useState<'choose' | 'book'>('choose')
  const [preparingBook, setPreparingBook] = useState(false)
  const [coverVersions, setCoverVersions] = useState<Record<string, string>>({})
  const presetOptions = useMemo(() => {
    const validPresets = imagePresets.filter((preset) => !preset.invalid)
    if (validPresets.length > 0) return validPresets
    return [{ id: defaultImagePresetId || 'game-cg', name: t('home.coverDefaultPreset') } as ImagePreset]
  }, [defaultImagePresetId, imagePresets, t])

  useEffect(() => {
    if (open) return
    setStep('choose')
    setPreparingBook(false)
  }, [open])

  const startBookCreation = async () => {
    if (preparingBook) return
    setPreparingBook(true)
    try {
      if (onBeforeCreateBook && !(await onBeforeCreateBook())) return
      setStep('book')
    } catch (error) {
      console.error('[features/agent-chat/AgentChatAddProjectDialog.tsx] preparing Book creation failed', { error })
    } finally {
      setPreparingBook(false)
    }
  }

  return (
    <>
      <Dialog open={open && step === 'choose'} onOpenChange={onOpenChange}>
        <DialogContent className="nova-panel w-[min(440px,calc(100vw-2rem))] gap-0 overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)] sm:max-w-[440px]">
          <DialogHeader className="border-b border-[var(--nova-border)] px-4 py-3.5 text-left">
            <DialogTitle className="text-sm font-semibold">{t('agentChat.project.add')}</DialogTitle>
            <DialogDescription className="text-xs leading-5 text-[var(--nova-text-faint)]">
              {t('agentChat.project.addDescription')}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-2 p-3">
            <ProjectChoice
              icon={FolderOpen}
              title={t('agentChat.project.openDirectory')}
              description={t('agentChat.project.openDirectoryDescription')}
              disabled={preparingBook}
              onClick={() => {
                onOpenChange(false)
                void onOpenDirectory()
              }}
            />
            <ProjectChoice
              icon={BookPlus}
              title={t('agentChat.project.createBook')}
              description={t('agentChat.project.createBookDescription')}
              disabled={preparingBook}
              pending={preparingBook}
              onClick={() => void startBookCreation()}
            />
          </div>
        </DialogContent>
      </Dialog>
      <BookFormDialog
        open={open && step === 'book'}
        mode="create"
        book={null}
        novaDir={novaDir}
        imagePresetOptions={presetOptions}
        defaultImagePresetId={defaultImagePresetId}
        coverVersion={(book: Pick<BookRecord, 'path' | 'cover_updated_at'>) => (
          coverVersions[book.path] || book.cover_updated_at || ''
        )}
        onOpenChange={onOpenChange}
        onCreated={(result) => onBookCreated?.(result.workspace, result.project_id)}
        onBooksChange={onBooksChange}
        onCoverUpdated={(path, version) => {
          setCoverVersions((current) => ({ ...current, [path]: version }))
        }}
      />
    </>
  )
}

function ProjectChoice({
  icon: Icon,
  title,
  description,
  disabled,
  pending = false,
  onClick,
}: {
  icon: typeof FolderOpen
  title: string
  description: string
  disabled: boolean
  pending?: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className="group flex min-w-0 items-center gap-3 rounded-[var(--nova-radius)] border border-transparent bg-[var(--nova-surface-2)] px-3 py-3 text-left transition-colors hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--nova-field-focus-border)] disabled:pointer-events-none disabled:opacity-60"
    >
      <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-[var(--nova-radius)] bg-[var(--nova-surface-3)] text-[var(--nova-text-muted)]">
        {pending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Icon className="h-4 w-4" />}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-medium text-[var(--nova-text)]">{title}</span>
        <span className="mt-0.5 block text-xs leading-5 text-[var(--nova-text-faint)]">{description}</span>
      </span>
      <ChevronRight className="h-4 w-4 shrink-0 text-[var(--nova-text-faint)] transition-transform group-hover:translate-x-0.5 group-hover:text-[var(--nova-text-muted)]" />
    </button>
  )
}
