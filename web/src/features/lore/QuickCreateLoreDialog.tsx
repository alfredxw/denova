import { useContext, useEffect, useId, useMemo, useRef, useState } from 'react'
import { QueryClientContext } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { createProjectLoreItem, type LoreItem } from '@/lib/api'
import { toast } from '@/lib/toast'
import { notifyLoreUpdated } from './events'
import { knowledgeSections } from './knowledge-sections'
import { useOptionalLoreCategories } from './lore-lens-preferences'

interface QuickCreateLoreDialogProps {
  open: boolean
  projectId: string
  initialName: string
  items: readonly LoreItem[]
  onOpenChange: (open: boolean) => void
  onCreated?: (item: LoreItem) => void
}

/** Lightweight dialog for turning a selected prose term into a Project Lore entry without leaving the chapter. */
export function QuickCreateLoreDialog({
  open,
  projectId,
  initialName,
  items,
  onOpenChange,
  onCreated,
}: QuickCreateLoreDialogProps) {
  const { t } = useTranslation()
  const queryClient = useContext(QueryClientContext)
  const categories = useOptionalLoreCategories(projectId)
  const sections = useMemo(() => knowledgeSections(categories), [categories])
  const nameInputId = useId()
  const categoryInputId = useId()
  const briefInputId = useId()
  const nameInputRef = useRef<HTMLInputElement>(null)
  const defaultCategory = useMemo(
    () => sections.find((item) => item.id === 'character')?.id || sections[0]?.id || 'character',
    [sections],
  )
  const [name, setName] = useState(initialName)
  const [category, setCategory] = useState(defaultCategory)
  const [brief, setBrief] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open) return
    setName(initialName.trim())
    setCategory(defaultCategory)
    setBrief('')
    setError('')
    const frame = requestAnimationFrame(() => nameInputRef.current?.focus())
    return () => cancelAnimationFrame(frame)
  }, [defaultCategory, initialName, open])

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (saving) return
    const title = name.trim()
    const validation = !/[\p{L}\p{Nd}]/u.test(title) || title.includes('[[') || title.includes(']]')
      ? t('lore.create.invalidName')
      : items.some((item) => item.name.trim().toLowerCase() === title.toLowerCase())
        ? t('lore.create.duplicateName')
        : ''
    if (validation) {
      setError(validation)
      nameInputRef.current?.focus()
      return
    }
    setSaving(true)
    setError('')
    try {
      const created = await createProjectLoreItem(projectId, {
        name: title,
        type: category,
        enabled: true,
        importance: 'important',
        load_mode: 'auto',
        tags: [],
        keywords: [],
        brief_description: brief.trim(),
        content: '',
      })
      console.info('[lore-lens] created lore item from selection', { projectId, itemId: created.id })
      notifyLoreUpdated({ projectId, ids: [created.id], source: 'quick-create' })
      void queryClient?.invalidateQueries({ queryKey: ['lore-items', projectId] })
      toast.success(t('lore.lens.quickCreateSuccess', { name: created.name }))
      onOpenChange(false)
      onCreated?.(created)
    } catch (cause) {
      console.error('[lore-lens] failed to quick-create lore item', { projectId, category, cause })
      setError(t('lore.create.failed'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(next) => { if (!saving) onOpenChange(next) }}>
      <DialogContent className="sm:max-w-md" data-testid="quick-create-lore-dialog">
        <DialogHeader>
          <DialogTitle>{t('lore.lens.quickCreateTitle')}</DialogTitle>
        </DialogHeader>
        <form onSubmit={(event) => { void handleSubmit(event) }} className="space-y-4">
          <FieldGroup>
            <Field data-invalid={Boolean(error)}>
              <FieldLabel htmlFor={nameInputId}>{t('settingPanel.field.name')}</FieldLabel>
              <Input
                ref={nameInputRef}
                id={nameInputId}
                value={name}
                disabled={saving}
                aria-invalid={Boolean(error)}
                onChange={(event) => { setName(event.target.value); setError('') }}
              />
              {error ? <FieldError role="alert">{error}</FieldError> : null}
            </Field>

            <Field>
              <FieldLabel htmlFor={categoryInputId}>{t('settingPanel.field.type')}</FieldLabel>
              <Select value={category} onValueChange={setCategory} disabled={saving}>
                <SelectTrigger id={categoryInputId}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {sections.map((entry) => (
                    <SelectItem key={entry.id} value={entry.id}>
                      {entry.name || t(entry.labelKey)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            <Field>
              <FieldLabel htmlFor={briefInputId}>{t('settingPanel.field.briefDescription')}</FieldLabel>
              <Textarea
                id={briefInputId}
                value={brief}
                disabled={saving}
                rows={3}
                placeholder={t('lore.lens.quickCreateBriefPlaceholder')}
                onChange={(event) => setBrief(event.target.value)}
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button type="button" variant="outline" disabled={saving} onClick={() => onOpenChange(false)}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={saving}>
              {saving ? t('common.saving') : t('lore.lens.quickCreateSubmit')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
