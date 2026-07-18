import { useEffect, useState } from 'react'
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
import { cn } from '@/lib/utils'

interface ConfirmDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: string
  confirmLabel?: string
  /** danger 时确认按钮使用危险色，用于删除等不可逆操作 */
  tone?: 'default' | 'danger'
  /** 附加明细列表（如受影响的多个路径），仅在多于一条时展示 */
  details?: string[]
  /** 抛错时错误内联展示且弹窗保持打开；成功或返回 void 时自动关闭 */
  onConfirm: () => void | Promise<void>
}

/** 通用确认弹窗，统一替代 window.confirm；内建 submitting / error 状态。 */
export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel,
  tone = 'default',
  details,
  onConfirm,
}: ConfirmDialogProps) {
  const { t } = useTranslation()
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (open) setError('')
  }, [open])

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
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription className="text-[var(--nova-text-muted)]">{description}</AlertDialogDescription>
        </AlertDialogHeader>
        {details && details.length > 1 && (
          <div className="max-h-28 overflow-y-auto rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 text-xs text-[var(--nova-text-muted)]">
            {details.map((item) => <div key={item} className="truncate">{item}</div>)}
          </div>
        )}
        {error && <div className="text-xs text-[var(--nova-danger)]">{error}</div>}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={submitting}>{t('common.cancel')}</AlertDialogCancel>
          <AlertDialogAction
            className={cn(tone === 'danger' && 'bg-[var(--nova-danger-bg)] text-[var(--nova-danger)] hover:bg-[var(--nova-danger-bg)]')}
            disabled={submitting}
            onClick={(e) => {
              e.preventDefault()
              void handleConfirm()
            }}
          >
            {confirmLabel ?? t('common.confirm')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
