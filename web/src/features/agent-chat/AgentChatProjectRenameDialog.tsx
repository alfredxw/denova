import { useEffect, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { AgentChatProject } from './api'

interface AgentChatProjectRenameDialogProps {
  project: AgentChatProject | null
  onOpenChange: (open: boolean) => void
  onRename: (name: string) => void | Promise<void>
}

/** Rename only the Project label; folder selection is always delegated to the OS. */
export function AgentChatProjectRenameDialog({ project, onOpenChange, onRename }: AgentChatProjectRenameDialogProps) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!project) return
    setName(project.name)
    setSubmitting(false)
    setError('')
  }, [project])

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!project || submitting) return
    const nextName = name.trim()
    if (!nextName) {
      setError(t('agentChat.project.nameRequired'))
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
      open={Boolean(project)}
      onOpenChange={(open) => {
        if (!submitting) onOpenChange(open)
      }}
    >
      <DialogContent className="border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)] sm:max-w-md">
        <form
          onSubmit={(event) => {
            void submit(event)
          }}
        >
          <DialogHeader>
            <DialogTitle>{t('agentChat.project.renameTitle')}</DialogTitle>
            <DialogDescription className="text-[var(--nova-text-muted)]">{t('agentChat.project.renameDescription')}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-2 py-4">
            <Label htmlFor="agent-chat-project-name">{t('agentChat.project.name')}</Label>
            <Input
              id="agent-chat-project-name"
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
