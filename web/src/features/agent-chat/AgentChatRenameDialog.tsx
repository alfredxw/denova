import { useEffect, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

interface AgentChatRenameDialogProps {
  open: boolean
  initialValue: string
  title: string
  description: string
  label: string
  requiredMessage: string
  inputId: string
  onOpenChange: (open: boolean) => void
  onRename: (name: string) => void | Promise<void>
}

/** Shared rename flow for Project labels and durable conversation titles. */
export function AgentChatRenameDialog({
  open,
  initialValue,
  title,
  description,
  label,
  requiredMessage,
  inputId,
  onOpenChange,
  onRename,
}: AgentChatRenameDialogProps) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open) return
    setName(initialValue)
    setSubmitting(false)
    setError('')
  }, [initialValue, open])

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!open || submitting) return
    const nextName = name.trim()
    if (!nextName) {
      setError(requiredMessage)
      return
    }
    setSubmitting(true)
    setError('')
    try {
      await onRename(nextName)
      onOpenChange(false)
    } catch (submitError) {
      setError(submitError instanceof Error ? submitError.message : String(submitError))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!submitting) onOpenChange(nextOpen)
      }}
    >
      <DialogContent className="border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)] sm:max-w-md">
        <form
          onSubmit={(event) => {
            void submit(event)
          }}
        >
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription className="text-[var(--nova-text-muted)]">{description}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-2 py-4">
            <Label htmlFor={inputId}>{label}</Label>
            <Input
              id={inputId}
              autoFocus
              value={name}
              onChange={(event) => setName(event.target.value)}
              spellCheck={false}
            />
            {error ? (
              <p role="alert" className="text-xs text-[var(--nova-danger)]">
                {error}
              </p>
            ) : null}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" disabled={submitting} onClick={() => onOpenChange(false)}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={submitting}>
              {t('common.save')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
