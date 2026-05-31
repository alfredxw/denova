import { useState } from 'react'
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
      <AlertDialogContent className="border-[#3a3d44] bg-[#25262a] text-[#d7dbe2]">
        <AlertDialogHeader>
          <AlertDialogTitle>确认删除？</AlertDialogTitle>
          <AlertDialogDescription className="text-[#858b96]">
            {paths.length > 1 ? `将删除选中的 ${paths.length} 项，文件会移入系统回收站。` : `将删除 ${paths[0] || ''}，文件会移入系统回收站。`}
          </AlertDialogDescription>
        </AlertDialogHeader>
        {paths.length > 1 && (
          <div className="max-h-28 overflow-y-auto rounded border border-[#303238] bg-[#1b1c1f] p-2 text-xs text-[#aeb4bf]">
            {paths.map(item => <div key={item} className="truncate">{item}</div>)}
          </div>
        )}
        {error && <div className="text-xs text-red-400">{error}</div>}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={submitting}>取消</AlertDialogCancel>
          <AlertDialogAction
            className="bg-red-600 text-white hover:bg-red-500"
            disabled={submitting}
            onClick={(e) => {
              e.preventDefault()
              void handleConfirm()
            }}
          >
            删除
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
