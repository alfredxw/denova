import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'

interface DeleteConfirmDialogProps {
  open: boolean
  path: string | string[]
  onOpenChange: (open: boolean) => void
  onConfirm: () => Promise<void>
}

/** 删除确认弹窗，避免误删 workspace 文件。 */
export function DeleteConfirmDialog({ open, path, onOpenChange, onConfirm }: DeleteConfirmDialogProps) {
  const { t } = useTranslation()
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const paths = Array.isArray(path) ? path : (path ? [path] : [])

  const handleConfirm = async () => {
    setSubmitting(true)
    setError('')
    try {
      await onConfirm()
      onOpenChange(false)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent className="border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)]">
        <AlertDialogHeader>
          <AlertDialogTitle>{t('sidebar.confirmDeleteTitle')}</AlertDialogTitle>
          <AlertDialogDescription className="text-[var(--nova-text-muted)]">
            {paths.length > 1 ? t('sidebar.confirmDeleteMany', { count: paths.length }) : t('sidebar.confirmDeleteOne', { path: paths[0] || '' })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        {paths.length > 1 && (
          <div className="max-h-28 overflow-y-auto rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 text-xs text-[var(--nova-text-muted)]">
            {paths.map(item => <div key={item} className="truncate">{item}</div>)}
          </div>
        )}
        {error && <div className="text-xs text-[var(--nova-danger)]">{error}</div>}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={submitting}>{t('common.cancel')}</AlertDialogCancel>
          <AlertDialogAction
            className="bg-[var(--nova-danger-bg)] text-[var(--nova-danger)] hover:bg-[var(--nova-danger-bg)]"
            disabled={submitting}
            onClick={(e) => {
              e.preventDefault()
              void handleConfirm()
            }}
          >
            {t('sidebar.delete')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
