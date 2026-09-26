import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Copy, Download, ImageIcon, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
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
import { ImagePreviewDialog } from '@/components/common/ImagePreviewDialog'
import { projectFileAssetURL, type LoreMaterial, type LoreMaterialMutation } from '@/lib/api'
import { speechPlayer } from '@/features/speech/player'

let activePreview: HTMLAudioElement | null = null

export function MaterialAudio({
  src,
  name,
  preload = 'none',
}: {
  src: string
  name: string
  preload?: 'none' | 'metadata'
}) {
  const { t } = useTranslation()
  const audio = useRef<HTMLAudioElement>(null)
  useEffect(() => {
    const element = audio.current
    const unsubscribe = speechPlayer.subscribe(() => {
      if (['loading', 'playing'].includes(speechPlayer.getSnapshot().status)) element?.pause()
    })
    return () => {
      unsubscribe()
      element?.pause()
      if (activePreview === element) activePreview = null
    }
  }, [])
  return (
    <audio
      ref={audio}
      controls
      preload={preload}
      src={src}
      aria-label={name}
      className="w-full min-w-0"
      onError={() => toast.error(t('lore.materials.previewFailed'))}
      onPlay={(event) => {
        if (activePreview !== event.currentTarget) activePreview?.pause()
        speechPlayer.stop()
        activePreview = event.currentTarget
      }}
    />
  )
}

export function LoreMaterialDialog({
  projectId,
  itemName,
  material,
  cover,
  busy,
  onClose,
  onMutate,
  onInspect,
}: {
  projectId: string
  itemName: string
  material: LoreMaterial
  cover: boolean
  busy: boolean
  onClose: () => void
  onMutate: (mutation: LoreMaterialMutation) => Promise<boolean>
  onInspect?: (material: LoreMaterial) => void
}) {
  const { t } = useTranslation()
  useEffect(() => {
    activePreview?.pause()
    return () => {
      activePreview?.pause()
    }
  }, [])
  const id = useId()
  const [name, setName] = useState(material.name)
  const [description, setDescription] = useState(material.description ?? '')
  const src = projectFileAssetURL(projectId, material.path)
  const image = material.mime_type.startsWith('image/')
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose()
      }}
    >
      <DialogContent
        className="max-h-[90dvh] overflow-y-auto sm:max-w-4xl"
        onPointerDownOutside={(event) => {
          if (busy) event.preventDefault()
        }}
      >
        <DialogHeader>
          <DialogTitle>{material.name}</DialogTitle>
          <DialogDescription>
            {t('lore.materials.detailDescription', { name: itemName })}
          </DialogDescription>
        </DialogHeader>
        <div className="grid min-w-0 gap-6 md:grid-cols-2">
          <div className="flex min-w-0 flex-col justify-center gap-3">
            {image ? (
              <ImagePreviewDialog src={src} title={material.name} alt={description}>
                <button
                  type="button"
                  aria-label={t('settingPanel.loreImage.openPreview')}
                  className="overflow-hidden rounded-lg bg-muted"
                >
                  <img
                    src={src}
                    alt={material.name}
                    className="max-h-[50dvh] w-full object-contain"
                  />
                </button>
              </ImagePreviewDialog>
            ) : (
              <MaterialAudio src={src} name={material.name} preload="metadata" />
            )}
            <p className="break-words text-xs text-muted-foreground">
              {material.original_name} · {material.mime_type} ·{' '}
              {(material.size_bytes / 1024 / 1024).toFixed(2)} MB
            </p>
            <p className="text-xs text-muted-foreground">
              {t('lore.materials.source')}:{' '}
              {t(
                material.source.kind === 'generated'
                  ? 'lore.materials.generatedSource'
                  : 'lore.materials.uploadSource',
              )}
            </p>
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" size="sm" asChild>
                <a href={src} download={material.original_name}>
                  <Download data-icon="inline-start" />
                  {t('lore.materials.download')}
                </a>
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  void navigator.clipboard
                    .writeText(
                      JSON.stringify({
                        lore: itemName,
                        material_id: material.id,
                        path: material.path,
                        name,
                        description,
                      }),
                    )
                    .then(() => toast.success(t('lore.materials.copied')))
                    .catch(() => toast.error(t('lore.materials.copyFailed')))
                }}
              >
                <Copy data-icon="inline-start" />
                {t('lore.materials.copyReference')}
              </Button>
              {image && onInspect && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    onInspect({ ...material, name, description })
                    onClose()
                  }}
                >
                  {t('lore.materials.inspect')}
                </Button>
              )}
            </div>
          </div>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor={`${id}-name`}>{t('lore.materials.name')}</FieldLabel>
              <Input
                id={`${id}-name`}
                value={name}
                onChange={(event) => setName(event.target.value)}
                disabled={busy}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={`${id}-description`}>
                {t('lore.materials.description')}
              </FieldLabel>
              <Textarea
                id={`${id}-description`}
                className="min-h-32"
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                disabled={busy}
              />
              <FieldDescription>{t('lore.materials.descriptionHint')}</FieldDescription>
            </Field>
            <div className="flex flex-wrap gap-2">
              <Button
                disabled={busy}
                onClick={() => {
                  void onMutate({ op: 'update', asset_id: material.id, name, description }).then(
                    (ok) => {
                      if (ok) onClose()
                    },
                  )
                }}
              >
                {t('common.save')}
              </Button>
              {image && (
                <Button
                  variant="outline"
                  disabled={busy}
                  onClick={() => void onMutate({ op: 'cover', asset_id: cover ? '' : material.id })}
                >
                  <ImageIcon data-icon="inline-start" />
                  {t(cover ? 'lore.materials.clearCover' : 'lore.materials.setCover')}
                </Button>
              )}
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => {
                  void onMutate({ op: 'remove', asset_id: material.id }).then((ok) => {
                    if (ok) onClose()
                  })
                }}
              >
                <Trash2 data-icon="inline-start" />
                {t('lore.materials.remove')}
              </Button>
            </div>
            <FieldDescription>{t('lore.materials.removeHint')}</FieldDescription>
          </FieldGroup>
        </div>
      </DialogContent>
    </Dialog>
  )
}
