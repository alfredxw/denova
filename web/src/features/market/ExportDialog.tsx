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
import { getBooks, type BookRecord } from '@/lib/api'
import { ExportResourcePicker } from './ExportResourcePicker'
import {
  exchange,
  downloadExport,
  type ExportDefinition,
  type ExportPlan,
  type ExportResource,
  type Installation,
  type LocalRef,
} from './api'

export function ExportDialog({
  projectID: defaultProject,
  initialResources,
  installation,
  onClose,
}: {
  projectID?: string
  initialResources?: LocalRef[]
  installation?: Installation
  onClose: () => void
}) {
  const { t } = useTranslation()
  const [projectID, setProjectID] = useState(
    defaultProject || installation?.project_id || '',
  )
  const [books, setBooks] = useState<BookRecord[]>([])
  useEffect(() => {
    let active = true
    void getBooks()
      .then((items) => {
        if (active) setBooks(items)
      })
      .catch(() => {
        if (active) setError(t('market.errors.projectsUnavailable'))
      })
    return () => {
      active = false
    }
  }, [t])
  const [choices, setChoices] = useState<ExportResource[]>([])
  const [selected, setSelected] = useState<string[]>([])
  const [name, setName] = useState(installation?.package.name || '')
  const [id, setID] = useState(installation?.package.id || crypto.randomUUID())
  const [native, setNative] = useState(false)
  const [definitions, setDefinitions] = useState<ExportDefinition[]>([])
  const [definitionID, setDefinitionID] = useState('')
  const [saveDefinition, setSaveDefinition] = useState(false)
  const [plan, setPlan] = useState<ExportPlan>()
  const [requestedRefs, setRequestedRefs] = useState(
    initialResources || installation?.bindings.map((binding) => binding.local),
  )
  useEffect(() => {
    let active = true
    void exchange<ExportDefinition[]>('/export-definitions')
      .then((items) => {
        if (active) setDefinitions(items)
      })
      .catch(() => {
        if (active) setError(t('market.errors.operationFailed'))
      })
    return () => {
      active = false
    }
  }, [t])
  const [busy, setBusy] = useState(true)
  const [error, setError] = useState('')
  const key = (value: ExportResource) => JSON.stringify(value.local)
  useEffect(() => {
    let alive = true
    setBusy(true)
    setError('')
    void exchange<ExportResource[]>(
      `/export-resources${projectID ? `?project_id=${encodeURIComponent(projectID)}` : ''}`,
    )
      .then((result) => {
        if (!alive) return
        setChoices(result)
        const selection = requestedRefs
        if (selection)
          setSelected(
            result
              .filter((resource) =>
                selection.some(
                  (ref) =>
                    ref.kind === resource.local.kind &&
                    ref.scope === resource.local.scope &&
                    ref.id === resource.local.id &&
                    ref.project_id === resource.local.project_id,
                ),
              )
              .map(key),
          )
      })
      .catch(() => {
        if (alive) setError(t('market.errors.operationFailed'))
      })
      .finally(() => {
        if (alive) setBusy(false)
      })
    return () => {
      alive = false
    }
  }, [projectID, requestedRefs, t])
  const resources = choices.filter((r) => selected.includes(key(r)))
  const nativeAllowed =
    resources.length === 1 &&
    (resources[0].local.kind === 'skill' ||
      resources[0].local.kind.startsWith('extension.'))
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose()
      }}
    >
      <DialogContent className="max-h-[90dvh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{t('market.export.title')}</DialogTitle>
          <DialogDescription>{t('market.export.help')}</DialogDescription>
        </DialogHeader>
        {!plan ? (
          <FieldGroup>
            {definitions.length > 0 && (
              <Field>
                <FieldLabel>{t('market.export.saved')}</FieldLabel>
                <div className="flex gap-2">
                  <Select
                    value={definitionID || 'new'}
                    onValueChange={(value) => {
                      setDefinitionID(value === 'new' ? '' : value)
                      const definition = definitions.find(
                        (item) => item.package.id === value,
                      )
                      if (!definition) return
                      setID(definition.package.id)
                      setName(definition.package.name)
                      setNative(!!definition.native)
                      setProjectID(
                        definition.resources.find((ref) => ref.project_id)
                          ?.project_id || '',
                      )
                      setRequestedRefs(definition.resources)
                      setSaveDefinition(true)
                    }}
                  >
                    <SelectTrigger className="min-w-0 flex-1">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="new">
                        {t('market.export.new')}
                      </SelectItem>
                      {definitions.map((definition) => (
                        <SelectItem
                          key={definition.package.id}
                          value={definition.package.id}
                        >
                          {definition.package.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {definitionID && (
                    <Button
                      variant="outline"
                      disabled={busy}
                      onClick={async () => {
                        const definition = definitions.find(
                          (item) => item.package.id === definitionID,
                        )
                        if (!definition) return
                        setBusy(true)
                        try {
                          await exchange(
                            `/export-definitions/${encodeURIComponent(definitionID)}?revision=${encodeURIComponent(definition.revision)}`,
                            undefined,
                            'DELETE',
                          )
                          setDefinitions((items) =>
                            items.filter(
                              (item) => item.package.id !== definitionID,
                            ),
                          )
                          setDefinitionID('')
                          setSaveDefinition(false)
                        } catch {
                          setError(t('market.errors.operationFailed'))
                        } finally {
                          setBusy(false)
                        }
                      }}
                    >
                      {t('common.delete')}
                    </Button>
                  )}
                </div>
              </Field>
            )}
            {!installation && !initialResources && (
              <Field>
                <FieldLabel>{t('market.import.project')}</FieldLabel>
                <Select
                  value={projectID || 'global'}
                  onValueChange={(value) => {
                    setProjectID(value === 'global' ? '' : value)
                    setSelected([])
                  }}
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="global">
                      {t('market.import.global')}
                    </SelectItem>
                    {books.map((book) => (
                      <SelectItem key={book.project_id} value={book.project_id}>
                        {book.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            )}
            <Field>
              <FieldLabel htmlFor="export-name">
                {t('market.export.name')}
              </FieldLabel>
              <Input
                id="export-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="export-id">
                {t('market.export.id')}
              </FieldLabel>
              <Input
                id="export-id"
                value={id}
                onChange={(event) => setID(event.target.value)}
              />
            </Field>
            <ExportResourcePicker
              choices={choices}
              selected={resources}
              busy={busy}
              onCheckedChange={(items, checked) => {
                const keys = items.map(key)
                setSelected((current) => checked
                  ? [...new Set([...current, ...keys])]
                  : current.filter((value) => !keys.includes(value)))
              }}
            />
            {nativeAllowed && (
              <Field orientation="horizontal">
                <Checkbox
                  id="native-export"
                  checked={native}
                  onCheckedChange={(checked) => setNative(checked === true)}
                />
                <FieldLabel htmlFor="native-export">
                  {t('market.export.native')}
                </FieldLabel>
              </Field>
            )}
            <Field orientation="horizontal">
              <Checkbox
                id="save-export-definition"
                checked={saveDefinition}
                onCheckedChange={(value) => setSaveDefinition(value === true)}
              />
              <FieldLabel htmlFor="save-export-definition">
                {t('market.export.save')}
              </FieldLabel>
            </Field>
          </FieldGroup>
        ) : (
          <div className="space-y-3">
            <p className="text-sm">
              {t('market.export.summary', {
                count: plan.resources.length || resources.length,
                files: plan.files,
                size: Math.ceil(plan.bytes / 1024),
              })}
            </p>
            <ul className="max-h-72 overflow-auto rounded-lg border divide-y">
              {plan.resources.map((resource) => (
                <li key={resource.id} className="p-3 text-sm break-all">
                  {t(`market.kinds.${resource.kind}`)}
                  <div className="text-xs text-muted-foreground">
                    {resource.path}
                  </div>
                </li>
              ))}
            </ul>
            <p className="text-xs text-muted-foreground">
              {t('market.export.frozen')}
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
            onClick={plan ? () => setPlan(undefined) : onClose}
          >
            {t(plan ? 'market.back' : 'common.cancel')}
          </Button>
          <Button
            disabled={
              busy ||
              !resources.length ||
              !name.trim() ||
              !/^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$/.test(id)
            }
            onClick={async () => {
              setBusy(true)
              setError('')
              try {
                if (plan) {
                  await downloadExport(plan)
                  toast.success(t('market.export.done'))
                  onClose()
                } else {
                  const request = {
                    resources: resources.map((r) => r.local),
                    package: {
                      id,
                      name,
                      version: installation?.package.version,
                      author: installation?.package.author,
                    },
                    native: nativeAllowed && native,
                    installation_id: installation?.installation_id,
                  }
                  const preview = await exchange<ExportPlan>(
                    '/exports',
                    request,
                  )
                  if (saveDefinition) {
                    const existing = definitions.find(
                      (item) =>
                        item.package.id === definitionID &&
                        item.package.id === id,
                    )
                    const saved = await exchange<ExportDefinition>(
                      '/export-definitions',
                      { ...request, revision: existing?.revision },
                      'PUT',
                    )
                    setDefinitions((items) => [
                      ...items.filter((item) => item.package.id !== id),
                      saved,
                    ])
                    setDefinitionID(id)
                  }
                  setPlan(preview)
                }
              } catch {
                setError(t('market.errors.operationFailed'))
              } finally {
                setBusy(false)
              }
            }}
          >
            {t(
              busy
                ? 'market.working'
                : plan
                  ? 'market.export.download'
                  : 'market.export.review',
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
