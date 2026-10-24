import { useEffect, useId, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { getImagePresets } from '@/features/interactive/api'
import { imagePresetName } from '@/features/interactive/image-preset-name'
import type { ImagePreset } from '@/features/interactive/types'
import type { LoreItemImageGenerateRequest } from '@/lib/api'
import { Spinner } from '@/components/ui/spinner'

export function LoreMaterialGenerateDialog({
  busy,
  onClose,
  onGenerate,
  title,
  description,
  modes = ['agent', 'custom'],
  children,
}: {
  busy: boolean
  onClose: () => void
  onGenerate: (input: LoreItemImageGenerateRequest) => Promise<boolean>
  title?: string
  description?: string
  modes?: Array<'agent' | 'custom'>
  children?: ReactNode
}) {
  const { t } = useTranslation()
  const id = useId()
  const [mode, setMode] = useState<'agent' | 'custom'>('agent')
  const [prompt, setPrompt] = useState('')
  const [presets, setPresets] = useState<ImagePreset[]>([])
  const [preset, setPreset] = useState('game-cg')
  useEffect(() => {
    let alive = true
    void getImagePresets()
      .then((items) => {
        if (alive) setPresets(items.filter((item) => !item.invalid))
      })
      .catch((error) => {
        if (alive) toast.error(error instanceof Error ? error.message : t('lore.materials.failed'))
      })
    return () => {
      alive = false
    }
  }, [t])
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title ?? t('lore.materials.generate')}</DialogTitle>
          <DialogDescription>{description ?? t('lore.materials.generateHint')}</DialogDescription>
        </DialogHeader>
        <FieldGroup>
          {modes.length > 1 && (
            <Field>
              <FieldLabel htmlFor={`${id}-mode`}>
                {t('settingPanel.loreImage.generationMode')}
              </FieldLabel>
              <Select
                value={mode}
                onValueChange={(value) => setMode(value as typeof mode)}
                disabled={busy}
              >
                <SelectTrigger id={`${id}-mode`}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {modes.map((value) => (
                      <SelectItem key={value} value={value}>
                        {t(
                          value === 'agent'
                            ? 'settingPanel.loreImage.modeAgent'
                            : 'settingPanel.loreImage.modeCustom',
                        )}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
          )}
          {children}
          {mode === 'agent' && (
            <Field>
              <FieldLabel htmlFor={`${id}-preset`}>{t('settingPanel.loreImage.preset')}</FieldLabel>
              <Select value={preset} onValueChange={setPreset} disabled={busy}>
                <SelectTrigger id={`${id}-preset`}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {presets.length ? (
                      presets.map((item) => (
                        <SelectItem key={item.id} value={item.id}>
                          {imagePresetName(item, t)}
                        </SelectItem>
                      ))
                    ) : (
                      <SelectItem value="game-cg">
                        {t('settingPanel.editor.defaultImagePreset')}
                      </SelectItem>
                    )}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
          )}
          <Field>
            <FieldLabel htmlFor={`${id}-prompt`}>
              {t(
                mode === 'custom'
                  ? 'settingPanel.loreImage.customPrompt'
                  : 'settingPanel.loreImage.instruction',
              )}
            </FieldLabel>
            <Textarea
              id={`${id}-prompt`}
              value={prompt}
              onChange={(event) => setPrompt(event.target.value)}
              disabled={busy}
              className="min-h-32"
            />
          </Field>
        </FieldGroup>
        <DialogFooter>
          <Button variant="outline" disabled={busy} onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            disabled={busy || (mode === 'custom' && !prompt.trim())}
            onClick={() => {
              void onGenerate({
                mode,
                image_preset_id: preset,
                instruction: mode === 'agent' ? prompt : undefined,
                prompt: mode === 'custom' ? prompt : undefined,
              }).then((ok) => {
                if (ok) onClose()
              })
            }}
          >
            {busy && <Spinner data-icon="inline-start" />}
            {t('lore.materials.generate')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
