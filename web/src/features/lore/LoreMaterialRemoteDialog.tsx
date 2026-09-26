import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import type { LoreMaterial, LoreMaterialMutation } from '@/lib/api'
import { MaterialImage } from './MaterialImage'

export function LoreMaterialRemoteDialog({
  material,
  busy,
  onClose,
  onSave,
}: {
  material?: LoreMaterial
  busy: boolean
  onClose: () => void
  onSave: (mutation: LoreMaterialMutation) => Promise<boolean>
}) {
  const { t } = useTranslation()
  const id = useId()
  const [url, setURL] = useState(material?.url ?? '')
  const [preview, setPreview] = useState(material?.url ?? '')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [saveLocally, setSaveLocally] = useState(false)
  let valid = false
  try {
    const parsed = new URL(url.trim())
    valid =
      parsed.protocol === 'https:' && !parsed.username && !parsed.password && url.length <= 8192
  } catch {
    /* The input remains editable until it is a complete URL. */
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose()
      }}
    >
      <DialogContent
        className="max-h-[90dvh] overflow-y-auto sm:max-w-xl"
        onPointerDownOutside={(event) => {
          if (busy) event.preventDefault()
        }}
      >
        <DialogHeader>
          <DialogTitle>
            {t(material ? 'lore.materials.replaceURL' : 'lore.materials.fromWeb')}
          </DialogTitle>
          <DialogDescription>{t('lore.materials.webHint')}</DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(event) => {
            event.preventDefault()
            if (valid && !busy)
              void onSave({
                op: 'remote',
                asset_id: material?.id,
                url: url.trim(),
                name,
                description,
                save_locally: saveLocally,
              }).then((ok) => {
                if (ok) onClose()
              })
          }}
        >
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor={`${id}-url`}>{t('lore.materials.url')}</FieldLabel>
              <Input
                id={`${id}-url`}
                type="url"
                placeholder="https://…"
                value={url}
                maxLength={8192}
                disabled={busy}
                onChange={(event) => setURL(event.target.value)}
                onBlur={() => setPreview(valid ? url.trim() : '')}
              />
              <FieldDescription>{t('lore.materials.directLinkHint')}</FieldDescription>
            </Field>
            <div className="flex h-40 items-center justify-center overflow-hidden rounded-md bg-muted text-xs text-muted-foreground">
              {preview ? (
                <MaterialImage
                  key={preview}
                  src={preview}
                  alt={t('lore.materials.webPreview')}
                  className="h-full w-full object-contain"
                />
              ) : (
                t('lore.materials.webPreview')
              )}
            </div>
            {!material && (
              <>
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
                  <FieldLabel htmlFor={`${id}-description`}>
                    {t('lore.materials.description')}
                  </FieldLabel>
                  <Textarea
                    id={`${id}-description`}
                    value={description}
                    disabled={busy}
                    onChange={(event) => setDescription(event.target.value)}
                  />
                </Field>
              </>
            )}
            <Field>
              <div className="flex items-center gap-2">
                <Checkbox
                  id={`${id}-local`}
                  checked={saveLocally}
                  disabled={busy}
                  onCheckedChange={(checked) => setSaveLocally(checked === true)}
                />
                <FieldLabel htmlFor={`${id}-local`}>{t('lore.materials.saveLocally')}</FieldLabel>
              </div>
              <FieldDescription>{t('lore.materials.saveLocallyHint')}</FieldDescription>
            </Field>
            <Button type="submit" disabled={busy || !valid}>
              {t(busy ? 'common.saving' : 'common.save')}
            </Button>
          </FieldGroup>
        </form>
      </DialogContent>
    </Dialog>
  )
}
