import { useEffect, useMemo, useRef, useState } from 'react'
import { Check, ChevronDown, Loader2, Plus, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import { discoverModels } from './api'
import {
  MODEL_PROTOCOL_ANTHROPIC_MESSAGES,
  MODEL_PROTOCOL_CHAT_COMPLETIONS,
  MODEL_PROTOCOL_RESPONSES,
} from './model-profiles'
import type { ModelEndpointSettings, ModelInfo } from './types'

type DiscoveryState =
  | { status: 'idle'; models: ModelInfo[] }
  | { status: 'loading'; models: ModelInfo[] }
  | { status: 'success'; models: ModelInfo[] }
  | { status: 'error'; models: ModelInfo[]; message: string }

export function ModelDiscoveryPicker({ endpoint, defaultProtocol, existingModels, onAdd }: {
  endpoint: ModelEndpointSettings
  defaultProtocol?: string
  existingModels: string[]
  onAdd: (models: ModelInfo[]) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState<Set<string>>(() => new Set())
  const [state, setState] = useState<DiscoveryState>({ status: 'idle', models: [] })
  const requestRef = useRef<AbortController | null>(null)
  const protocol = endpoint.protocol?.trim() || defaultProtocol?.trim() || ''
  const supported = protocol === MODEL_PROTOCOL_CHAT_COMPLETIONS
    || protocol === MODEL_PROTOCOL_RESPONSES
    || protocol === MODEL_PROTOCOL_ANTHROPIC_MESSAGES
  const routeFingerprint = useMemo(() => JSON.stringify({ ...endpoint, protocol }), [endpoint, protocol])
  const existing = useMemo(() => new Set(existingModels.map((model) => model.trim()).filter(Boolean)), [existingModels])
  const visibleModels = useMemo(() => {
    const normalized = query.trim().toLocaleLowerCase()
    if (!normalized) return state.models
    return state.models.filter((model) => `${model.display_name ?? ''}\n${model.id}\n${model.owned_by ?? ''}`.toLocaleLowerCase().includes(normalized))
  }, [query, state.models])

  useEffect(() => {
    requestRef.current?.abort()
    requestRef.current = null
    setState({ status: 'idle', models: [] })
    setSelected(new Set())
    setQuery('')
    setOpen(false)
  }, [routeFingerprint])

  useEffect(() => () => requestRef.current?.abort(), [])

  const load = async () => {
    requestRef.current?.abort()
    const request = new AbortController()
    requestRef.current = request
    setState((current) => ({ status: 'loading', models: current.models }))
    try {
      const result = await discoverModels(endpoint, { endpoint_id: endpoint.id }, request.signal)
      if (requestRef.current === request) {
        requestRef.current = null
        setState({ status: 'success', models: result.models ?? [] })
      }
    } catch (error) {
      if (request.signal.aborted) return
      requestRef.current = null
      setState((current) => ({
        status: 'error',
        models: current.models,
        message: error instanceof Error ? error.message : String(error),
      }))
    }
  }

  const available = supported && Boolean(endpoint.provider?.trim())
  const selectedModels = state.models.filter((model) => selected.has(model.id) && !existing.has(model.id))

  return (
    <Popover open={open} onOpenChange={(nextOpen) => {
      setOpen(nextOpen)
      if (nextOpen && state.status === 'idle') void load()
    }}>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="sm" disabled={!available}>
          {state.status === 'loading' ? <Loader2 className="animate-spin" data-icon="inline-start" /> : <ChevronDown data-icon="inline-start" />}
          {available ? t('settings.model.discoveryAddModels') : t('settings.model.discoveryUnsupported')}
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        sideOffset={4}
        collisionPadding={8}
        className="nova-panel w-[min(32rem,calc(100vw-1rem))] gap-2 border p-2 text-[var(--nova-text)]"
      >
        <div className="flex items-center gap-2">
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={t('settings.model.discoverySearchPlaceholder')}
            aria-label={t('settings.model.discoverySearchPlaceholder')}
            className="min-w-0 flex-1"
          />
          <Button type="button" variant="outline" size="icon-sm" disabled={state.status === 'loading'} aria-label={t('settings.model.discoveryRefresh')} onClick={() => void load()}>
            <RefreshCw className={cn(state.status === 'loading' && 'animate-spin')} />
          </Button>
        </div>
        {state.status === 'loading' && state.models.length === 0 && (
          <div className="px-1 py-3 text-xs text-[var(--nova-text-faint)]">{t('settings.model.discoveryLoading')}</div>
        )}
        {state.status === 'error' && state.models.length === 0 && (
          <div className="px-1 py-2 text-xs leading-5 text-red-600 dark:text-red-400" role="alert">
            {t('settings.model.discoveryFailure', { error: state.message })}
          </div>
        )}
        {state.status === 'success' && state.models.length === 0 && (
          <div className="px-1 py-3 text-xs text-[var(--nova-text-faint)]">{t('settings.model.discoveryEmpty')}</div>
        )}
        {state.models.length > 0 && (
          <div role="listbox" aria-multiselectable="true" aria-label={t('settings.model.discoveryList')} className="max-h-72 overflow-y-auto overscroll-contain [scrollbar-gutter:stable]">
            {visibleModels.map((model) => {
              const alreadyAdded = existing.has(model.id)
              const checked = selected.has(model.id) || alreadyAdded
              return (
                <button
                  key={model.id}
                  type="button"
                  role="option"
                  aria-selected={checked}
                  disabled={alreadyAdded}
                  className={cn(
                    'flex min-h-8 w-full min-w-0 items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs',
                    checked ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]',
                    alreadyAdded && 'opacity-60',
                  )}
                  onClick={() => setSelected((current) => {
                    const next = new Set(current)
                    if (next.has(model.id)) next.delete(model.id)
                    else next.add(model.id)
                    return next
                  })}
                >
                  <span className="min-w-0 flex-1">
                    <span className="block truncate" title={model.display_name || model.id}>{model.display_name || model.id}</span>
                    {model.display_name && <span className="block truncate font-mono text-[10px] text-[var(--nova-text-faint)]">{model.id}</span>}
                  </span>
                  {alreadyAdded ? <span className="shrink-0 text-[10px] text-[var(--nova-text-faint)]">{t('settings.model.discoveryAdded')}</span> : checked && <Check />}
                </button>
              )
            })}
          </div>
        )}
        <Button
          type="button"
          size="sm"
          disabled={selectedModels.length === 0}
          onClick={() => {
            onAdd(selectedModels)
            setSelected(new Set())
            setOpen(false)
          }}
        >
          <Plus data-icon="inline-start" />
          {t('settings.model.discoveryAddSelected', { count: selectedModels.length })}
        </Button>
      </PopoverContent>
    </Popover>
  )
}
