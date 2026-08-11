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

interface UnsavedConfigGuardDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onDiscard: () => void
}

/** 可执行配置存在未保存草稿时，返回/切换前的统一“继续编辑或放弃”对话框。 */
export function UnsavedConfigGuardDialog({
  open,
  onOpenChange,
  onDiscard,
}: UnsavedConfigGuardDialogProps) {
  const { t } = useTranslation()
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('configGuard.title')}</AlertDialogTitle>
          <AlertDialogDescription>{t('configGuard.description')}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t('configGuard.continueEditing')}</AlertDialogCancel>
          <AlertDialogAction onClick={onDiscard}>{t('configGuard.discardChanges')}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
