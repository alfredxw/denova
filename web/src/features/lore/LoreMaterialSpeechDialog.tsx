import { useId, useState } from 'react'
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
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import type { LoreItemSpeechGenerateRequest } from '@/lib/api'

export function LoreMaterialSpeechDialog({
  itemName,
  busy,
  onClose,
  onGenerate,
}: {
  itemName: string
  busy: boolean
  onClose: () => void
  onGenerate: (input: LoreItemSpeechGenerateRequest) => Promise<boolean>
}) {
  const { t } = useTranslation()
  const id = useId()
  const [name, setName] = useState(itemName)
  const [text, setText] = useState('')
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose()
      }}
    >
      <DialogContent className="max-h-[90dvh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{t('lore.materials.generateSpeech')}</DialogTitle>
          <DialogDescription>{t('lore.materials.speechHint')}</DialogDescription>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor={`${id}-name`}>{t('lore.materials.name')}</FieldLabel>
            <Input
              id={`${id}-name`}
              value={name}
              disabled={busy}
              onChange={(event) => setName(event.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={`${id}-text`}>{t('lore.materials.speechText')}</FieldLabel>
            <Textarea
              id={`${id}-text`}
              className="min-h-40"
              value={text}
              maxLength={4096}
              disabled={busy}
              onChange={(event) => setText(event.target.value)}
            />
            <FieldDescription>{t('lore.materials.speechTextHint')}</FieldDescription>
          </Field>
        </FieldGroup>
        <DialogFooter>
          <Button variant="outline" disabled={busy} onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            disabled={busy || !text.trim()}
            onClick={() => {
              void onGenerate({ name, text }).then((ok) => {
                if (ok) onClose()
              })
            }}
          >
            {busy && <Spinner data-icon="inline-start" />}
            {t('lore.materials.generateSpeech')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
