import { useEffect, useState } from 'react'
import { ChevronDown, ChevronUp, Plus, RotateCcw, Sparkles, Trash2, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import type { AgentQuickPromptBehavior, AgentQuickPromptSettings } from '@/features/settings/types'
import type { QuickPromptSettingsChanges } from './use-agent-quick-prompts'

const MAX_PROMPT_NAME_LENGTH = 128
const MAX_PROMPT_LENGTH = 64 * 1024

interface QuickPromptSettingsDialogProps {
  open: boolean
  scopeLabel: string
  prompts: AgentQuickPromptSettings[]
  defaults: AgentQuickPromptSettings[]
  customized: boolean
  showInCommands: boolean
  onOpenChange: (open: boolean) => void
  onSave: (changes: QuickPromptSettingsChanges) => Promise<void>
}

export function QuickPromptSettingsDialog({
  open,
  scopeLabel,
  prompts,
  defaults,
  customized,
  showInCommands,
  onOpenChange,
  onSave,
}: QuickPromptSettingsDialogProps) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState<AgentQuickPromptSettings[]>([])
  const [restoreDefaults, setRestoreDefaults] = useState(false)
  const [commandsDraft, setCommandsDraft] = useState(showInCommands)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')

  useEffect(() => {
    if (!open) return
    setDraft(clonePrompts(prompts))
    setRestoreDefaults(false)
    setCommandsDraft(showInCommands)
    setSaveError('')
  }, [open, prompts, showInCommands])

  const invalid = draft.some((prompt) => !prompt.name.trim() || !prompt.prompt.trim())
  const promptsDirty = restoreDefaults
    ? customized
    : JSON.stringify(draft) !== JSON.stringify(prompts)
  const dirty = promptsDirty || commandsDraft !== showInCommands
  const canRestore = customized || JSON.stringify(draft) !== JSON.stringify(defaults)

  const updatePrompt = (index: number, patch: Partial<AgentQuickPromptSettings>) => {
    setRestoreDefaults(false)
    setDraft((current) => current.map((prompt, promptIndex) => (
      promptIndex === index ? { ...prompt, ...patch } : prompt
    )))
  }
  const movePrompt = (index: number, offset: -1 | 1) => {
    const target = index + offset
    if (target < 0 || target >= draft.length) return
    setRestoreDefaults(false)
    setDraft((current) => {
      const reordered = [...current]
      const moving = reordered[index]
      reordered[index] = reordered[target]
      reordered[target] = moving
      return reordered
    })
  }
  const addPrompt = () => {
    setRestoreDefaults(false)
    setDraft((current) => [...current, {
      id: createQuickPromptID(),
      name: t('chat.quick.settings.newPromptName'),
      prompt: '',
      behavior: 'fill',
      enabled: true,
    }])
  }
  const restore = () => {
    setDraft(clonePrompts(defaults))
    setRestoreDefaults(true)
    setSaveError('')
  }
  const save = async () => {
    if (invalid || !dirty || saving) return
    setSaving(true)
    setSaveError('')
    try {
      await onSave({
        ...(promptsDirty ? { prompts: restoreDefaults ? null : normalizePrompts(draft) } : {}),
        ...(commandsDraft !== showInCommands ? { showInCommands: commandsDraft } : {}),
      })
      onOpenChange(false)
    } catch (error) {
      console.error('[features/agent-quick-prompts/QuickPromptSettingsDialog.tsx] saving quick prompts failed', { error })
      setSaveError(t('chat.quick.settings.saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(nextOpen) => { if (!saving) onOpenChange(nextOpen) }}>
      <DialogContent className="max-h-[min(90dvh,48rem)] grid-rows-[auto_minmax(0,1fr)_auto] gap-0 overflow-hidden p-0 sm:max-w-[44rem]">
        <DialogHeader className="border-b px-5 py-4 pr-12 text-left">
          <DialogTitle>{t('chat.quick.settings.title', { scope: scopeLabel })}</DialogTitle>
          <DialogDescription>{t('chat.quick.settings.description')}</DialogDescription>
        </DialogHeader>

        <div className="min-h-0 overflow-y-auto px-5 py-4">
          <label className="mb-4 flex cursor-pointer items-center gap-3 rounded-lg border p-3">
            <span className="min-w-0 flex-1">
              <span className="block text-sm font-medium">{t('chat.quick.settings.showInCommands')}</span>
              <span className="mt-1 block text-xs text-muted-foreground">{t('chat.quick.settings.commandsHint')}</span>
            </span>
            <Switch checked={commandsDraft} onCheckedChange={setCommandsDraft} disabled={saving} aria-label={t('chat.quick.settings.showInCommands')} />
          </label>
          {draft.length === 0 ? (
            <Empty className="min-h-48 border">
              <EmptyHeader>
                <EmptyMedia variant="icon"><Sparkles aria-hidden="true" /></EmptyMedia>
                <EmptyTitle>{t('chat.quick.settings.emptyTitle')}</EmptyTitle>
                <EmptyDescription>{t('chat.quick.settings.emptyDescription')}</EmptyDescription>
              </EmptyHeader>
              <EmptyContent>
                <Button type="button" variant="outline" size="sm" onClick={addPrompt}>
                  <Plus data-icon="inline-start" />
                  {t('chat.quick.settings.add')}
                </Button>
              </EmptyContent>
            </Empty>
          ) : (
            <div className="flex flex-col gap-3">
              {draft.map((prompt, index) => (
                <QuickPromptEditor
                  key={prompt.id}
                  prompt={prompt}
                  index={index}
                  count={draft.length}
                  onUpdate={(patch) => updatePrompt(index, patch)}
                  onMove={(offset) => movePrompt(index, offset)}
                  onRemove={() => {
                    setRestoreDefaults(false)
                    setDraft((current) => current.filter((_, promptIndex) => promptIndex !== index))
                  }}
                />
              ))}
            </div>
          )}
          {saveError ? <p role="alert" className="mt-3 text-sm text-destructive">{saveError}</p> : null}
        </div>

        <DialogFooter className="m-0 flex-row items-center justify-between gap-2 border-t px-5 py-3 sm:justify-between">
          <div className="flex min-w-0 items-center gap-2">
            <Button type="button" variant="ghost" size="sm" disabled={!canRestore || saving} onClick={restore}>
              <RotateCcw data-icon="inline-start" />
              <span className="hidden sm:inline">{t('chat.quick.settings.restore')}</span>
            </Button>
            <Button type="button" variant="outline" size="sm" disabled={saving} onClick={addPrompt}>
              <Plus data-icon="inline-start" />
              {t('chat.quick.settings.add')}
            </Button>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button type="button" variant="ghost" size="sm" disabled={saving} onClick={() => onOpenChange(false)}>
              {t('common.cancel')}
            </Button>
            <Button type="button" size="sm" disabled={invalid || !dirty || saving} onClick={() => void save()}>
              {saving ? t('common.saving') : t('common.save')}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function QuickPromptEditor({ prompt, index, count, onUpdate, onMove, onRemove }: {
  prompt: AgentQuickPromptSettings
  index: number
  count: number
  onUpdate: (patch: Partial<AgentQuickPromptSettings>) => void
  onMove: (offset: -1 | 1) => void
  onRemove: () => void
}) {
  const { t } = useTranslation()
  const nameInvalid = !prompt.name.trim()
  const promptInvalid = !prompt.prompt.trim()
  const displayName = prompt.name.trim() || t('chat.quick.settings.unnamed')
  const behaviorLabel = t(`chat.quick.behavior.${prompt.behavior}`)

  return (
    <section className="overflow-hidden rounded-xl border bg-card text-card-foreground">
      <div className="flex min-h-12 items-center gap-2 px-3 py-2">
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-2">
            <span className="truncate text-sm font-medium">{displayName}</span>
            <Badge variant={prompt.behavior === 'send' ? 'secondary' : 'outline'} className="shrink-0">
              {prompt.behavior === 'send' ? <Zap data-icon="inline-start" /> : null}
              {behaviorLabel}
            </Badge>
          </div>
        </div>
        <Switch
          size="sm"
          checked={prompt.enabled}
          onCheckedChange={(enabled) => onUpdate({ enabled })}
          aria-label={t('chat.quick.settings.toggle', { name: displayName })}
        />
        <div className="flex shrink-0 items-center gap-0.5">
          <Button type="button" variant="ghost" size="icon-sm" disabled={index === 0} onClick={() => onMove(-1)} aria-label={t('chat.quick.settings.moveUp', { name: displayName })}>
            <ChevronUp />
          </Button>
          <Button type="button" variant="ghost" size="icon-sm" disabled={index === count - 1} onClick={() => onMove(1)} aria-label={t('chat.quick.settings.moveDown', { name: displayName })}>
            <ChevronDown />
          </Button>
          <Button type="button" variant="ghost" size="icon-sm" onClick={onRemove} aria-label={t('chat.quick.settings.delete', { name: displayName })}>
            <Trash2 />
          </Button>
        </div>
      </div>
      <Separator />
      <FieldGroup className="gap-3 p-3">
        <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_12rem]">
          <Field data-invalid={nameInvalid}>
            <FieldLabel htmlFor={`quick-prompt-name-${prompt.id}`}>{t('chat.quick.settings.name')}</FieldLabel>
            <Input
              id={`quick-prompt-name-${prompt.id}`}
              value={prompt.name}
              maxLength={MAX_PROMPT_NAME_LENGTH}
              aria-invalid={nameInvalid}
              onChange={(event) => onUpdate({ name: event.target.value })}
            />
            {nameInvalid ? <FieldError>{t('chat.quick.settings.required')}</FieldError> : null}
          </Field>
          <Field>
            <FieldLabel htmlFor={`quick-prompt-behavior-${prompt.id}`}>{t('chat.quick.settings.behavior')}</FieldLabel>
            <Select value={prompt.behavior} onValueChange={(behavior) => onUpdate({ behavior: behavior as AgentQuickPromptBehavior })}>
              <SelectTrigger id={`quick-prompt-behavior-${prompt.id}`}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="fill">{t('chat.quick.behavior.fill')}</SelectItem>
                  <SelectItem value="send">{t('chat.quick.behavior.send')}</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
        </div>
        <Field data-invalid={promptInvalid}>
          <FieldLabel htmlFor={`quick-prompt-content-${prompt.id}`}>{t('chat.quick.settings.prompt')}</FieldLabel>
          <Textarea
            id={`quick-prompt-content-${prompt.id}`}
            value={prompt.prompt}
            rows={4}
            maxLength={MAX_PROMPT_LENGTH}
            aria-invalid={promptInvalid}
            onChange={(event) => onUpdate({ prompt: event.target.value })}
          />
          {promptInvalid ? <FieldError>{t('chat.quick.settings.required')}</FieldError> : null}
          {prompt.behavior === 'send' ? <p className="text-xs text-muted-foreground">{t('chat.quick.settings.sendHint')}</p> : null}
        </Field>
      </FieldGroup>
    </section>
  )
}

function normalizePrompts(prompts: AgentQuickPromptSettings[]): AgentQuickPromptSettings[] {
  return prompts.map((prompt) => ({
    ...prompt,
    name: prompt.name.trim(),
    prompt: prompt.prompt.trim(),
  }))
}

function clonePrompts(prompts: AgentQuickPromptSettings[]): AgentQuickPromptSettings[] {
  return prompts.map((prompt) => ({ ...prompt }))
}

function createQuickPromptID(): string {
  const suffix = globalThis.crypto?.randomUUID?.().replaceAll('-', '')
    ?? `${Date.now().toString(36)}${Math.random().toString(36).slice(2)}`
  return `prompt-${suffix.toLowerCase()}`
}
