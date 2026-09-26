import { useEffect, useId, useState } from 'react'
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
import type { ImagePreset } from '@/features/interactive/types'
import type { LoreItemImageGenerateRequest } from '@/lib/api'
import { Spinner } from '@/components/ui/spinner'

export function LoreMaterialGenerateDialog({
  busy,
  onClose,
  onGenerate,
}: {
  busy: boolean
  onClose: () => void
  onGenerate: (input: LoreItemImageGenerateRequest) => Promise<boolean>
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
          <DialogTitle>{t('lore.materials.generate')}</DialogTitle>
          <DialogDescription>{t('lore.materials.generateHint')}</DialogDescription>
        </DialogHeader>
        <FieldGroup>
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
                  <SelectItem value="agent">{t('settingPanel.loreImage.modeAgent')}</SelectItem>
                  <SelectItem value="custom">{t('settingPanel.loreImage.modeCustom')}</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
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
                          {item.name}
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
