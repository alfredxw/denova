import { useEffect, useId, useState } from 'react'
import { GitBranch, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { CustomAgentSelect } from '@/features/agents/CustomAgentSelect'
import type { BranchCreationSource } from './model'

interface CreateBranchDialogProps {
  projectId: string
  source: BranchCreationSource | null
  onClose: () => void
  onCreate: (source: BranchCreationSource, title: string, customAgentId?: string) => void | Promise<void>
}

/** Shared branch creation boundary used by story replies and the full route map. */
export function CreateBranchDialog({ projectId, source, onClose, onCreate }: CreateBranchDialogProps) {
  const { t } = useTranslation()
  const inputId = useId()
  const [title, setTitle] = useState('')
  const [customAgentId, setCustomAgentId] = useState<string | undefined>(undefined)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    setTitle(source ? t('branchTimeline.newFromNode', { title: source.title }) : '')
    setCustomAgentId(undefined)
    setError('')
  }, [source, t])

  const close = () => {
    if (!creating) onClose()
  }

  const submit = async () => {
    if (!source || creating) return
    setCreating(true)
    setError('')
    try {
      await onCreate(source, title.trim() || t('branchTimeline.newBranch'), customAgentId)
      onClose()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t('branchTimeline.createFailed'))
    } finally {
      setCreating(false)
    }
  }

  return (
    <Dialog open={Boolean(source)} onOpenChange={(open) => { if (!open) close() }}>
      <DialogContent className="nova-panel border text-[var(--nova-text)]" showCloseButton={false}>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <GitBranch className="h-4 w-4 text-[var(--director-brass,var(--nova-text-muted))]" />
            {t('branchTimeline.dialogTitle')}
          </DialogTitle>
          <DialogDescription className="text-[var(--nova-text-muted)]">
            {t('branchTimeline.dialogDescription', { title: source?.title || '' })}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-2">
          <Label htmlFor={inputId}>{t('branchTimeline.nameLabel')}</Label>
          <Input
            id={inputId}
            className="nova-field text-sm"
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter' && !event.nativeEvent.isComposing) void submit()
            }}
            placeholder={t('branchTimeline.namePlaceholder')}
            autoFocus
          />
          <Label>{t('agents.custom.select')}</Label>
          <CustomAgentSelect
            projectId={projectId}
            baseKind="interactive_story"
            value={customAgentId}
            onValueChange={setCustomAgentId}
            inheritLabel={t('agents.custom.inheritCurrent')}
            className="nova-field h-9 w-full"
          />
          <div className="text-[11px] leading-4 text-[var(--nova-text-faint)]">{t('agents.custom.switchNote')}</div>
          {source?.summary ? <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-2 text-xs leading-5 text-[var(--nova-text-muted)]">{source.summary}</div> : null}
          {error ? (
            <div role="alert" className="rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] p-2 text-xs text-[var(--nova-danger)]">
              {error}
            </div>
          ) : null}
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={close} disabled={creating}>{t('common.cancel')}</Button>
          <Button className="gap-1.5 border border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)] hover:bg-[var(--nova-hover)]" onClick={() => void submit()} disabled={!source || creating}>
            {creating ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <GitBranch className="h-3.5 w-3.5" />}
            {creating ? t('common.creating') : t('branchTimeline.createAndSwitch')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
