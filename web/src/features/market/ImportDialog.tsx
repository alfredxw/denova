import { useEffect, useState } from 'react'
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
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ImportResourcePicker } from './ImportResourcePicker'
import { CompatibilityReport } from '@/components/workbench/CharacterCardImportDialog'
import { getBooks, type BookRecord } from '@/lib/api'
import {
  dependencySelection,
  discardPreview,
  type ImportSelection,
  exchange,
  previewSource,
  type Installation,
  type Plan,
  type Preview,
  type Source,
} from './api'

export interface ImportDialogProps {
  source?: Source
  preview?: Preview
  selection?: ImportSelection
  previewOwner?: 'dialog' | 'caller'
  installation?: Installation
  projectID?: string
  initialScope?: string
  onClose: () => void
  onInstalled: (installation: Installation) => void | Promise<void>
}
export function ImportDialog({
  source,
  preview: initialPreview,
  selection: initialSelection,
  previewOwner = 'dialog',
  installation,
  projectID: defaultProject,
  initialScope = 'user',
  onClose,
  onInstalled,
}: ImportDialogProps) {
  const { t } = useTranslation()
  const [preview, setPreview] = useState(initialPreview)
  const [candidateID, setCandidateID] = useState(
    initialSelection?.candidateID || initialPreview?.candidates.find(
      (candidate) =>
        !installation || candidate.package.id === installation.package.id,
    )?.candidate_id || '',
  )
  const [selected, setSelected] = useState(
    initialSelection?.resourceIDs || initialPreview?.candidates
      .find(
        (candidate) =>
          !installation || candidate.package.id === installation.package.id,
      )
      ?.resources.filter(
        (resource) =>
          !installation ||
          installation.bindings.some(
            (binding) => binding.resource_id === resource.id,
          ),
      )
      .map((r) => r.id) || [],
  )
  const [plan, setPlan] = useState<Plan>()
  const [sourceKind, setSourceKind] = useState('github')
  const [url, setURL] = useState(source?.url || '')
  const [ref, setRef] = useState(source?.ref || '')
  const [path, setPath] = useState(source?.path || '')
  const [file, setFile] = useState<File>()
  const [projectID, setProjectID] = useState(
    installation?.project_id || defaultProject || '',
  )
  const [scope, setScope] = useState(initialScope)
  const [books, setBooks] = useState<BookRecord[]>([])
  const [grants, setGrants] = useState<Record<string, string[]>>({})
  const [names, setNames] = useState<Record<string, string>>({})
  const [replace, setReplace] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const candidate = preview?.candidates.find(
    (c) => c.candidate_id === candidateID,
  )
  const chosen = dependencySelection(candidate?.resources || [], selected)
  const resources =
    candidate?.resources.filter((r) => chosen.includes(r.id)) || []
  const needsProject =
    resources.some((r) =>
      ['lore.item', 'game.opening', 'project.cover'].includes(r.kind),
    ) || (resources.some((r) => r.kind === 'skill') && scope === 'workspace')
  const missingConsent = resources.some((r) =>
    r.extension?.manifest.permissions.required.some(
      (p) => !(grants[r.id] || []).includes(p),
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
      setError(
        error instanceof Error
          ? error.message
          : t('market.errors.operationFailed'),
      )
    } finally {
      setBusy(false)
    }
  }
  const loadPreview = async () => {
    const result = await previewSource(
      file ||
        source || {
          kind: sourceKind as Source['kind'],
          url,
          ref: ref || undefined,
          path: path || undefined,
        },
    )
    setPreview(result)
    setCandidateID(result.candidates[0].candidate_id)
    setSelected(result.candidates[0].resources.map((r) => r.id))
  }
  const close = () => {
    if (busy) return
    if (preview && previewOwner === 'dialog') discardPreview(preview)
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
          if (busy) event.preventDefault()
        }}
      >
        <DialogHeader>
          <DialogTitle>
            {t(plan ? 'market.import.confirmTitle' : 'market.import.title')}
          </DialogTitle>
          <DialogDescription>
            {t(plan ? 'market.import.confirmHelp' : 'market.import.help')}
          </DialogDescription>
        </DialogHeader>
        {preview?.character && (
          <CompatibilityReport preview={preview.character} />
        )}
        {!preview && (
          <FieldGroup>
            {!source && (
              <Field>
                <FieldLabel>{t('market.import.source')}</FieldLabel>
                <Select
                  value={sourceKind}
                  onValueChange={(value) => {
                    setSourceKind(value)
                    setFile(undefined)
                  }}
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="github">GitHub</SelectItem>
                    <SelectItem value="https_zip">
                      {t('market.import.url')}
                    </SelectItem>
                    <SelectItem value="file">
                      {t('market.import.file')}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </Field>
            )}
            {sourceKind === 'file' && !source ? (
              <Field>
                <FieldLabel htmlFor="market-file">
                  {t('market.import.file')}
                </FieldLabel>
                <Input
                  id="market-file"
                  type="file"
                  accept=".zip,.png,.json"
                  onChange={(event) => setFile(event.target.files?.[0])}
                />
              </Field>
            ) : (
              <>
                <Field>
                  <FieldLabel htmlFor="market-url">
                    {t('market.import.url')}
                  </FieldLabel>
                  <Input
                    id="market-url"
                    value={url}
                    onChange={(event) => setURL(event.target.value)}
                    readOnly={!!source}
                    placeholder="https://github.com/owner/repository"
                  />
                </Field>
                {(source?.kind === 'github' ||
                  (!source && sourceKind === 'github')) && (
                  <div className="grid gap-4 sm:grid-cols-2">
                    <Field>
                      <FieldLabel htmlFor="market-ref">
                        {t('market.import.ref')}
                      </FieldLabel>
                      <Input
                        id="market-ref"
                        value={ref}
                        readOnly={!!source}
                        onChange={(event) => setRef(event.target.value)}
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="market-path">
                        {t('market.import.path')}
                      </FieldLabel>
                      <Input
                        id="market-path"
                        value={path}
                        readOnly={!!source}
                        onChange={(event) => setPath(event.target.value)}
                      />
                    </Field>
                  </div>
                )}
              </>
            )}
          </FieldGroup>
        )}
        {preview && !plan && (
          <FieldGroup>
            {preview.candidates.length > 1 && !initialSelection && <Field>
              <FieldLabel>{t('market.import.package')}</FieldLabel>
              <Select
                value={candidateID}
                onValueChange={(id) => {
                  setCandidateID(id)
                  setSelected(
                    preview.candidates
                      .find((c) => c.candidate_id === id)
                      ?.resources.map((r) => r.id) || [],
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
            {!initialSelection && candidate && <ImportResourcePicker previewID={preview.preview_id} candidate={candidate} selected={selected} onChange={setSelected} />}
            {initialSelection && <div className="space-y-2 rounded-lg border p-3">
              <p className="text-sm font-medium">{candidate?.package.name} · {t('market.export.selected', { count: chosen.length })}</p>
              <ul className="space-y-1 text-sm text-muted-foreground">{resources.map((resource) => <li key={resource.id} className="[overflow-wrap:anywhere]">{resource.name} · {t(`market.kinds.${resource.kind}`)}</li>)}</ul>
              <Button variant="ghost" size="sm" onClick={close}>{t('market.contents.editSelection')}</Button>
            </div>}
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
                            checked={(grants[resource.id] || []).includes(
                              permission,
                            )}
                            onCheckedChange={(checked) =>
                              setGrants((current) => ({
                                ...current,
                                [resource.id]: checked
                                  ? [
                                      ...(current[resource.id] || []),
                                      permission,
                                    ]
                                  : (current[resource.id] || []).filter(
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
            {needsProject && (
              <Field>
                <FieldLabel>{t('market.import.project')}</FieldLabel>
                <Select
                  value={projectID}
                  onValueChange={setProjectID}
                  disabled={!!installation}
                >
                  <SelectTrigger className="w-full">
                    <SelectValue
                      placeholder={t('market.import.selectProject')}
                    />
                  </SelectTrigger>
                  <SelectContent>
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
                  </SelectContent>
                </Select>
              </Field>
            )}
            {(installation ||
              resources.some(
                (resource) => resource.kind === 'project.cover',
              )) && (
              <Field orientation="horizontal">
                <Checkbox
                  id="replace-modified"
                  checked={replace}
                  onCheckedChange={(checked) => setReplace(checked === true)}
                />
                <FieldLabel htmlFor="replace-modified">
                  {t('market.import.replaceModified')}
                </FieldLabel>
              </Field>
            )}
          </FieldGroup>
        )}
        {plan && (
          <div className="space-y-3">
            <h3 className="font-medium">{plan.installation.package.name}</h3>
            <ul className="divide-y rounded-lg border">
              {plan.items.map((item) => (
                <li
                  key={item.resource_id}
                  className="flex flex-wrap justify-between gap-2 p-3"
                >
                  <span className="break-words">
                    {item.name || item.local.id}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {t(`market.actions.${item.action}`)} ·{' '}
                    {t(`market.kinds.${item.local.kind}`)}
                  </span>
                </li>
              ))}
            </ul>
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
        <DialogFooter>
          <Button
            variant="outline"
            disabled={busy}
            onClick={plan ? () => setPlan(undefined) : close}
          >
            {t(plan ? 'market.back' : 'common.cancel')}
          </Button>
          <Button
            disabled={
              busy ||
              (!preview && !url && !file) ||
              (!!preview &&
                !plan &&
                (!chosen.length ||
                  (needsProject && !projectID) ||
                  missingConsent))
            }
            onClick={() =>
              void run(async () => {
                if (!preview) await loadPreview()
                else if (!plan)
                  setPlan(
                    await exchange<Plan>('/plans', {
                      preview_id: preview.preview_id,
                      candidate_id: candidateID,
                      resources: selected,
                      project_id: projectID,
                      skill_scope: scope,
                      installation_id: installation?.installation_id,
                      grants,
                      names,
                      replace_modified: replace,
                      update_mode: installation?.update_mode || 'manual',
                    }),
                  )
                else {
                  const installed = await exchange<Installation>(
                    `/plans/${plan.plan_id}/apply`,
                    {},
                  )
                  toast.success(t('market.import.done'))
                  await onInstalled(installed)
                  if (previewOwner === 'dialog') discardPreview(preview)
                  onClose()
                }
              })
            }
          >
            {t(
              busy
                ? 'market.working'
                : plan
                  ? 'market.import.install'
                  : preview
                    ? 'market.import.review'
                    : 'market.import.preview',
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
