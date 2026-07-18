import { useEffect, useMemo, useState, type ComponentType } from 'react'
import { FilePlus2, PenLine, SearchCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { FormField } from '@/components/forms/form-field'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { AutomationExecutionTarget, AutomationTaskTemplate, BookRecord } from '@/lib/api'

interface AutomationTemplateDialogProps {
  open: boolean
  workspace: string
  books: BookRecord[]
  templates: AutomationTaskTemplate[]
  onOpenChange: (open: boolean) => void
  onChoose: (template: AutomationTaskTemplate | null, target: AutomationExecutionTarget) => void
}

export function AutomationTemplateDialog({
  open,
  workspace,
  books,
  templates,
  onOpenChange,
  onChoose,
}: AutomationTemplateDialogProps) {
  const { t } = useTranslation()
  const defaultTargetValue = workspace ? `workspace:${workspace}` : 'user'
  const [targetValue, setTargetValue] = useState(defaultTargetValue)
  useEffect(() => {
    if (open) setTargetValue(defaultTargetValue)
  }, [defaultTargetValue, open])

  const workspaceOptions = useMemo(() => {
    if (!workspace || books.some((book) => book.path === workspace)) return books
    const name = workspace.split('/').filter(Boolean).at(-1) || workspace
    return [{ name, path: workspace, author: '', last_opened_at: '' }, ...books]
  }, [books, workspace])
  const target: AutomationExecutionTarget = targetValue === 'user'
    ? { kind: 'user' }
    : { kind: 'workspace', workspace: targetValue.slice('workspace:'.length) }
  const availableTemplates = templates.filter((template) => template.target_kinds.includes(target.kind))
  const choose = (template: AutomationTaskTemplate | null) => {
    onChoose(template, target)
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="nova-panel max-w-3xl gap-0 overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)]">
        <DialogHeader className="gap-1 border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-4 py-3 text-left">
          <DialogTitle className="text-sm">{t('automations.create.title')}</DialogTitle>
          <DialogDescription className="text-[11px] leading-5 text-[var(--nova-text-faint)]">
            {t('automations.create.description')}
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4 px-4 py-4">
          <FormField label={t('automations.field.target')}>
            <Select value={targetValue} onValueChange={setTargetValue}>
              <SelectTrigger className="nova-field min-h-8 w-full rounded-[var(--nova-radius)] border text-xs" aria-label={t('automations.field.target')}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="user">{t('automations.target.global')}</SelectItem>
                  {workspaceOptions.map((book) => (
                    <SelectItem key={book.path} value={`workspace:${book.path}`}>
                      {t('automations.target.workspace', { name: book.name })}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </FormField>

          <div>
            <div className="mb-2 text-xs font-medium text-[var(--nova-text)]">{t('automations.create.chooseTemplate')}</div>
            <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
              <TemplateChoice
                icon={FilePlus2}
                title={t('automations.template.blank')}
                description={t('automations.template.blankDescription')}
                meta={t('automations.template.meta.manualReadOnly')}
                onClick={() => choose(null)}
              />
              {availableTemplates.map((template) => (
                <TemplateChoice
                  key={`${template.id}:${template.version}`}
                  icon={template.id === 'continue_writing' ? PenLine : SearchCheck}
                  title={template.defaults.name}
                  description={template.description}
                  meta={templateMeta(template, t)}
                  onClick={() => choose(template)}
                />
              ))}
            </div>
            {target.kind === 'user' && (
              <p className="mt-3 text-[11px] leading-5 text-[var(--nova-text-faint)]">
                {t('automations.template.workspaceOnlyHelp')}
              </p>
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function TemplateChoice({ icon: Icon, title, description, meta, onClick }: {
  icon: ComponentType<{ className?: string }>
  title: string
  description: string
  meta: string
  onClick: () => void
}) {
  return (
    <Button
      type="button"
      variant="outline"
      onClick={onClick}
      className="h-auto min-h-36 items-start justify-start whitespace-normal rounded-[var(--nova-radius)] border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3 text-left hover:bg-[var(--nova-hover)]"
    >
      <span className="flex min-w-0 flex-1 flex-col items-start gap-2">
        <span className="flex h-7 w-7 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)]">
          <Icon className="h-3.5 w-3.5" />
        </span>
        <span className="font-medium text-[var(--nova-text)]">{title}</span>
        <span className="text-[11px] font-normal leading-5 text-[var(--nova-text-muted)]">{description}</span>
        <span className="mt-auto text-[10px] font-normal text-[var(--nova-text-faint)]">{meta}</span>
      </span>
    </Button>
  )
}

function templateMeta(template: AutomationTaskTemplate, t: (key: string, options?: Record<string, unknown>) => string) {
  const chapterBatch = template.defaults.triggers.find((trigger) => trigger.type === 'chapter_batch')
  if (chapterBatch) {
    return t('automations.template.meta.chapterBatch', { count: chapterBatch.chapter_batch_size || 5 })
  }
  if (template.defaults.write_mode === 'confirm_write') {
    return t('automations.template.meta.confirmWrite')
  }
  return t('automations.template.meta.manualReadOnly')
}
