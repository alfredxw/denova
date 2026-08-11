import { useState } from 'react'
import { GitBranchPlus, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'

interface CreateInteractiveBranchDialogProps {
  turnId: string
  sourceTitle: string
  onClose: () => void
  onCreate: (turnId: string, title: string) => Promise<void>
}

export function CreateInteractiveBranchDialog({ turnId, sourceTitle, onClose, onCreate }: CreateInteractiveBranchDialogProps) {
  const { t } = useTranslation()
  const [title, setTitle] = useState(() => t('storyStage.branchFromTurn.defaultTitle', { title: sourceTitle }))
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')
  const normalizedTitle = title.trim()

  const create = async () => {
    if (creating || !normalizedTitle) return
    setCreating(true)
    setError('')
    try {
      await onCreate(turnId, normalizedTitle)
      onClose()
    } catch (createError) {
      console.warn('[interactive-branch-create] create from historical turn failed', { turnId, error: createError })
      setError(createError instanceof Error ? createError.message : t('storyStage.branchFromTurn.createFailed'))
    } finally {
      setCreating(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => {
      if (!open && !creating) onClose()
    }}>
      <DialogContent showCloseButton={false} className="max-w-[min(calc(100vw-2rem),520px)] gap-0 overflow-hidden border border-[var(--nova-border)] bg-[var(--nova-surface)] p-0 text-[var(--nova-text)]">
        <form onSubmit={(event) => {
          event.preventDefault()
          void create()
        }}>
          <DialogHeader className="relative border-b border-[var(--nova-border)] px-4 py-3 pr-12 text-left">
            <DialogTitle>{t('storyStage.branchFromTurn.title')}</DialogTitle>
            <DialogDescription>{t('storyStage.branchFromTurn.description')}</DialogDescription>
            <Button type="button" variant="ghost" size="icon-sm" className="absolute right-2 top-2" disabled={creating} onClick={onClose} aria-label={t('common.close')}>
              <X className="h-4 w-4" />
            </Button>
          </DialogHeader>

          <div className="space-y-3 px-4 py-4">
            <label htmlFor={`interactive-branch-${turnId}`} className="block text-xs font-medium text-[var(--nova-text-muted)]">
              {t('storyStage.branchFromTurn.fieldLabel')}
            </label>
            <Input
              id={`interactive-branch-${turnId}`}
              autoFocus
              value={title}
              disabled={creating}
              onChange={(event) => setTitle(event.target.value)}
              className="nova-field"
            />
            {error ? (
              <div role="alert" className="rounded-md border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs text-[var(--nova-danger)]">
                {error}
              </div>
            ) : null}
          </div>

          <DialogFooter className="border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
            <Button type="button" variant="outline" disabled={creating} onClick={onClose}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={creating || !normalizedTitle}>
              <GitBranchPlus className="h-4 w-4" />
              {creating ? t('common.creating') : t('storyStage.branchFromTurn.createAndSwitch')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
