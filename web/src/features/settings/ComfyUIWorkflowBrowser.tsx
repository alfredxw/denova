import { useEffect, useRef, useState, type ChangeEvent, type MutableRefObject } from 'react'
import { Check, CircleAlert, FileJson, LoaderCircle, RefreshCw, Server, SlidersHorizontal } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { Field, FieldContent, FieldDescription, FieldGroup, FieldTitle } from '@/components/ui/field'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { cn } from '@/lib/utils'
import { discoverComfyUIWorkflows, loadComfyUIWorkflow } from './api'
import type {
  ComfyUIBindings,
  ComfyUIBindingCandidates,
  ComfyUIInputBinding,
  ComfyUIInputCandidate,
  ComfyUIProfileSettings,
  ComfyUIWorkflowCatalog,
  ComfyUIWorkflowStatus,
  ComfyUIWorkflowSummary,
  ImageAPIEndpointSettings,
  ImageAPIProfileSettings,
} from './types'

type Activity = 'idle' | 'discovering' | 'importing' | 'loading'
type BindingName = keyof ComfyUIBindings

const MAX_WORKFLOW_BYTES = 5 * 1024 * 1024
const WORKFLOW_VALUE = '__workflow_value__'

interface ComfyUIWorkflowBrowserProps {
  endpoint: ImageAPIEndpointSettings
  profile: ImageAPIProfileSettings
  onChange: (settings: ComfyUIProfileSettings) => void
}

interface BindingSelectProps {
  label: string
  binding?: ComfyUIInputBinding
  candidates: ComfyUIInputCandidate[]
  required?: boolean
  triggerClassName?: string
  onChange: (binding?: ComfyUIInputBinding) => void
}

export function ComfyUIWorkflowBrowser({ endpoint, profile, onChange }: ComfyUIWorkflowBrowserProps) {
  const { t } = useTranslation()
  const [catalog, setCatalog] = useState<ComfyUIWorkflowCatalog | null>(null)
  const [activity, setActivity] = useState<Activity>('idle')
  const [loadingPath, setLoadingPath] = useState('')
  const [error, setError] = useState('')
  const [candidateCache, setCandidateCache] = useState<ComfyUIBindingCandidates>({})
  const activeRequest = useRef<AbortController | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const connectionKey = `${endpoint.base_url ?? ''}\0${endpoint.api_key ?? ''}\0${JSON.stringify(endpoint.headers ?? {})}`
  const settings = profile.comfyui ?? { workflow_mode: 'remote' as const }
  const isImported = settings.workflow_mode === 'api'
  const selectedSummary = catalog?.workflows.find((workflow) => workflow.path === settings.workflow_path)
  const needsParsing = settings.workflow_mode !== 'api'
    && selectedSummary !== undefined
    && selectedSummary.status !== 'invalid'
    && (!settings.workflow || selectedSummary.status !== 'ready')
  const showBindings = settings.workflow_mode !== 'api'
    && Boolean(settings.workflow)
    && (!selectedSummary || selectedSummary.status === 'ready')
  const bindings = settings.bindings ?? {}
  const workflowDescription = isImported
    ? t('settings.imageApi.workflowImportedHint')
    : settings.workflow_path || t('settings.imageApi.workflowRemoteHint')
  let discoverButtonLabel = t(catalog ? 'settings.imageApi.workflowRefresh' : 'settings.imageApi.workflowDiscover')
  if (activity === 'discovering') discoverButtonLabel = t('settings.imageApi.workflowDiscovering')
  let importButtonLabel = t(isImported ? 'settings.imageApi.workflowReplace' : 'settings.imageApi.workflowUpload')
  if (activity === 'importing') importButtonLabel = t('settings.imageApi.workflowImporting')

  useEffect(() => {
    activeRequest.current?.abort()
    setCatalog(null)
    setActivity('idle')
    setLoadingPath('')
    setError('')
    setCandidateCache({})
  }, [connectionKey])

  useEffect(() => () => activeRequest.current?.abort(), [])

  async function discover(): Promise<void> {
    const controller = startRequest(activeRequest)
    setActivity('discovering')
    setLoadingPath('')
    setError('')
    try {
      const result = await discoverComfyUIWorkflows(endpoint, profile, controller.signal)
      if (controller.signal.aborted) return
      setCatalog(result)
      const selected = result.workflows.find((workflow) => workflow.path === settings.workflow_path)
      if (selected?.status === 'ready' && !isImported && !settings.workflow) {
        await load(selected)
      }
    } catch (requestError) {
      if (!controller.signal.aborted) setError(t('settings.imageApi.workflowDiscoveryFailure', { error: errorMessage(requestError) }))
    } finally {
      if (activeRequest.current === controller) setActivity('idle')
    }
  }

  async function load(workflow: ComfyUIWorkflowSummary): Promise<void> {
    if (workflow.status !== 'ready') return
    const controller = startRequest(activeRequest)
    setActivity('loading')
    setLoadingPath(workflow.path)
    setError('')
    try {
      const snapshot = await loadComfyUIWorkflow(endpoint, profile, workflow.path, controller.signal)
      if (controller.signal.aborted) return
      setCandidateCache(snapshot.candidates ?? {})
      onChange({
        workflow_mode: 'remote',
        workflow: snapshot.workflow,
        workflow_name: snapshot.name,
        workflow_id: snapshot.workflow_id,
        workflow_path: snapshot.path,
        workflow_modified: snapshot.modified,
        workflow_job_id: snapshot.job_id,
        workflow_job_time: snapshot.job_time,
        bindings: snapshot.bindings,
      })
    } catch (requestError) {
      if (!controller.signal.aborted) setError(t('settings.imageApi.workflowLoadFailure', { error: errorMessage(requestError) }))
    } finally {
      if (activeRequest.current === controller) {
        setActivity('idle')
        setLoadingPath('')
      }
    }
  }

  async function selectWorkflow(workflow: ComfyUIWorkflowSummary): Promise<void> {
    if (workflow.status === 'invalid') return
    if (workflow.status === 'ready') {
      await load(workflow)
      return
    }
    activeRequest.current?.abort()
    setActivity('idle')
    setLoadingPath('')
    setError('')
    setCandidateCache({})
    onChange({
      workflow_mode: 'remote',
      workflow_name: workflow.name,
      workflow_id: workflow.workflow_id,
      workflow_path: workflow.path,
      workflow_modified: workflow.modified,
    })
  }

  async function importWorkflow(event: ChangeEvent<HTMLInputElement>): Promise<void> {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    if (file.size > MAX_WORKFLOW_BYTES) {
      setError(t('settings.imageApi.workflowTooLarge'))
      return
    }

    const controller = startRequest(activeRequest)
    setActivity('importing')
    setLoadingPath('')
    setError('')
    try {
      const workflow = await file.text()
      if (controller.signal.aborted) return
      const parsed: unknown = JSON.parse(workflow)
      if (!isComfyUIAPIWorkflow(parsed)) throw new Error('invalid-api-format')
      setCandidateCache({})
      onChange({ workflow_mode: 'api', workflow, workflow_name: file.name })
    } catch (importError) {
      if (!controller.signal.aborted) {
        console.warn(`[settings] failed to read ComfyUI API workflow file=${file.name}`, importError)
        setError(t('settings.imageApi.workflowInvalid'))
      }
    } finally {
      if (activeRequest.current === controller) setActivity('idle')
    }
  }

  function updateBinding(name: BindingName, binding?: ComfyUIInputBinding): void {
    const next = { ...bindings }
    if (binding) next[name] = binding
    else delete next[name]
    onChange({
      ...settings,
      workflow_mode: 'remote',
      bindings: Object.keys(next).length > 0 ? next : undefined,
    })
  }
  return (
    <div className="overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] md:col-span-12">
      <input
        ref={fileInputRef}
        type="file"
        accept=".json,application/json"
        aria-label={t('settings.imageApi.workflowUpload')}
        className="hidden"
        onChange={(event) => void importWorkflow(event)}
      />
      <div className="flex flex-col gap-2 p-2.5 sm:flex-row sm:items-center">
        <div className="flex min-w-0 flex-1 items-start gap-2">
          {isImported
            ? <FileJson className="mt-0.5 size-4 shrink-0 text-[var(--nova-text-muted)]" aria-hidden="true" />
            : <Server className="mt-0.5 size-4 shrink-0 text-[var(--nova-text-muted)]" aria-hidden="true" />}
          <div className="min-w-0">
            <div className="truncate text-xs font-medium text-[var(--nova-text)]">
              {settings.workflow_name || t('settings.imageApi.workflowRemoteEmptyTitle')}
            </div>
            <div className="break-words text-[11px] leading-4 text-[var(--nova-text-faint)]">
              {workflowDescription}
            </div>
          </div>
        </div>
        <div className="flex flex-wrap gap-1.5">
          <Button type="button" variant="outline" size="sm" disabled={activity !== 'idle'} onClick={() => void discover()}>
            {activity === 'discovering' ? <LoaderCircle className="animate-spin" data-icon="inline-start" /> : <RefreshCw data-icon="inline-start" />}
            {discoverButtonLabel}
          </Button>
          <Button type="button" variant="outline" size="sm" disabled={activity !== 'idle'} onClick={() => fileInputRef.current?.click()}>
            {activity === 'importing' ? <LoaderCircle className="animate-spin" data-icon="inline-start" /> : <FileJson data-icon="inline-start" />}
            {importButtonLabel}
          </Button>
        </div>
      </div>

      {error && (
        <>
          <Separator />
          <div role="alert" className="px-2.5 py-2 text-[11px] leading-4 text-destructive">
            {error}
          </div>
        </>
      )}

      {catalog && (
        <>
          <Separator />
          <div className="p-1.5">
            {catalog.workflows.length === 0 ? (
              <Empty className="gap-2 rounded-none p-4">
                <EmptyHeader className="gap-1">
                  <EmptyMedia variant="icon"><Server /></EmptyMedia>
                  <EmptyTitle>{t('settings.imageApi.workflowRemoteEmptyTitle')}</EmptyTitle>
                  <EmptyDescription className="text-xs">{t('settings.imageApi.workflowRemoteEmpty')}</EmptyDescription>
                </EmptyHeader>
              </Empty>
            ) : catalog.workflows.map((workflow) => {
              const selected = workflow.path === settings.workflow_path
              const loading = workflow.path === loadingPath
              const selectable = workflow.status !== 'invalid' && activity === 'idle'
              return (
                <button
                  key={workflow.path}
                  type="button"
                  disabled={!selectable}
                  aria-pressed={selected}
                  onClick={() => void selectWorkflow(workflow)}
                  className={cn(
                    'grid w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-2 rounded-[calc(var(--nova-radius)-2px)] px-2 py-2 text-left transition-colors',
                    selected && 'bg-[var(--nova-surface-3)]',
                    selectable ? 'hover:bg-[var(--nova-surface-2)]' : 'cursor-not-allowed opacity-70',
                  )}
                >
                  <span className="min-w-0">
                    <span className="flex items-center gap-1.5">
                      <span className="truncate text-xs font-medium text-[var(--nova-text)]">{workflow.name}</span>
                      {selected && <Check className="size-3.5 shrink-0 text-[var(--nova-text-muted)]" aria-label={t('settings.imageApi.workflowSelected')} />}
                    </span>
                    <span className="block truncate text-[11px] text-[var(--nova-text-faint)]">{workflow.path}</span>
                  </span>
                  <Badge variant={workflowStatusVariant(workflow.status)}>
                    {loading && <LoaderCircle className="animate-spin" data-icon="inline-start" />}
                    {t(workflowStatusKey(workflow.status))}
                  </Badge>
                </button>
              )
            })}
            <div className="px-2 pb-1 pt-1.5 text-[10px] leading-4 text-[var(--nova-text-faint)]">
              {t('settings.imageApi.workflowStatusHelp')}
            </div>
          </div>
        </>
      )}

      {needsParsing && (
        <>
          <Separator />
          <Empty className="gap-3 rounded-none p-4">
            <EmptyHeader className="gap-1.5">
              <EmptyMedia variant="icon" className="text-amber-600 dark:text-amber-400"><CircleAlert /></EmptyMedia>
              <EmptyTitle>{t('settings.imageApi.workflowParseRequired')}</EmptyTitle>
              <EmptyDescription className="text-xs">{t('settings.imageApi.workflowParseRequiredHint')}</EmptyDescription>
            </EmptyHeader>
            <EmptyContent className="flex-row flex-wrap justify-center">
              <Button type="button" variant="outline" size="sm" disabled={activity !== 'idle'} onClick={() => void discover()}>
                <RefreshCw data-icon="inline-start" />{t('settings.imageApi.workflowRefresh')}
              </Button>
              <Button type="button" variant="outline" size="sm" disabled={activity !== 'idle'} onClick={() => fileInputRef.current?.click()}>
                <FileJson data-icon="inline-start" />{t('settings.imageApi.workflowUpload')}
              </Button>
            </EmptyContent>
          </Empty>
        </>
      )}

      {showBindings && (
        <>
          <Separator />
          <div className="px-2.5 py-2.5">
            <div className="flex items-center justify-between gap-2">
              <div className="flex min-w-0 items-center gap-2">
                <SlidersHorizontal className="size-4 shrink-0 text-[var(--nova-text-muted)]" aria-hidden="true" />
                <div className="text-xs font-medium leading-4 text-[var(--nova-text)]">{t('settings.imageApi.workflowBindings')}</div>
              </div>
              <Badge className="shrink-0" variant={bindings.prompt ? 'secondary' : 'destructive'}>
                {bindings.prompt ? <Check data-icon="inline-start" /> : <CircleAlert data-icon="inline-start" />}
                {bindings.prompt ? t('settings.imageApi.workflowBindingsReady') : t('settings.imageApi.workflowBindingsMissing')}
              </Badge>
            </div>
            <div className="mt-0.5 pl-6 text-[11px] leading-4 text-[var(--nova-text-faint)]">
              {t('settings.imageApi.workflowBindingsHint')}
            </div>
          </div>
          <Separator />
          <FieldGroup className="gap-0 divide-y divide-[var(--nova-border)]">
            <Field orientation="responsive" data-invalid={!bindings.prompt ? true : undefined} className="px-2.5 py-2.5">
              <FieldContent>
                <FieldTitle>{t('settings.imageApi.workflowBindingPrompt')}</FieldTitle>
                <FieldDescription className="text-xs">{t('settings.imageApi.workflowBindingPromptHint')}</FieldDescription>
              </FieldContent>
              <BindingSelect
                label={t('settings.imageApi.workflowBindingPrompt')}
                binding={bindings.prompt}
                candidates={candidateCache.prompt ?? []}
                required
                onChange={(binding) => updateBinding('prompt', binding)}
              />
            </Field>
            <Field orientation="responsive" className="px-2.5 py-2.5">
              <FieldContent>
                <FieldTitle>{t('settings.imageApi.workflowBindingCount')}</FieldTitle>
                <FieldDescription className="text-xs">{t('settings.imageApi.workflowBindingCountHint')}</FieldDescription>
              </FieldContent>
              <BindingSelect
                label={t('settings.imageApi.workflowBindingCount')}
                binding={bindings.count}
                candidates={candidateCache.count ?? []}
                onChange={(binding) => updateBinding('count', binding)}
              />
            </Field>
            <Field orientation="responsive" className="px-2.5 py-2.5">
              <FieldContent>
                <FieldTitle>{t('settings.imageApi.workflowBindingSize')}</FieldTitle>
                <FieldDescription className="text-xs">{t('settings.imageApi.workflowBindingSizeHint')}</FieldDescription>
              </FieldContent>
              <div className="grid w-full gap-2 sm:grid-cols-2 md:w-[28.5rem]">
                <BindingSelect
                  label={t('settings.imageApi.workflowBindingWidth')}
                  binding={bindings.width}
                  candidates={candidateCache.width ?? []}
                  triggerClassName="md:w-56"
                  onChange={(binding) => updateBinding('width', binding)}
                />
                <BindingSelect
                  label={t('settings.imageApi.workflowBindingHeight')}
                  binding={bindings.height}
                  candidates={candidateCache.height ?? []}
                  triggerClassName="md:w-56"
                  onChange={(binding) => updateBinding('height', binding)}
                />
              </div>
            </Field>
          </FieldGroup>
        </>
      )}
    </div>
  )
}

function BindingSelect({ label, binding, candidates, required, triggerClassName, onChange }: BindingSelectProps) {
  const { t } = useTranslation()
  const available = withCurrentBinding(binding, candidates)
  const value = binding ? bindingKey(binding) : WORKFLOW_VALUE

  function handleValueChange(next: string) {
    if (next === WORKFLOW_VALUE) {
      onChange(undefined)
      return
    }
    const selected = available.find((candidate) => bindingKey(candidate) === next)
    if (selected) onChange({ node_id: selected.node_id, input_name: selected.input_name })
  }

  return (
    <Select value={value} onValueChange={handleValueChange}>
      <SelectTrigger size="sm" className={cn('w-full md:w-72', triggerClassName)} aria-label={label} aria-invalid={required && !binding ? true : undefined}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent className="nova-panel max-h-80 border text-[var(--nova-text)]">
        <SelectGroup>
          <SelectItem value={WORKFLOW_VALUE}>
            {required ? t('settings.imageApi.workflowBindingUnbound') : t('settings.imageApi.workflowBindingWorkflowValue')}
          </SelectItem>
          {available.map((candidate) => (
            <SelectItem key={bindingKey(candidate)} value={bindingKey(candidate)}>
              {candidate.label ? `${candidate.label} · ` : ''}{bindingLabel(candidate)}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  )
}

function withCurrentBinding(binding: ComfyUIInputBinding | undefined, candidates: ComfyUIInputCandidate[]): ComfyUIInputCandidate[] {
  if (!binding || candidates.some((candidate) => bindingKey(candidate) === bindingKey(binding))) return candidates
  return [{ ...binding, label: '' }, ...candidates]
}

function bindingKey(binding: ComfyUIInputBinding): string {
  return JSON.stringify([binding.node_id, binding.input_name])
}

function bindingLabel(binding: ComfyUIInputBinding): string {
  return `${binding.node_id}.${binding.input_name}`
}

function startRequest(reference: MutableRefObject<AbortController | null>): AbortController {
  reference.current?.abort()
  const controller = new AbortController()
  reference.current = controller
  return controller
}

function workflowStatusKey(status: ComfyUIWorkflowStatus): string {
  return `settings.imageApi.workflowStatus.${status}`
}

function workflowStatusVariant(status: ComfyUIWorkflowStatus): 'secondary' | 'outline' | 'destructive' {
  switch (status) {
    case 'ready':
      return 'secondary'
    case 'stale':
    case 'not_run':
      return 'outline'
    case 'invalid':
      return 'destructive'
  }
}

function isComfyUIAPIWorkflow(value: unknown): value is Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const nodes = Object.values(value)
  return nodes.length > 0 && nodes.every((node) => {
    if (!node || typeof node !== 'object' || Array.isArray(node)) return false
    const record = node as Record<string, unknown>
    return typeof record.class_type === 'string'
      && Boolean(record.inputs)
      && typeof record.inputs === 'object'
      && !Array.isArray(record.inputs)
  })
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}
