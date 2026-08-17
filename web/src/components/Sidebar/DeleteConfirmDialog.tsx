import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'

interface DeleteConfirmDialogProps {
  open: boolean
  path: string | string[]
  recovery?: 'version-history' | 'none'
  onOpenChange: (open: boolean) => void
  onConfirm: () => Promise<void>
}

/** 删除确认弹窗，避免误删 workspace 文件。 */
export function DeleteConfirmDialog({
  open,
  path,
  recovery = 'version-history',
  onOpenChange,
  onConfirm,
}: DeleteConfirmDialogProps) {
  const { t } = useTranslation()
  const paths = Array.isArray(path) ? path : (path ? [path] : [])
  const descriptionKey = recovery === 'version-history'
    ? paths.length > 1 ? 'sidebar.confirmDeleteMany' : 'sidebar.confirmDeleteOne'
    : paths.length > 1 ? 'sidebar.confirmDeleteManyPermanent' : 'sidebar.confirmDeleteOnePermanent'
  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('sidebar.confirmDeleteTitle')}
      description={t(descriptionKey, { count: paths.length, path: paths[0] || '' })}
      details={paths.length > 1 ? paths : undefined}
      confirmLabel={t('sidebar.delete')}
      tone="danger"
      onConfirm={onConfirm}
    />
  )
}
