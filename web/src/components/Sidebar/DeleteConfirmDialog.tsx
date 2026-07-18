import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'

interface DeleteConfirmDialogProps {
  open: boolean
  path: string | string[]
  onOpenChange: (open: boolean) => void
  onConfirm: () => Promise<void>
}

/** 删除确认弹窗，避免误删 workspace 文件。 */
export function DeleteConfirmDialog({ open, path, onOpenChange, onConfirm }: DeleteConfirmDialogProps) {
  const { t } = useTranslation()
  const paths = Array.isArray(path) ? path : (path ? [path] : [])
  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('sidebar.confirmDeleteTitle')}
      description={paths.length > 1 ? t('sidebar.confirmDeleteMany', { count: paths.length }) : t('sidebar.confirmDeleteOne', { path: paths[0] || '' })}
      details={paths.length > 1 ? paths : undefined}
      confirmLabel={t('sidebar.delete')}
      tone="danger"
      onConfirm={onConfirm}
    />
  )
}
