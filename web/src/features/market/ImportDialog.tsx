import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import type { CatalogEntry } from '@/features/platform/api'
import { ImportSourceFields, type ResourceInput } from './ImportSourceFields'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { UpdateReview } from './UpdateReview'
import { ImportResourcePicker } from './ImportResourcePicker'
import { useBookCreation } from '@/components/workbench/book-creation'
import { CompatibilityReport } from '@/components/workbench/CharacterCardImportDialog'
import { createBook, getBooks, type BookRecord } from '@/lib/api'
import { notifyLoreUpdated } from '@/features/lore/events'
import { invalidateSettingsCache } from '@/features/settings/api'
import type { GameDefaultField } from '@/features/interactive/game-creation-defaults'
import { ImportGameDefaults, packageDefaultOptions } from './ImportGameDefaults'
import {
  dependencySelection,
  discardPreview,
  exchange,
  previewSource,
  type Installation,
  type Plan,
  type PlanRequest,
  type Preview,
  type Source,
} from './api'

export interface ImportDialogProps {
  source?: Source
  preview?: Preview
  installation?: Installation
  projectID?: string
  initialScope?: string
  installedExtensions?: CatalogEntry[]
  onClose: () => void
  onInstalled: (installation: Installation) => void | Promise<void>
}
export function ImportDialog({
  source,
  preview: initialPreview,
  installation,
  projectID: defaultProject,
  initialScope = 'user',
  installedExtensions = [],
  onClose,
  onInstalled,
}: ImportDialogProps) {
  const { t } = useTranslation()
  const bookCreation = useBookCreation()
  const [preview, setPreview] = useState(initialPreview)
  const [candidateID, setCandidateID] = useState(
    initialPreview?.candidates.find(
      (candidate) =>
        !installation || candidate.package.id === installation.package.id,
    )?.candidate_id || '',
  )
  const [selected, setSelected] = useState(
    initialPreview?.candidates
      .find(
        (candidate) =>
          !installation || candidate.package.id === installation.package.id,
      )
      ?.resources.filter(
        (resource) =>
          resource.kind !== 'project.creator' &&
          (!installation || installation.bindings.some(
            (binding) => binding.resource_id === resource.id,
          )),
      )
      .map((r) => r.id) || [],
  )
  const [plan, setPlan] = useState<Plan>()
  const [planRequest, setPlanRequest] = useState<PlanRequest>()
  const [resolutions, setResolutions] = useState<Record<string, Record<string, string>>>({})
  const [input, setInput] = useState<ResourceInput>(installation?.source.kind === 'file' ? { kind: 'file' } : { kind: 'github', url: '' })
  const [projectID, setProjectID] = useState(
    installation?.project_id || defaultProject || '',
  )
  const [bookDestination, setBookDestination] = useState<'new' | 'existing'>('existing')
  const [bookTitle, setBookTitle] = useState('')
  const [createdProjectID, setCreatedProjectID] = useState('')
  const [scope, setScope] = useState(initialScope)
  const [books, setBooks] = useState<BookRecord[]>([])
  const [grants, setGrants] = useState<Record<string, string[]>>({})
  const [names, setNames] = useState<Record<string, string>>({})
  const [sharedResources, setSharedResources] = useState<'reuse' | 'copy'>('reuse')
  const [replace, setReplace] = useState(false)
  const [defaultsSelection, setDefaultsSelection] = useState<GameDefaultField[] | null>(null)
  const [busy, setBusy] = useState(!!source && !initialPreview)
  const alive = useRef(false)
  const pending = useRef<Promise<void> | undefined>(undefined)
  const downloadedPreview = useRef<Preview | undefined>(undefined)
  const [error, setError] = useState('')
  const needsPlanReview = plan?.updates?.some(item => item.conflict &&
    (resolutions[item.resource_id]?.[item.member_id || ''] || '') !== (item.resolution || ''),
  ) ?? false
  const candidate = preview?.candidates.find(
    (c) => c.candidate_id === candidateID,
  )
  const requested = dependencySelection(candidate?.resources || [], selected)
  const canCreateBook = !installation && candidate?.resources.some(
    (r) => ['lore.collection', 'project.creator'].includes(r.kind) && requested.includes(r.id),
  )
  const creatingBook = canCreateBook && bookDestination === 'new'
  // Retain the new-book cover policy when retrying a failed plan for that book.
  const includeCharacterBookCover = creatingBook || (!!createdProjectID && projectID === createdProjectID)
  const availableResources = candidate?.resources.filter(
    (r) => !preview?.character || r.kind !== 'project.cover' || includeCharacterBookCover,
  ) || []
  const chosen = requested.filter((id) => availableResources.some((r) => r.id === id))
  const resources = availableResources.filter((r) => chosen.includes(r.id))
  const defaultOptions = candidate ? packageDefaultOptions(candidate, chosen) : []
  const defaultFields = (defaultsSelection ?? (creatingBook ? defaultOptions.filter(option => option.available).map(option => option.field) : []))
    .filter(field => defaultOptions.some(option => option.field === field && option.available))
  const needsProject =
    resources.some((r) =>
      ['lore.collection', 'game.openings', 'project.cover', 'project.creator'].includes(r.kind),
    ) || (resources.some((r) => r.kind === 'skill') && scope === 'workspace') || defaultFields.length > 0
  const permissionGrants = (resource: Preview['candidates'][number]['resources'][number]) => {
    if (grants[resource.id]) return grants[resource.id]
    const extension = installation && resource.extension
      ? installedExtensions.find(item => !item.removed && item.kind === resource.extension?.kind && item.id === resource.extension.manifest.id)
      : undefined
    const release = extension?.releases.find(item => item.ref.releaseId === extension.currentRelease)
    const permissions = resource.extension?.manifest.permissions
    return (release?.grants ?? extension?.grants ?? []).filter(permission => permissions?.required.includes(permission) || permissions?.optional?.includes(permission))
  }
  const sourceReady = source || (input.kind === 'file' ? input.file : input.kind === 'directory' ? input.directory.trim() : input.url.trim())
  const missingConsent = resources.some((r) =>
    r.extension?.manifest.permissions.required.some(
      (p) => !permissionGrants(r).includes(p),
    ),
  )
  useEffect(() => {
    let alive = true
    void getBooks()
      .then((items) => {
        if (alive) setBooks(items)
      })
      .catch(() => {
        if (alive) setError(t('market.errors.projectsUnavailable'))
      })
    return () => {
      alive = false
    }
  }, [t])
  const run = async (action: () => Promise<void>) => {
    setBusy(true)
    setError('')
    try {
      await action()
    } catch (error) {
      console.error('[market] import operation failed', error)
      if (alive.current) setError(
        error instanceof Error
          ? error.message
          : t('market.errors.operationFailed'),
      )
    } finally {
      if (alive.current) setBusy(false)
    }
  }
  const loadPreview = () => {
    if (pending.current) return pending.current
    pending.current = (async () => {
      const result = await previewSource(source || (input.kind === 'file' ? input.file! : input))
      if (!alive.current) { discardPreview(result); return }
      downloadedPreview.current = result
      setPreview(result)
      const candidate = result.candidates.find(item => item.package.id === installation?.package.id) ?? result.candidates[0]
      setCandidateID(candidate.candidate_id)
      setSelected(candidate.resources.filter(resource => resource.kind !== 'project.creator' &&
        (!installation || installation.bindings.some(binding => binding.resource_id === resource.id))).map(resource => resource.id))
    })().finally(() => { pending.current = undefined })
    return pending.current
  }
  useEffect(() => {
    alive.current = true
    // A supplied source means the user already chose Get. Keep its download
    // in the import flow and reuse the request during StrictMode effect replay.
    if (source && !initialPreview) void run(loadPreview)
    return () => {
      alive.current = false
      if (downloadedPreview.current) {
        discardPreview(downloadedPreview.current)
        downloadedPreview.current = undefined
      }
    }
  }, [])
  const close = () => {
    if (busy && preview) return
    if (preview) {
      discardPreview(preview)
      downloadedPreview.current = undefined
    }
    onClose()
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) close()
      }}
    >
      <DialogContent
        className="max-h-[min(90dvh,56rem)] overflow-y-auto"
        onInteractOutside={(event) => {
          if (busy && preview) event.preventDefault()
        }}
      >
        <DialogHeader>
          <DialogTitle>
            {t(plan ? 'market.import.confirmTitle' : installation ? 'market.update.review' : 'market.import.title')}
          </DialogTitle>
          <DialogDescription>
            {t(plan ? 'market.import.confirmHelp' : installation ? 'market.update.sourceHelp' : source ? 'market.contents.selectHelp' : 'market.import.help')}
          </DialogDescription>
        </DialogHeader>
        {preview?.character && (
          <CompatibilityReport preview={preview.character} />
        )}
        {!preview && source && <p role="status" className="text-sm text-muted-foreground">{t(busy ? 'market.import.downloading' : 'market.contents.failed')}</p>}
        {!preview && !source && <ImportSourceFields value={input} onChange={setInput} busy={busy} run={run} onImportedSource={onClose} />}
        {preview && !plan && (
          <FieldGroup>
            {preview.candidates.length > 1 && <Field>
              <FieldLabel>{t('market.import.package')}</FieldLabel>
              <Select
                value={candidateID}
                onValueChange={(id) => {
                  setCandidateID(id)
                  setReplace(false)
                  setDefaultsSelection(null)
                  setSelected(
                    preview.candidates
                      .find((c) => c.candidate_id === id)
                      ?.resources.filter((r) => r.kind !== 'project.creator').map((r) => r.id) || [],
                  )
                  setGrants({})
                  setNames({})
                }}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {preview.candidates.map((c) => (
                    <SelectItem key={c.candidate_id} value={c.candidate_id}>
                      {c.package.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>}
            {candidate && <ImportResourcePicker previewID={preview.preview_id} candidate={{ ...candidate, resources: availableResources }} selected={selected} onChange={(ids) => { setSelected(ids); setReplace(false) }} />}

            {!installation && resources.some(resource => resource.kind.startsWith('preset.') || resource.kind === 'style.reference' || resource.kind === 'skill' && scope === 'user') && (
              <Field>
                <FieldLabel htmlFor="market-shared-resources">{t('market.import.sharedResources')}</FieldLabel>
                <Select value={sharedResources} onValueChange={(value: 'reuse' | 'copy') => setSharedResources(value)}>
                  <SelectTrigger id="market-shared-resources" className="w-full"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="reuse">{t('market.import.reuseShared')}</SelectItem>
                    <SelectItem value="copy">{t('market.import.copyShared')}</SelectItem>
                  </SelectContent>
                </Select>
                <FieldDescription>{t('market.import.sharedHelp')}</FieldDescription>
              </Field>
            )}

            <div className="space-y-3">
              {resources.filter((resource) => resource.kind === 'skill' || resource.extension).map((resource) => (
                <div key={resource.id} className="space-y-3 rounded-lg border p-3">
                  <p className="text-sm font-medium">{resource.name}</p>
                  {resource.kind === 'skill' &&
                    !installation && (
                      <Field>
                        <FieldLabel htmlFor={`name-${resource.id}`}>
                          {t('market.import.localName')}
                        </FieldLabel>
                        <Input
                          id={`name-${resource.id}`}
                          value={names[resource.id] ?? resource.name}
                          onChange={(event) =>
                            setNames((current) => ({
                              ...current,
                              [resource.id]: event.target.value,
                            }))
                          }
                        />
                      </Field>
                    )}
                  {resource.extension && (
                    <div className="space-y-2 rounded-md bg-muted p-3">
                      <p className="text-xs text-muted-foreground">
                        {t('market.import.permissions')}
                      </p>
                      {[
                        ...resource.extension.manifest.permissions.required,
                        ...(resource.extension.manifest.permissions.optional ||
                          []),
                      ].map((permission) => (
                        <Field key={permission} orientation="horizontal">
                          <Checkbox
                            id={`permission-${resource.id}-${permission}`}
                            checked={permissionGrants(resource).includes(
                              permission,
                            )}
                            onCheckedChange={(checked) =>
                              setGrants((current) => ({
                                ...current,
                                [resource.id]: checked
                                  ? [
                                      ...(current[resource.id] ?? permissionGrants(resource)),
                                      permission,
                                    ]
                                  : (current[resource.id] ?? permissionGrants(resource)).filter(
                                      (p) => p !== permission,
                                    ),
                              }))
                            }
                          />
                          <FieldLabel
                            htmlFor={`permission-${resource.id}-${permission}`}
                          >
                            {t(`platform.permission.${permission}`)}
                            {resource.extension?.manifest.permissions.required.includes(
                              permission,
                            )
                              ? ` · ${t('market.import.required')}`
                              : ''}
                          </FieldLabel>
                        </Field>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
            {resources.some((r) => r.kind === 'skill') && !installation && (
              <Field>
                <FieldLabel>{t('market.import.scope')}</FieldLabel>
                <Select value={scope} onValueChange={setScope}>
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="user">
                      {t('market.import.global')}
                    </SelectItem>
                    <SelectItem value="workspace">
                      {t('market.import.project')}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </Field>
            )}
            {canCreateBook && (
              <FieldSet>
                <FieldLegend id="market-import-mode" variant="label">{t('market.import.bookDestination')}</FieldLegend>
                <RadioGroup
                  aria-labelledby="market-import-mode"
                  value={bookDestination}
                  onValueChange={(value) => {
                    if (value === 'new' || value === 'existing') { setBookDestination(value); setDefaultsSelection(null); setReplace(false) }
                  }}
                  disabled={busy}
                >
                  <Field orientation="horizontal" data-disabled={busy}>
                    <RadioGroupItem id="market-new-book" value="new" />
                    <FieldLabel htmlFor="market-new-book">{t('market.import.newBook')}</FieldLabel>
                  </Field>
                  <Field orientation="horizontal" data-disabled={busy}>
                    <RadioGroupItem id="market-existing-book" value="existing" />
                    <FieldLabel htmlFor="market-existing-book">{t('market.import.existingBook')}</FieldLabel>
                  </Field>
                </RadioGroup>
              </FieldSet>
            )}
            {(needsProject || defaultOptions.length > 0) && !creatingBook && (
              <Field data-disabled={busy || !!installation}>
                <FieldLabel htmlFor="market-target-book">{t('market.import.project')}</FieldLabel>
                <Select
                  value={projectID}
                  onValueChange={id => { setProjectID(id); setDefaultsSelection(null); setReplace(false) }}
                  disabled={busy || !!installation}
                >
                  <SelectTrigger id="market-target-book" className="w-full min-w-0">
                    <SelectValue
                      placeholder={t('market.import.selectProject')}
                    />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {books
                        .filter((book) => book.project_id)
                        .map((book) => (
                          <SelectItem
                            key={book.project_id}
                            value={book.project_id!}
                          >
                            {book.name}
                          </SelectItem>
                        ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>
            )}
            {creatingBook && (
              <Field data-disabled={busy}>
                <FieldLabel htmlFor="market-book-title">{t('market.import.bookTitle')}</FieldLabel>
                <Input id="market-book-title" value={bookTitle} disabled={busy} onChange={(event) => setBookTitle(event.target.value)} />
                <FieldDescription>{t('market.import.newBookHelp')}</FieldDescription>
              </Field>
            )}
            {candidate && defaultOptions.length > 0 && <ImportGameDefaults candidate={candidate} resources={chosen} projectID={projectID} newBook={!!creatingBook} selected={defaultFields} onChange={setDefaultsSelection} />}
            {(!installation &&
              resources.some(
                (resource) => resource.kind === 'project.cover' || resource.kind === 'project.creator',
              )) && (
              <Field orientation="horizontal">
                <Checkbox
                  id="replace-modified"
                  checked={replace}
                  onCheckedChange={(checked) => setReplace(checked === true)}
                />
                <FieldLabel htmlFor="replace-modified">
                  {t(resources.some(r => r.kind === 'project.creator') ? 'market.import.replaceCreator' : 'market.import.replaceModified')}
                </FieldLabel>
              </Field>
            )}
          </FieldGroup>
        )}
        {plan && (
          <div className="space-y-3">
            <h3 className="font-medium">{plan.installation.package.name}</h3>
            {installation && plan.updates ? <UpdateReview items={plan.updates} resolutions={resolutions} busy={busy} onResolve={(items, choice) => {
              setResolutions(current => {
                const next = structuredClone(current)
                for (const item of items) {
                  next[item.resource_id] ??= {}
                  next[item.resource_id][item.member_id || ''] = choice
                }
                return next
              })
            }} /> : <ul className="divide-y rounded-lg border">
              {plan.items.map((item) => (
                <li
                  key={item.resource_id}
                  className="flex flex-wrap justify-between gap-2 p-3"
                >
                  <span className="break-words">
                    {item.local.kind === 'skill' ? item.local.id : item.name || item.local.id}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {t(`market.actions.${item.action}`)} ·{' '}
                    {t(`market.kinds.${item.local.kind}`)}
                  </span>
                </li>
              ))}
            </ul>}
            {needsPlanReview && <p role="status" className="text-sm text-muted-foreground">{t('market.update.choicesChanged')}</p>}
            <p className="text-xs text-muted-foreground break-all">
              {preview?.source.commit
                ? t('market.import.commit', { commit: preview.source.commit })
                : preview?.source.url || preview?.source.filename}
            </p>
          </div>
        )}
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {plan?.game_defaults_applied && <section className="space-y-2 rounded-lg border p-3">
          <p className="text-sm font-medium">{t('gameDefaults.adopt')}</p>
          <p className="text-xs text-muted-foreground">{t('gameDefaults.help')}</p>
          {defaultOptions.filter(option => plan.game_defaults_applied?.[option.field] !== undefined).map(option => <p key={option.field} className="text-sm break-words">
            {t(`gameDefaults.fields.${option.field}`)} · {option.name}
            {plan.game_defaults_before?.[option.field] !== undefined && <span className="ml-2 text-xs text-muted-foreground">{t('gameDefaults.replaceExisting')}</span>}
          </p>)}
        </section>}
        <DialogFooter>
          <Button
            variant="outline"
            disabled={busy && !!preview}
            onClick={plan ? () => setPlan(undefined) : close}
          >
            {t(plan ? 'market.back' : 'common.cancel')}
          </Button>
          <Button
            disabled={
              busy ||
              (!preview && !sourceReady) ||
              (!!preview &&
                !plan &&
                (!chosen.length ||
                  (needsProject && (creatingBook ? !bookTitle.trim() : !projectID)) ||
                  missingConsent))
            }
            onClick={() =>
              void run(async () => {
                if (!preview) await loadPreview()
                else if (!plan) {
                  let targetProject = projectID
                  if (creatingBook) {
                    if (!(await bookCreation.beforeCreate())) return
                    const created = await createBook(bookTitle.trim())
                    targetProject = created.project_id
                    setDefaultsSelection(defaultFields)
                    // Retain the created target even if planning fails, so retry never creates it twice.
                    setProjectID(targetProject)
                    setCreatedProjectID(targetProject)
                    setBookDestination('existing')
                    setBooks((current) => [...current, {
                      project_id: targetProject, path: created.workspace,
                      name: created.book_meta.title, author: created.book_meta.author || '', last_opened_at: '',
                    }])
                    // Creation already selects the book on the server, even if planning later fails.
                    await bookCreation.onCreated(created.workspace)
                  }
                  const request: PlanRequest = {
                      preview_id: preview.preview_id,
                      candidate_id: candidateID,
                      resources: selected.filter((id) => chosen.includes(id)),
                      project_id: targetProject,
                      skill_scope: scope,
                      installation_id: installation?.installation_id,
                      grants: Object.fromEntries(resources.filter(resource => resource.extension).map(resource => [resource.id, permissionGrants(resource)])),
                      names,
                      shared_resources: sharedResources,
                      replace_modified: replace,
                      update_mode: installation?.update_mode || 'manual',
                      ...(defaultFields.length ? { game_defaults_fields: defaultFields } : {}),
                  }
                  const reviewed = await exchange<Plan>('/plans', request)
                  setPlanRequest(request); setResolutions({}); setPlan(reviewed)
                } else if (needsPlanReview) {
                  const reviewed = await exchange<Plan>('/plans', { ...planRequest, resolutions })
                  setPlan(reviewed)
                } else {
                  const installed = await exchange<Installation>(
                    `/plans/${plan.plan_id}/apply`,
                    {},
                  )
                  if (plan.game_defaults_applied && installed.project_id) invalidateSettingsCache(installed.project_id)
                  // New books are opened before import; invalidate their initially empty catalog.
                  const loreProjects = new Set(installed.bindings
                    .filter(binding => binding.local.kind === 'lore.collection')
                    .map(binding => binding.local.project_id || installed.project_id)
                    .filter((id): id is string => Boolean(id)))
                  for (const projectId of loreProjects) notifyLoreUpdated({ projectId, source: 'resource-import' })
                  await onInstalled(installed)
                  toast.success(t('market.import.done'))
                  if (preview) {
                    discardPreview(preview)
                    downloadedPreview.current = undefined
                  }
                  onClose()
                }
              })
            }
          >
            {t(
              busy
                ? !preview ? 'market.import.downloading' : 'market.working'
                : plan
                  ? needsPlanReview ? 'market.import.refreshPlan' : installation ? 'market.update.apply' : resources.every(resource => resource.kind.startsWith('extension.')) ? 'market.import.installExtensions' : 'market.import.install'
                  : preview
                    ? creatingBook ? 'market.import.createAndReview' : 'market.import.review'
                    : source ? 'market.contents.retry' : 'market.import.preview',
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
