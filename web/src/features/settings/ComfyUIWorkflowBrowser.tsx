import { useEffect, useRef, useState, type KeyboardEvent, type MutableRefObject } from 'react'
import { Check, LoaderCircle, RefreshCw, Server, SlidersHorizontal } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { discoverComfyUIWorkflows, loadComfyUIWorkflow } from './api'
import type {
  ComfyUIParameterRole,
  ComfyUIParameterSettings,
  ComfyUIProfileSettings,
  ComfyUIWorkflowCatalog,
  ComfyUIWorkflowStatus,
  ComfyUIWorkflowSummary,
  ImageAPIEndpointSettings,
  ImageAPIProfileSettings,
} from './types'

type Activity = 'idle' | 'discovering' | 'loading'

interface ComfyUIWorkflowBrowserProps {
  endpoint: ImageAPIEndpointSettings
  profile: ImageAPIProfileSettings
  onChange: (settings: ComfyUIProfileSettings) => void
}

interface ComfyUIParameterEditorProps {
  parameter: ComfyUIParameterSettings
  options?: string[]
  onChange: (patch: Partial<ComfyUIParameterSettings>) => void
}

interface ComfyUIParameterValueProps {
  parameter: ComfyUIParameterSettings
  options?: string[]
  onChange: (value: string) => void
}

export function ComfyUIWorkflowBrowser({ endpoint, profile, onChange }: ComfyUIWorkflowBrowserProps) {
  const { t } = useTranslation()
  const [catalog, setCatalog] = useState<ComfyUIWorkflowCatalog | null>(null)
  const [activity, setActivity] = useState<Activity>('idle')
  const [loadingPath, setLoadingPath] = useState('')
  const [error, setError] = useState('')
  const [optionCache, setOptionCache] = useState<Record<string, string[]>>({})
  const activeRequest = useRef<AbortController | null>(null)
  const connectionKey = `${endpoint.base_url ?? ''}\0${endpoint.api_key ?? ''}\0${JSON.stringify(endpoint.headers ?? {})}`

  useEffect(() => {
    activeRequest.current?.abort()
    setCatalog(null)
    setActivity('idle')
    setLoadingPath('')
    setError('')
  }, [connectionKey])

  useEffect(() => () => activeRequest.current?.abort(), [])

  const discover = async () => {
    const controller = startRequest(activeRequest)
    setActivity('discovering')
    setLoadingPath('')
    setError('')
    try {
      const result = await discoverComfyUIWorkflows(endpoint, profile, controller.signal)
      if (controller.signal.aborted) return
      setCatalog(result)
    } catch (requestError) {
      if (!controller.signal.aborted) setError(errorMessage(requestError))
    } finally {
      if (activeRequest.current === controller) setActivity('idle')
    }
  }

  const load = async (workflow: ComfyUIWorkflowSummary) => {
    if (workflow.status !== 'ready') return
    const controller = startRequest(activeRequest)
    setActivity('loading')
    setLoadingPath(workflow.path)
    setError('')
    try {
      const snapshot = await loadComfyUIWorkflow(endpoint, profile, workflow.path, controller.signal)
      if (controller.signal.aborted) return
      const nextOptions: Record<string, string[]> = {}
      for (const parameter of snapshot.parameters ?? []) {
        if (parameter.options?.length) nextOptions[parameterKey(parameter)] = parameter.options
      }
      setOptionCache(nextOptions)
      onChange({
        workflow_mode: 'remote',
        workflow: snapshot.workflow,
        workflow_name: snapshot.name,
        workflow_id: snapshot.workflow_id,
        workflow_path: snapshot.path,
        workflow_modified: snapshot.modified,
        workflow_job_id: snapshot.job_id,
        workflow_job_time: snapshot.job_time,
        parameters: (snapshot.parameters ?? []).map((parameter) => ({ ...parameter, options: undefined })),
      })
    } catch (requestError) {
      if (!controller.signal.aborted) setError(errorMessage(requestError))
    } finally {
      if (activeRequest.current === controller) {
        setActivity('idle')
        setLoadingPath('')
      }
    }
  }

  const settings = profile.comfyui ?? { workflow_mode: 'remote' as const }
  const parameters = settings.parameters ?? []
  const updateParameter = (index: number, patch: Partial<ComfyUIParameterSettings>) => {
    onChange({
      ...settings,
      workflow_mode: 'remote',
      parameters: parameters.map((parameter, current) => current === index ? { ...parameter, ...patch } : parameter),
    })
  }

  return (
    <div className="overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-1)] md:col-span-12">
      <div className="flex flex-col gap-2 p-2.5 sm:flex-row sm:items-center">
        <div className="flex min-w-0 flex-1 items-start gap-2">
          <Server className="mt-0.5 size-4 shrink-0 text-[var(--nova-text-muted)]" aria-hidden="true" />
          <div className="min-w-0">
            <div className="truncate text-xs font-medium text-[var(--nova-text)]">
              {settings.workflow_name || t('settings.imageApi.workflowRemoteEmptyTitle')}
            </div>
            <div className="break-words text-[11px] leading-4 text-[var(--nova-text-faint)]">
              {settings.workflow_path || t('settings.imageApi.workflowRemoteHint')}
            </div>
          </div>
        </div>
        <Button type="button" variant="outline" size="sm" disabled={activity !== 'idle'} onClick={() => void discover()}>
          {activity === 'discovering' ? <LoaderCircle className="animate-spin" data-icon="inline-start" /> : <RefreshCw data-icon="inline-start" />}
          {activity === 'discovering' ? t('settings.imageApi.workflowDiscovering') : t('settings.imageApi.workflowDiscover')}
        </Button>
      </div>

      {error && (
        <div role="alert" className="border-t border-[var(--nova-border)] bg-red-500/5 px-2.5 py-2 text-[11px] leading-4 text-red-700 dark:text-red-300">
          {t('settings.imageApi.workflowDiscoveryFailure', { error })}
        </div>
      )}

      {catalog && (
        <div className="border-t border-[var(--nova-border)] p-1.5">
          {catalog.workflows.length === 0 ? (
            <div className="px-2 py-3 text-center text-[11px] text-[var(--nova-text-faint)]">{t('settings.imageApi.workflowRemoteEmpty')}</div>
          ) : catalog.workflows.map((workflow) => {
            const selected = workflow.path === settings.workflow_path
            const loading = workflow.path === loadingPath
            const selectable = workflow.status === 'ready' && activity === 'idle'
            return (
              <button
                key={workflow.path}
                type="button"
                disabled={!selectable}
                aria-pressed={selected}
                onClick={() => void load(workflow)}
                className={`grid w-full grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 rounded-[calc(var(--nova-radius)-2px)] px-2 py-2 text-left transition-colors ${selected ? 'bg-[var(--nova-surface-3)]' : ''} ${selectable ? 'hover:bg-[var(--nova-surface-2)]' : 'cursor-not-allowed opacity-70'}`}
              >
                <WorkflowStatusDot status={workflow.status} />
                <span className="min-w-0">
                  <span className="flex items-center gap-1.5">
                    <span className="truncate text-xs font-medium text-[var(--nova-text)]">{workflow.name}</span>
                    {selected && <Check className="size-3.5 shrink-0 text-emerald-600 dark:text-emerald-400" aria-label={t('settings.imageApi.workflowSelected')} />}
                  </span>
                  <span className="block truncate text-[11px] text-[var(--nova-text-faint)]">{workflow.path}</span>
                </span>
                <span className="flex max-w-44 items-center gap-1 text-right text-[10px] leading-3 text-[var(--nova-text-muted)]">
                  {loading && <LoaderCircle className="size-3 animate-spin" aria-hidden="true" />}
                  {t(workflowStatusKey(workflow.status))}
                </span>
              </button>
            )
          })}
          <div className="px-2 pb-1 pt-1.5 text-[10px] leading-4 text-[var(--nova-text-faint)]">
            {t('settings.imageApi.workflowStatusHelp')}
          </div>
        </div>
      )}

      {settings.workflow && (
        <div className="border-t border-[var(--nova-border)]">
          <div className="flex items-start gap-2 px-2.5 py-2">
            <SlidersHorizontal className="mt-0.5 size-4 shrink-0 text-[var(--nova-text-muted)]" aria-hidden="true" />
            <div>
              <div className="text-xs font-medium text-[var(--nova-text)]">{t('settings.imageApi.workflowParameters', { count: parameters.length })}</div>
              <div className="text-[11px] leading-4 text-[var(--nova-text-faint)]">{t('settings.imageApi.workflowParametersHint')}</div>
            </div>
          </div>
          {parameters.length === 0 ? (
            <div className="border-t border-[var(--nova-border)] px-2.5 py-3 text-[11px] text-[var(--nova-text-faint)]">{t('settings.imageApi.workflowParametersEmpty')}</div>
          ) : (
            <div className="divide-y divide-[var(--nova-border)] border-t border-[var(--nova-border)]">
              {parameters.map((parameter, index) => (
                <ComfyUIParameterEditor
                  key={parameterKey(parameter)}
                  parameter={parameter}
                  options={parameter.options?.length ? parameter.options : optionCache[parameterKey(parameter)]}
                  onChange={(patch) => updateParameter(index, patch)}
                />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function ComfyUIParameterEditor({ parameter, options, onChange }: ComfyUIParameterEditorProps) {
  const { t } = useTranslation()
  const role = parameter.role ?? 'parameter'
  const roles = parameterRoles(parameter.type, role)
  const runtime = role === 'prompt' || role === 'width' || role === 'height' || role === 'batch_size' || role === 'seed'
  return (
    <div className="grid gap-2 px-2.5 py-2.5 lg:grid-cols-[minmax(10rem,1fr)_11rem_minmax(12rem,1.2fr)] lg:items-start">
      <div className="min-w-0">
        <div className="truncate text-xs font-medium text-[var(--nova-text)]">{parameter.label || `${parameter.node_id}.${parameter.input_name}`}</div>
        <div className="truncate font-mono text-[10px] text-[var(--nova-text-faint)]">{parameter.node_id}.{parameter.input_name} · {parameter.type}</div>
      </div>
      <label className="flex min-w-0 flex-col gap-1">
        <span className="text-[10px] leading-none text-[var(--nova-text-faint)]">{t('settings.imageApi.workflowParameterBinding')}</span>
        <Select value={role} onValueChange={(value) => onChange({ role: value as ComfyUIParameterRole })}>
          <SelectTrigger size="sm" className="w-full"><SelectValue /></SelectTrigger>
          <SelectContent className="nova-panel border text-[var(--nova-text)]">
            <SelectGroup>{roles.map((value) => <SelectItem key={value} value={value}>{t(roleLabelKey(value))}</SelectItem>)}</SelectGroup>
          </SelectContent>
        </Select>
      </label>
      <label className="flex min-w-0 flex-col gap-1">
        <span className="text-[10px] leading-none text-[var(--nova-text-faint)]">{t('settings.imageApi.workflowParameterValue')}</span>
        {runtime ? (
          <span className="flex min-h-8 items-center rounded-md border border-dashed border-[var(--nova-border)] px-2 text-[11px] leading-4 text-[var(--nova-text-faint)]">
            {t('settings.imageApi.workflowParameterRuntime')}
          </span>
        ) : (
          <ComfyUIParameterValue parameter={parameter} options={options} onChange={(value) => onChange({ value })} />
        )}
      </label>
    </div>
  )
}

function ComfyUIParameterValue({ parameter, options, onChange }: ComfyUIParameterValueProps) {
  if (parameter.type === 'BOOLEAN') {
    return (
      <Select value={parameter.value === 'true' ? 'true' : 'false'} onValueChange={onChange}>
        <SelectTrigger size="sm" className="w-full"><SelectValue /></SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectItem value="true">true</SelectItem>
          <SelectItem value="false">false</SelectItem>
        </SelectContent>
      </Select>
    )
  }
  if (parameter.type === 'INT' || parameter.type === 'FLOAT') {
    return <NumberLiteralInput parameter={parameter} onChange={onChange} />
  }
  const value = decodeStringLiteral(parameter.value)
  const values = options?.filter(Boolean)
  if (parameter.type === 'COMBO' && values?.length) {
    const available = values.includes(value) ? values : [value, ...values].filter(Boolean)
    return (
      <Select value={value} onValueChange={(next) => onChange(JSON.stringify(next))}>
        <SelectTrigger size="sm" className="w-full"><SelectValue /></SelectTrigger>
        <SelectContent className="nova-panel max-h-80 border text-[var(--nova-text)]">
          <SelectGroup>{available.map((option) => <SelectItem key={option} value={option}>{option}</SelectItem>)}</SelectGroup>
        </SelectContent>
      </Select>
    )
  }
  if (parameter.multiline) {
    return <Textarea value={value} rows={2} onChange={(event) => onChange(JSON.stringify(event.target.value))} />
  }
  return <Input value={value} onChange={(event) => onChange(JSON.stringify(event.target.value))} />
}

function NumberLiteralInput({ parameter, onChange }: { parameter: ComfyUIParameterSettings; onChange: (value: string) => void }) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState(parameter.value)
  useEffect(() => setDraft(parameter.value), [parameter.value])
  const valid = isNumberLiteral(draft, parameter.type === 'INT')
  const commit = () => {
    if (valid) onChange(draft)
    else setDraft(parameter.value)
  }
  const handleKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') event.currentTarget.blur()
    if (event.key === 'Escape') {
      setDraft(parameter.value)
      event.currentTarget.blur()
    }
  }
  return (
    <div className="flex flex-col gap-1">
      <Input
        value={draft}
        inputMode={parameter.type === 'INT' ? 'numeric' : 'decimal'}
        aria-invalid={!valid}
        onChange={(event) => setDraft(event.target.value)}
        onBlur={commit}
        onKeyDown={handleKeyDown}
      />
      {!valid && <span className="text-[10px] text-red-600 dark:text-red-400">{t('settings.imageApi.workflowParameterNumberInvalid')}</span>}
    </div>
  )
}

function WorkflowStatusDot({ status }: { status: ComfyUIWorkflowStatus }) {
  let color = 'bg-[var(--nova-text-faint)]'
  if (status === 'ready') color = 'bg-emerald-500 dark:bg-emerald-400'
  else if (status === 'stale') color = 'bg-amber-500 dark:bg-amber-400'
  else if (status === 'invalid') color = 'bg-red-500 dark:bg-red-400'
  return <span className={`size-2 rounded-full ${color}`} aria-hidden="true" />
}

function startRequest(reference: MutableRefObject<AbortController | null>): AbortController {
  reference.current?.abort()
  const controller = new AbortController()
  reference.current = controller
  return controller
}

function parameterRoles(type: string, current: ComfyUIParameterRole): ComfyUIParameterRole[] {
  let roles: ComfyUIParameterRole[] = ['parameter']
  if (type === 'STRING') roles = ['parameter', 'prompt', 'negative_prompt']
  else if (type === 'INT' || type === 'FLOAT') roles = ['parameter', 'width', 'height', 'batch_size', 'seed']
  if (!roles.includes(current)) roles.push(current)
  return roles
}

function roleLabelKey(role: ComfyUIParameterRole): string {
  return `settings.imageApi.workflowParameterRole.${role}`
}

function workflowStatusKey(status: ComfyUIWorkflowStatus): string {
  return `settings.imageApi.workflowStatus.${status}`
}

function parameterKey(parameter: ComfyUIParameterSettings): string {
  return `${parameter.node_id}\0${parameter.input_name}`
}

function decodeStringLiteral(value: string): string {
  try {
    const decoded: unknown = JSON.parse(value)
    return typeof decoded === 'string' ? decoded : value
  } catch {
    return value
  }
}

function isNumberLiteral(value: string, integer: boolean): boolean {
  const normalized = value.trim()
  if (integer) return /^-?\d+$/.test(normalized)
  return /^-?(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?$/i.test(normalized)
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}
