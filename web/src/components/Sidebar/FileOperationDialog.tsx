import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'

export type FileOperationMode = 'create-file' | 'create-dir' | 'rename' | 'copy' | 'move'

interface FileOperationDialogProps {
  open: boolean
  mode: FileOperationMode
  targetPath: string
  defaultValue: string
  onOpenChange: (open: boolean) => void
  onSubmit: (value: string) => Promise<void>
}

const MODE_META: Record<FileOperationMode, { titleKey: string; descriptionKey: string; labelKey: string }> = {
  'create-file': {
    titleKey: 'sidebar.createFile',
    descriptionKey: 'sidebar.createFileDescription',
    labelKey: 'sidebar.filePath',
  },
  'create-dir': {
    titleKey: 'sidebar.createDir',
    descriptionKey: 'sidebar.createDirDescription',
    labelKey: 'sidebar.dirPath',
  },
  rename: {
    titleKey: 'sidebar.rename',
    descriptionKey: 'sidebar.renameDescription',
    labelKey: 'sidebar.newName',
  },
  copy: {
    titleKey: 'sidebar.copy',
    descriptionKey: 'sidebar.copyDescription',
    labelKey: 'sidebar.targetPath',
  },
  move: {
    titleKey: 'sidebar.move',
    descriptionKey: 'sidebar.moveDescription',
    labelKey: 'sidebar.targetPath',
  },
}

/** 文件操作弹窗，统一承载新建、重命名、复制和移动输入。 */
export function FileOperationDialog({
  open,
  mode,
  targetPath,
  defaultValue,
  onOpenChange,
  onSubmit,
}: FileOperationDialogProps) {
  const { t } = useTranslation()
  const [value, setValue] = useState(defaultValue)
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const meta = MODE_META[mode]

  useEffect(() => {
    if (open) {
      setValue(defaultValue)
      setError('')
    }
  }, [defaultValue, open])

  const handleSubmit = async () => {
    const trimmed = value.trim()
    if (!trimmed || submitting) return
    setSubmitting(true)
    setError('')
    try {
      await onSubmit(trimmed)
      onOpenChange(false)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="border-[#3a3d44] bg-[#25262a] text-[#d7dbe2]">
        <DialogHeader>
          <DialogTitle>{t(meta.titleKey)}</DialogTitle>
          <DialogDescription className="text-[#858b96]">
            {targetPath ? t('sidebar.currentTarget', { path: targetPath }) : t(meta.descriptionKey)}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <label className="text-xs text-[#aeb4bf]" htmlFor="file-operation-input">
            {t(meta.labelKey)}
          </label>
          <Input
            id="file-operation-input"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                void handleSubmit()
              }
            }}
            className="border-[#3a3d44] bg-[#1b1c1f] text-[#d7dbe2]"
            autoFocus
          />
          {error && <div className="text-xs text-red-400">{error}</div>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={submitting}>
            {t('common.cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={!value.trim() || submitting}>
            {t('common.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
