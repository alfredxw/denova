import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AudioLines, ImagePlus, MoreHorizontal, Plus, Search, Upload } from 'lucide-react'
import { toast } from 'sonner'
import {
  Attachment,
  AttachmentAction,
  AttachmentActions,
  AttachmentContent,
  AttachmentDescription,
  AttachmentMedia,
  AttachmentTitle,
  AttachmentTrigger,
} from '@/components/ui/attachment'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import {
  createAgentCommandID,
  generateLoreItemImage,
  generateLoreItemSpeech,
  getLoreAssets,
  mutateLoreMaterial,
  projectFileAssetURL,
  uploadLoreItemMaterial,
  type LoreAsset,
  type LoreItem,
  type LoreMaterial,
  type LoreMaterialMutation,
} from '@/lib/api'
import { notifyLoreUpdated } from './events'
import { LoreMaterialDialog, MaterialAudio } from './LoreMaterialDialog'
import { LoreMaterialGenerateDialog } from './LoreMaterialGenerateDialog'
import { LoreMaterialSpeechDialog } from './LoreMaterialSpeechDialog'
import { useImageModelConfigured } from '@/features/settings/use-image-model-configured'
import { useSpeechSettings } from '@/features/speech/hooks'
import { speechConfigError } from '@/features/speech/player'

export function LoreMaterialsPanel({
  projectId,
  item,
  onChanged,
  onInspectMaterial,
}: {
  projectId: string
  item: LoreItem
  onChanged: (item: LoreItem) => void
  onInspectMaterial?: (material: LoreMaterial) => void
}) {
  const { t } = useTranslation()
  const imageConfigured = useImageModelConfigured(projectId)
  const { settings: speechSettings } = useSpeechSettings()
  const speechConfigured = !speechConfigError(speechSettings)
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState('all')
  const [busy, setBusy] = useState(false)
  const [selected, setSelected] = useState<string | null>(null)
  const [generateOpen, setGenerateOpen] = useState<'image' | 'speech' | null>(null)
  const [pickerOpen, setPickerOpen] = useState(false)
  const [assets, setAssets] = useState<LoreAsset[] | null>(null)
  const [assetsError, setAssetsError] = useState('')
  const [pickerQuery, setPickerQuery] = useState('')
  const [uploadProgress, setUploadProgress] = useState<{ done: number; total: number } | null>(null)
  const [failed, setFailed] = useState<Array<{ file: File; error: string }>>([])
  const input = useRef<HTMLInputElement>(null)
  const alive = useRef(true)
  const running = useRef(false)
  const onChangedRef = useRef(onChanged)
  onChangedRef.current = onChanged
  useEffect(() => {
    alive.current = true
    return () => {
      alive.current = false
    }
  }, [])
  const materials = item.resolved_materials ?? []
  const selectedMaterial = materials.find((material) => material.id === selected)
  const accept = (saved: LoreItem) => {
    if (alive.current) onChangedRef.current(saved)
    notifyLoreUpdated({ projectId, ids: [saved.id], source: 'materials' })
  }
  const perform = async (operation: () => Promise<LoreItem>) => {
    if (running.current) return false
    running.current = true
    setBusy(true)
    try {
      accept(await operation())
      return true
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('lore.materials.failed'))
      return false
    } finally {
      running.current = false
      if (alive.current) setBusy(false)
    }
  }
  const mutate = (mutation: LoreMaterialMutation) =>
    perform(() => mutateLoreMaterial(projectId, item.id, mutation))
  const upload = async (files: File[]) => {
    if (!files.length || running.current) return
    running.current = true
    setBusy(true)
    setFailed([])
    const errors: Array<{ file: File; error: string }> = []
    let done = 0
    for (const file of files) {
      if (alive.current) setUploadProgress({ done, total: files.length })
      try {
        accept(await uploadLoreItemMaterial(projectId, item.id, file))
      } catch (error) {
        errors.push({
          file,
          error: error instanceof Error ? error.message : t('lore.materials.failed'),
        })
      }
      done++
    }
    running.current = false
    if (alive.current) {
      setFailed(errors)
      setUploadProgress(null)
      setBusy(false)
    }
    if (errors.length)
      toast.error(
        t('lore.materials.uploadSummary', {
          success: files.length - errors.length,
          failed: errors.length,
        }),
      )
    else toast.success(t('lore.materials.uploaded', { count: files.length }))
  }
  const loadAssets = async () => {
    setAssets(null)
    setAssetsError('')
    try {
      const result = await getLoreAssets(projectId)
      if (alive.current) setAssets(result)
    } catch (error) {
      if (alive.current)
        setAssetsError(error instanceof Error ? error.message : t('lore.materials.failed'))
    }
  }
  const matches = (material: LoreMaterial) =>
    [material.name, material.description ?? '', material.original_name]
      .join('\n')
      .toLowerCase()
      .includes(query.toLowerCase())
  const filtered = materials.filter(matches)
  return (
    <div className="flex min-w-0 flex-col gap-4 p-3 sm:p-4">
      <div className="flex flex-wrap items-center gap-2">
        <InputGroup className="min-w-40 flex-1">
          <InputGroupAddon>
            <Search />
          </InputGroupAddon>
          <InputGroupInput
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            aria-label={t('lore.materials.search')}
            placeholder={t('lore.materials.search')}
          />
        </InputGroup>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button disabled={busy}>
              <Plus data-icon="inline-start" />
              {t('lore.materials.add')}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuGroup>
              <DropdownMenuItem onSelect={() => input.current?.click()}>
                {t('lore.materials.upload')}
              </DropdownMenuItem>
              <DropdownMenuItem
                onSelect={() => {
                  setPickerOpen(true)
                  void loadAssets()
                }}
              >
                {t('lore.materials.chooseExisting')}
              </DropdownMenuItem>
              {imageConfigured && (
                <DropdownMenuItem onSelect={() => setGenerateOpen('image')}>
                  {t('lore.materials.generate')}
                </DropdownMenuItem>
              )}
              {speechConfigured && (
                <DropdownMenuItem onSelect={() => setGenerateOpen('speech')}>
                  {t('lore.materials.generateSpeech')}
                </DropdownMenuItem>
              )}
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
        <input
          ref={input}
          className="hidden"
          type="file"
          accept="image/png,image/jpeg,audio/mpeg,audio/wav,.mp3,.wav"
          multiple
          aria-label={t('lore.materials.upload')}
          onChange={(event) => {
            const files = Array.from(event.currentTarget.files ?? [])
            event.currentTarget.value = ''
            void upload(files)
          }}
        />
      </div>
      <ToggleGroup
        type="single"
        value={filter}
        onValueChange={(value) => {
          if (value) setFilter(value)
        }}
        variant="outline"
        aria-label={t('lore.materials.filter')}
      >
        <ToggleGroupItem value="all">{t('lore.materials.all')}</ToggleGroupItem>
        <ToggleGroupItem value="image">{t('lore.materials.images')}</ToggleGroupItem>
        <ToggleGroupItem value="audio">{t('lore.materials.audio')}</ToggleGroupItem>
      </ToggleGroup>
      {uploadProgress && (
        <div role="status" className="flex flex-col gap-2">
          <span className="text-sm">{t('lore.materials.uploadProgress', uploadProgress)}</span>
          <Progress value={(uploadProgress.done / uploadProgress.total) * 100} />
        </div>
      )}
      {failed.map(({ file, error }, index) => (
        <Attachment key={`${file.name}:${index}`} state="error" className="w-full">
          <AttachmentMedia>
            <Upload />
          </AttachmentMedia>
          <AttachmentContent>
            <AttachmentTitle>{file.name}</AttachmentTitle>
            <AttachmentDescription>{error}</AttachmentDescription>
          </AttachmentContent>
          <AttachmentActions>
            <Button
              size="sm"
              variant="outline"
              disabled={busy}
              onClick={() => {
                void upload(failed.map((entry) => entry.file))
              }}
            >
              {t('lore.materials.retryFailed')}
            </Button>
          </AttachmentActions>
        </Attachment>
      ))}
      {(['image', 'audio'] as const).map((kind) => {
        if (filter !== 'all' && filter !== kind) return null
        const group = filtered.filter((material) => material.mime_type.startsWith(`${kind}/`))
        if (!group.length) return null
        return (
          <section
            key={kind}
            className="flex min-w-0 flex-col gap-2"
            aria-label={t(kind === 'image' ? 'lore.materials.images' : 'lore.materials.audio')}
          >
            <h3 className="text-sm font-medium">
              {t(kind === 'image' ? 'lore.materials.images' : 'lore.materials.audio')} ·{' '}
              {group.length}
            </h3>
            <div
              className={
                kind === 'image'
                  ? 'grid grid-cols-[repeat(auto-fill,minmax(min(100%,10rem),1fr))] gap-3'
                  : 'flex flex-col gap-2'
              }
            >
              {group.map((material) => (
                <MaterialCard
                  key={material.id}
                  projectId={projectId}
                  material={material}
                  cover={material.path === item.image?.image_path}
                  disabled={busy}
                  onOpen={() => setSelected(material.id)}
                  onMutate={mutate}
                />
              ))}
            </div>
          </section>
        )
      })}
      {!filtered.some(
        (material) => filter === 'all' || material.mime_type.startsWith(`${filter}/`),
      ) && (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>
              {t(materials.length ? 'lore.materials.noMatches' : 'lore.materials.empty')}
            </EmptyTitle>
            <EmptyDescription>{t('lore.materials.emptyHint')}</EmptyDescription>
          </EmptyHeader>
          <div className="flex flex-wrap justify-center gap-2">
            <Button variant="outline" disabled={busy} onClick={() => input.current?.click()}>
              <Upload data-icon="inline-start" />
              {t('lore.materials.upload')}
            </Button>
            {imageConfigured && (
              <Button variant="outline" disabled={busy} onClick={() => setGenerateOpen('image')}>
                <ImagePlus data-icon="inline-start" />
                {t('lore.materials.generate')}
              </Button>
            )}
            {speechConfigured && (
              <Button variant="outline" disabled={busy} onClick={() => setGenerateOpen('speech')}>
                <AudioLines data-icon="inline-start" />
                {t('lore.materials.generateSpeech')}
              </Button>
            )}
          </div>
        </Empty>
      )}
      {selectedMaterial && (
        <LoreMaterialDialog
          key={selectedMaterial.id}
          projectId={projectId}
          itemName={item.name}
          material={selectedMaterial}
          cover={selectedMaterial.path === item.image?.image_path}
          busy={busy}
          onClose={() => setSelected(null)}
          onMutate={mutate}
          onInspect={onInspectMaterial}
        />
      )}
      {generateOpen === 'image' && (
        <LoreMaterialGenerateDialog
          busy={busy}
          onClose={() => setGenerateOpen(null)}
          onGenerate={(request) =>
            perform(() =>
              generateLoreItemImage(projectId, item.id, {
                ...request,
                command_id: request.mode === 'agent' ? createAgentCommandID() : undefined,
              }),
            )
          }
        />
      )}
      {generateOpen === 'speech' && (
        <LoreMaterialSpeechDialog
          itemName={item.name}
          busy={busy}
          onClose={() => setGenerateOpen(null)}
          onGenerate={(request) => perform(() => generateLoreItemSpeech(projectId, item.id, request))}
        />
      )}
      <Dialog open={pickerOpen} onOpenChange={setPickerOpen}>
        <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t('lore.materials.chooseExisting')}</DialogTitle>
            <DialogDescription>{t('lore.materials.chooseHint')}</DialogDescription>
          </DialogHeader>
          <InputGroup>
            <InputGroupAddon>
              <Search />
            </InputGroupAddon>
            <InputGroupInput
              value={pickerQuery}
              onChange={(event) => setPickerQuery(event.target.value)}
              aria-label={t('lore.materials.search')}
              placeholder={t('lore.materials.search')}
            />
          </InputGroup>
          {assetsError ? (
            <div role="alert">
              <p>{assetsError}</p>
              <Button variant="outline" onClick={() => void loadAssets()}>
                {t('common.retry')}
              </Button>
            </div>
          ) : !assets ? (
            <Skeleton className="h-32 w-full" />
          ) : (
            <div className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,9rem),1fr))] gap-2">
              {assets
                .filter(
                  (asset) =>
                    !materials.some((material) => material.path === asset.path) &&
                    asset.original_name.toLowerCase().includes(pickerQuery.toLowerCase()),
                )
                .map((asset) => (
                  <MaterialCard
                    key={asset.id}
                    projectId={projectId}
                    material={{ ...asset, name: asset.original_name }}
                    disabled={busy}
                    onOpen={() => {
                      void mutate({ op: 'link', asset_id: asset.id }).then((ok) => {
                        if (ok) setPickerOpen(false)
                      })
                    }}
                  />
                ))}
              {!assets.some(
                (asset) =>
                  !materials.some((material) => material.path === asset.path) &&
                  asset.original_name.toLowerCase().includes(pickerQuery.toLowerCase()),
              ) && (
                <Empty className="col-span-full">
                  <EmptyHeader>
                    <EmptyTitle>{t('lore.materials.noMatches')}</EmptyTitle>
                  </EmptyHeader>
                </Empty>
              )}
            </div>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}

function MaterialCard({
  projectId,
  material,
  cover,
  disabled,
  onOpen,
  onMutate,
}: {
  projectId: string
  material: LoreMaterial
  cover?: boolean
  disabled?: boolean
  onOpen: () => void
  onMutate?: (mutation: LoreMaterialMutation) => Promise<boolean>
}) {
  const { t } = useTranslation()
  const image = material.mime_type.startsWith('image/')
  return (
    <Attachment
      orientation={image ? 'vertical' : 'horizontal'}
      className="w-full has-data-[slot=attachment-content]:w-full"
      aria-label={material.name}
    >
      <AttachmentMedia variant={image ? 'image' : 'icon'}>
        {image ? (
          <img src={projectFileAssetURL(projectId, material.path)} alt="" loading="lazy" />
        ) : (
          <AudioLines />
        )}
      </AttachmentMedia>
      <AttachmentContent>
        <AttachmentTitle>{material.name}</AttachmentTitle>
        <AttachmentDescription>
          {material.description || material.original_name}
        </AttachmentDescription>
        {cover && <Badge variant="secondary">{t('lore.materials.cover')}</Badge>}
      </AttachmentContent>
      <AttachmentTrigger
        aria-label={t('lore.materials.open', { name: material.name })}
        disabled={disabled}
        onClick={onOpen}
      />
      {onMutate && (
        <AttachmentActions>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <AttachmentAction
                aria-label={t('lore.materials.actions', { name: material.name })}
                disabled={disabled}
              >
                <MoreHorizontal />
              </AttachmentAction>
            </DropdownMenuTrigger>
            <DropdownMenuContent>
              <DropdownMenuGroup>
                <DropdownMenuItem onSelect={onOpen}>{t('lore.materials.details')}</DropdownMenuItem>
                {image && (
                  <DropdownMenuItem
                    onSelect={() =>
                      void onMutate({ op: 'cover', asset_id: cover ? '' : material.id })
                    }
                  >
                    {t(cover ? 'lore.materials.clearCover' : 'lore.materials.setCover')}
                  </DropdownMenuItem>
                )}
                <DropdownMenuItem
                  onSelect={() => void onMutate({ op: 'remove', asset_id: material.id })}
                >
                  {t('lore.materials.remove')}
                </DropdownMenuItem>
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </AttachmentActions>
      )}
      {!image && onMutate && (
        <AttachmentActions className="w-full">
          <MaterialAudio src={projectFileAssetURL(projectId, material.path)} name={material.name} />
        </AttachmentActions>
      )}
    </Attachment>
  )
}
